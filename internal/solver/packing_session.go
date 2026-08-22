package solver

import (
	"math/bits"
	"sort"

	"backpack-brawl-solver/internal/model"
)

// constellationRootPackingSession retains an in-flight dynamic-MRV layer so a
// node allocation can end between any two candidate expansions.
type constellationRootPackingSession struct {
	catalog           model.Catalog
	instances         []model.InventoryInstance
	optionsByInstance map[string][]model.Placement
	root              constellationSkeleton
	config            Config
	gridMask          uint64
	reportNode        func(bool) bool

	result    constellationRootPackingResult
	shadow    *constellationShadowReference
	remaining []model.InventoryInstance
	beamWidth int

	states      []constellationRootMRVState
	depth       int
	depthActive bool
	nextByClass map[string]constellationRootMRVState
	depthInfo   model.ConstellationRootPackingDepthDiagnostic
	shadowDepth *model.ConstellationForcedCandidateShadowDepth

	stateIndex    int
	statePrepared bool
	optionIndex   int
	selected      model.InventoryInstance
	options       []model.Placement
	nextMask      uint64
	nextArea      int

	selectedItemIDs map[string]struct{}
	initialized     bool
	done            bool
	final           constellationRootPackingResult
}

func newConstellationRootPackingSession(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	reportNode func(bool) bool,
) *constellationRootPackingSession {
	anchoredIDs := make([]string, 0, len(root.placed))
	for _, placement := range root.placed {
		anchoredIDs = append(anchoredIDs, placement.InstanceID)
	}
	sort.Strings(anchoredIDs)
	session := &constellationRootPackingSession{
		catalog:           catalog,
		instances:         instances,
		optionsByInstance: optionsByInstance,
		root:              root,
		config:            config,
		gridMask:          gridMask,
		reportNode:        reportNode,
		result: constellationRootPackingResult{
			initialOccupiedMask: root.occupied,
			initialFreeMask:     gridMask &^ root.occupied,
			anchoredInstanceIDs: anchoredIDs,
			packingStrategy:     constellationPackingStrategyStateMRV,
		},
		selectedItemIDs: make(map[string]struct{}),
	}
	if config.constellationRootPackingCollector != nil {
		session.shadow = config.constellationRootPackingCollector.shadow
	}
	return session
}

// Run consumes at most nodeAllocation additional candidate expansions. It may
// be called again after an allocation ends; completed sessions are immutable.
func (session *constellationRootPackingSession) Run(nodeAllocation int64) constellationRootPackingResult {
	if session.done {
		return session.final
	}
	if nodeAllocation <= 0 {
		if !session.initialized {
			result := session.result
			result.terminationReason = "no_budget"
			return result
		}
		return session.pauseResult()
	}
	if !session.initialized {
		session.initialize()
		if session.done {
			return session.final
		}
	}

	var consumed int64
	for !session.done {
		if !session.depthActive {
			if consumed >= nodeAllocation {
				return session.pauseResult()
			}
			session.beginDepth()
			continue
		}
		if session.statePrepared {
			if session.optionIndex >= len(session.options) {
				session.statePrepared = false
				session.stateIndex++
				continue
			}
			if consumed >= nodeAllocation {
				return session.pauseResult()
			}
			if !session.reportNode(false) {
				session.commitDepth()
				session.finish(true)
				return session.final
			}
			consumed++
			session.result.nodes++
			option := session.options[session.optionIndex]
			session.optionIndex++
			session.expandOption(option)
			continue
		}
		if session.stateIndex >= len(session.states) {
			session.commitDepth()
			if len(session.states) == 0 || session.depth > len(session.remaining) {
				session.finish(false)
			}
			continue
		}
		if consumed >= nodeAllocation {
			return session.pauseResult()
		}
		session.prepareState()
	}
	return session.final
}

func (session *constellationRootPackingSession) Done() bool {
	return session.done
}

func (session *constellationRootPackingSession) initialize() {
	session.initialized = true
	placedByID := placementByInstanceID(session.root.placed)
	remainingMask := uint64(0)
	remainingArea := 0
	for _, instance := range session.instances {
		if _, placed := placedByID[instance.InstanceID]; placed {
			continue
		}
		session.remaining = append(session.remaining, instance)
	}
	if len(session.remaining) == 0 {
		evaluation := evaluateLayoutForConfig(session.catalog, session.root.placed, session.config)
		session.result.solutions = []model.Solution{{
			Placements:          append([]model.Placement(nil), session.root.placed...),
			Evaluation:          evaluation,
			LayoutKey:           layoutKey(session.root.placed, session.instances),
			CanonicalLayoutHash: canonicalLayoutHash(session.root.placed),
		}}
		session.result.candidates = 1
		session.result.terminationReason = "completed"
		session.done = true
		session.final = session.result
		return
	}
	session.remaining = packingSeedOrder(session.remaining, session.optionsByInstance)
	remainingOrder := make([]string, 0, len(session.remaining))
	for _, instance := range session.remaining {
		remainingOrder = append(remainingOrder, instance.InstanceID)
		remainingMask |= uint64(1) << uint(instance.OriginalIndex)
		remainingArea += len(session.catalog.Items[instance.ItemID].Shape)
	}
	session.result.remainingPackingOrder = remainingOrder
	initialRestricted, initialFlexibility, feasible := packingFeasibility(session.remaining, session.optionsByInstance, session.root.occupied, session.root.placed)
	session.result.initialRestricted = initialRestricted
	session.result.initialFlexibility = initialFlexibility
	session.result.initialFragmentation = freeSpaceFragmentation(session.gridMask, session.root.occupied)
	session.result.packingInputKey = constellationRootPackingInputKey(session.root.sourceGeometryKey, session.result.initialOccupiedMask, session.result.initialFreeMask, session.result.anchoredInstanceIDs, remainingOrder)
	if !feasible {
		session.result.hardPruned = 1
		session.result.terminationReason = "hard_dead"
		session.done = true
		session.final = session.result
		return
	}
	session.beamWidth = policyForConfig(session.config).ConstellationSeedPackingBeamWidth
	if session.config.forcedRootPackingReplay && session.config.ConstellationForcedCandidateRootedPackingBeamWidth > 0 {
		session.beamWidth = session.config.ConstellationForcedCandidateRootedPackingBeamWidth
	}
	if session.beamWidth <= 0 {
		session.beamWidth = policyForConfig(session.config).PackingSeedBeamWidth
	}
	session.states = []constellationRootMRVState{{
		packingSeedState: packingSeedState{
			occupied:      session.root.occupied,
			placed:        append([]model.Placement(nil), session.root.placed...),
			restricted:    session.result.initialRestricted,
			flexibility:   session.result.initialFlexibility,
			fragmentation: session.result.initialFragmentation,
			score:         session.root.score,
			key:           session.root.signature,
		},
		remainingMask: remainingMask,
		remainingArea: remainingArea,
	}}
	session.depth = 1
}

func (session *constellationRootPackingSession) beginDepth() {
	session.depthActive = true
	session.nextByClass = make(map[string]constellationRootMRVState, session.beamWidth*4)
	session.depthInfo = model.ConstellationRootPackingDepthDiagnostic{
		Depth:                     session.depth,
		SelectedInstanceHistogram: make(map[string]int64),
		StatesBeforeExpansion:     len(session.states),
	}
	session.stateIndex = 0
	session.statePrepared = false
	session.optionIndex = 0
	session.shadowDepth = nil
	if session.shadow == nil {
		return
	}
	compatibleBefore := 0
	for _, state := range session.states {
		if session.shadow.compatible(state.placed) {
			compatibleBefore++
		}
	}
	session.shadowDepth = &model.ConstellationForcedCandidateShadowDepth{
		Depth:                         session.depth,
		StatesBeforeExpansion:         len(session.states),
		StatesBeforeWitnessCompatible: compatibleBefore,
	}
}

func (session *constellationRootPackingSession) prepareState() {
	state := session.states[session.stateIndex]
	if state.remainingArea > bits.OnesCount64(session.gridMask&^state.occupied) {
		session.result.hardPruned++
		session.stateIndex++
		return
	}
	selectedIndex, options, selected := constellationRootMRVSelection(session.remaining, state.remainingMask, session.optionsByInstance, state.occupied, state.placed)
	if !selected {
		session.result.hardPruned++
		session.depthInfo.ZeroDomainPrunes++
		session.stateIndex++
		return
	}
	session.selected = session.remaining[selectedIndex]
	if session.shadowDepth != nil {
		session.shadowDepth.ShadowSymmetryPruned += constellationShadowSymmetryPrunedOptions(session.selected, session.optionsByInstance[session.selected.InstanceID], state.occupied, state.placed, session.shadow)
	}
	selectedLegal := len(options)
	session.depthInfo.SelectedInstanceHistogram[session.selected.InstanceID]++
	if session.depthInfo.MinLegalPlacements == 0 || selectedLegal < session.depthInfo.MinLegalPlacements {
		session.depthInfo.MinLegalPlacements = selectedLegal
	}
	if selectedLegal > session.depthInfo.MaxLegalPlacements {
		session.depthInfo.MaxLegalPlacements = selectedLegal
	}
	session.selectedItemIDs[session.selected.ItemID] = struct{}{}
	session.options = options
	session.optionIndex = 0
	session.nextMask = state.remainingMask &^ (uint64(1) << uint(session.selected.OriginalIndex))
	session.nextArea = state.remainingArea - len(session.catalog.Items[session.selected.ItemID].Shape)
	session.statePrepared = true
}

func (session *constellationRootPackingSession) expandOption(option model.Placement) {
	state := session.states[session.stateIndex]
	nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
	shadowCompatible := session.shadow != nil && session.shadow.compatible(nextPlaced)
	nextOccupied := state.occupied | option.Mask
	restricted, flexibility, feasible := constellationRootMRVFeasibility(session.remaining, session.nextMask, session.optionsByInstance, nextOccupied, nextPlaced)
	if !feasible {
		session.result.hardPruned++
		session.depthInfo.ZeroDomainPrunes++
		if shadowCompatible {
			session.shadowDepth.ShadowFeasibilityPruned++
		}
		return
	}
	if session.nextMask == 0 && session.result.firstCompleteNodes == 0 {
		session.result.firstCompleteNodes = session.result.nodes
	}
	candidate := constellationRootMRVState{
		packingSeedState: packingSeedState{
			occupied:      nextOccupied,
			placed:        nextPlaced,
			restricted:    restricted,
			flexibility:   flexibility,
			fragmentation: freeSpaceFragmentation(session.gridMask, nextOccupied),
			score:         evaluateScoreForConfig(session.catalog, nextPlaced, session.config),
			key:           session.root.signature + "|" + coverageSeedAppendKey(state.key, option),
		},
		remainingMask: session.nextMask,
		remainingArea: session.nextArea,
	}
	if session.shadowDepth != nil {
		session.shadowDepth.Generated++
		if shadowCompatible {
			session.shadowDepth.GeneratedWitnessCompatible++
		}
	}
	classKey := constellationRootMRVStateKey(candidate)
	if previous, exists := session.nextByClass[classKey]; exists {
		session.result.deduplicated++
		if !constellationRootPackingStateLess(session.config, candidate.packingSeedState, previous.packingSeedState) {
			return
		}
	}
	session.nextByClass[classKey] = candidate
}

func (session *constellationRootPackingSession) commitDepth() {
	trace := (*model.ConstellationForcedCandidateShadowTrace)(nil)
	if session.shadow != nil {
		trace = &session.shadow.trace
	}
	session.states = constellationRootPackingFinishMRVDepth(session.nextByClass, &session.depthInfo, session.shadowDepth, session.shadow, trace, session.beamWidth, session.config)
	session.result.beamEvictions += session.depthInfo.BeamEvictions
	session.result.mrvDepths = append(session.result.mrvDepths, session.depthInfo)
	session.result.layerWidths = append(session.result.layerWidths, model.PackingSeedLayerWidth{Depth: session.depth, States: len(session.states)})
	session.depth++
	session.depthActive = false
	session.nextByClass = nil
	session.shadowDepth = nil
}

func (session *constellationRootPackingSession) pauseResult() constellationRootPackingResult {
	if !session.depthActive {
		trace := (*model.ConstellationForcedCandidateShadowTrace)(nil)
		if session.shadow != nil {
			trace = &session.shadow.trace
		}
		return session.resultForFrontier(session.result, session.states, true, trace)
	}
	result := session.result
	result.mrvDepths = append([]model.ConstellationRootPackingDepthDiagnostic(nil), result.mrvDepths...)
	result.layerWidths = append([]model.PackingSeedLayerWidth(nil), result.layerWidths...)
	depthInfo := session.depthInfo
	var shadowDepth *model.ConstellationForcedCandidateShadowDepth
	var trace *model.ConstellationForcedCandidateShadowTrace
	if session.shadowDepth != nil {
		copy := *session.shadowDepth
		shadowDepth = &copy
		traceCopy := session.shadow.trace
		traceCopy.Depths = append([]model.ConstellationForcedCandidateShadowDepth(nil), traceCopy.Depths...)
		trace = &traceCopy
	}
	states := constellationRootPackingFinishMRVDepth(session.nextByClass, &depthInfo, shadowDepth, session.shadow, trace, session.beamWidth, session.config)
	result.beamEvictions += depthInfo.BeamEvictions
	result.mrvDepths = append(result.mrvDepths, depthInfo)
	result.layerWidths = append(result.layerWidths, model.PackingSeedLayerWidth{Depth: session.depth, States: len(states)})
	return session.resultForFrontier(result, states, true, trace)
}

func (session *constellationRootPackingSession) finish(exhausted bool) {
	trace := (*model.ConstellationForcedCandidateShadowTrace)(nil)
	if session.shadow != nil {
		trace = &session.shadow.trace
	}
	session.final = session.resultForFrontier(session.result, session.states, exhausted, trace)
	session.done = true
}

func (session *constellationRootPackingSession) resultForFrontier(result constellationRootPackingResult, states []constellationRootMRVState, exhausted bool, trace *model.ConstellationForcedCandidateShadowTrace) constellationRootPackingResult {
	results := make([]model.Solution, 0, session.config.TopN)
	candidates := 0
	for _, state := range states {
		if state.remainingMask != 0 {
			continue
		}
		candidates++
		results = collectConstellationRootPackingSolution(session.catalog, results, state.placed, session.instances, session.root.rootID, session.config)
	}
	result.solutions = results
	result.candidates = candidates
	result.distinctNextItems = len(session.selectedItemIDs)
	if trace != nil {
		traceCopy := *trace
		traceCopy.Depths = append([]model.ConstellationForcedCandidateShadowDepth(nil), trace.Depths...)
		result.shadowTrace = &traceCopy
	}
	allComplete := len(states) > 0
	for _, state := range states {
		if state.remainingMask != 0 {
			allComplete = false
			break
		}
	}
	result.terminationReason = "completed"
	if exhausted && !allComplete {
		result.terminationReason = "budget_exhausted"
	} else if len(states) == 0 {
		result.terminationReason = "no_states"
	}
	return result
}

func constellationRootPackingFinishMRVDepth(
	nextByClass map[string]constellationRootMRVState,
	depthInfo *model.ConstellationRootPackingDepthDiagnostic,
	shadowDepth *model.ConstellationForcedCandidateShadowDepth,
	shadow *constellationShadowReference,
	trace *model.ConstellationForcedCandidateShadowTrace,
	beamWidth int,
	config Config,
) []constellationRootMRVState {
	depthInfo.StatesAfterDedup = len(nextByClass)
	if shadowDepth != nil {
		shadowDepth.Deduplicated = len(nextByClass)
		for _, state := range nextByClass {
			if shadow.compatible(state.placed) {
				shadowDepth.DedupWitnessCompatible++
			}
		}
	}
	next := make([]constellationRootMRVState, 0, len(nextByClass))
	for _, state := range nextByClass {
		next = append(next, state)
	}
	sort.Slice(next, func(i, j int) bool {
		return constellationRootPackingStateLess(config, next[i].packingSeedState, next[j].packingSeedState)
	})
	if shadowDepth != nil {
		shadowDepth.PrecutStates = len(next)
		bestCompatible := -1
		for index, state := range next {
			if !shadow.compatible(state.placed) {
				continue
			}
			shadowDepth.PrecutWitnessCompatible++
			if bestCompatible < 0 {
				bestCompatible = index
				shadowDepth.BestPrecutRank = index + 1
			}
		}
		if bestCompatible >= 0 {
			bestState := next[bestCompatible].packingSeedState
			tieStart, tieEnd := bestCompatible, bestCompatible
			for index, state := range next {
				compare := constellationRootPackingStatePrimaryCompare(config, state.packingSeedState, bestState)
				if compare > 0 {
					shadowDepth.BestPrecutBetterCount++
				}
				if compare == 0 {
					shadowDepth.BestPrecutTieCount++
					if index < tieStart {
						tieStart = index
					}
					if index > tieEnd {
						tieEnd = index
					}
				}
			}
			shadowDepth.CutoffTieCrossed = tieStart < beamWidth && tieEnd >= beamWidth
			shadowDepth.RankingAutopsy = constellationShadowRankingAutopsy(next, bestCompatible, beamWidth, config)
			shadowDepth.CounterfactualReranking = constellationShadowCounterfactualReranking(next, shadow, beamWidth)
		}
	}
	if len(next) > beamWidth {
		depthInfo.BeamEvictions = int64(len(next) - beamWidth)
		clear(next[beamWidth:])
		next = next[:beamWidth]
	}
	if shadowDepth != nil {
		shadowDepth.RetainedStates = len(next)
		for index, state := range next {
			if shadow.compatible(state.placed) {
				shadowDepth.RetainedWitnessCompatible++
				if shadowDepth.BestRetainedRank == 0 {
					shadowDepth.BestRetainedRank = index + 1
				}
			}
		}
		if trace.FirstLossStage == "" && shadowDepth.StatesBeforeWitnessCompatible > 0 && shadowDepth.RetainedWitnessCompatible == 0 {
			trace.FirstLossDepth = depthInfo.Depth
			switch {
			case shadowDepth.GeneratedWitnessCompatible == 0 && shadowDepth.ShadowSymmetryPruned > 0:
				trace.FirstLossStage = "symmetry"
			case shadowDepth.GeneratedWitnessCompatible == 0 && shadowDepth.ShadowFeasibilityPruned > 0:
				trace.FirstLossStage = "feasibility"
			case shadowDepth.GeneratedWitnessCompatible == 0:
				trace.FirstLossStage = "generation"
			case shadowDepth.DedupWitnessCompatible == 0:
				trace.FirstLossStage = "dedup"
			default:
				trace.FirstLossStage = "beam"
				trace.FirstBeamEvictionDepth = depthInfo.Depth
			}
		}
		trace.Depths = append(trace.Depths, *shadowDepth)
	}
	depthInfo.StatesRetained = len(next)
	return next
}

func constellationRootPackingMRV(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func(bool) bool,
) constellationRootPackingResult {
	return newConstellationRootPackingSession(catalog, instances, optionsByInstance, root, config, gridMask, reportNode).Run(nodeBudget)
}

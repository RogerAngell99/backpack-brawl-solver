package solver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

// constellationSeedState keeps priority sources and their geometric targets
// together before the remaining inventory is packed. It deliberately contains
// no diagnostic reference data.
type constellationSeedState struct {
	occupied  uint64
	placed    []model.Placement
	score     model.Score
	potential int
	exactKey  string
}

type constellationSkeleton struct {
	id                     string
	rootID                 string
	occupied               uint64
	placed                 []model.Placement
	score                  model.Score
	signature              string
	exactKey               string
	priorityLinks          []model.PlateauLink
	sourceGeometryKey      string
	sourceGeometryOrbitKey string
	targetAssignmentKey    string
	selectionPolicy        string
	relaxedFromExactKey    string
	frontierExactKey       string
	relaxationFrontierSize int
}

type constellationRootPackingResult struct {
	solutions             []model.Solution
	nodes                 int64
	candidates            int
	deduplicated          int64
	hardPruned            int64
	symmetryPruned        int64
	beamEvictions         int64
	layerWidths           []model.PackingSeedLayerWidth
	initialOccupiedMask   uint64
	initialFreeMask       uint64
	anchoredInstanceIDs   []string
	remainingPackingOrder []string
	initialRestricted     int
	initialFlexibility    int
	initialFragmentation  int
	terminationReason     string
	packingInputKey       string
	packingStrategy       string
	firstCompleteNodes    int64
	distinctNextItems     int
	mrvDepths             []model.ConstellationRootPackingDepthDiagnostic
	shadowTrace           *model.ConstellationForcedCandidateShadowTrace
	parentGuardedFrontier *model.ConstellationParentFrontierHedgeAttempt
}

type constellationCompletionProbeResult struct {
	nodes             int64
	terminationReason string
	feasibilityStatus string
	searchExhausted   bool
	witnessHash       string
	stopSource        string
}

type constellationCompletionOptimizationProbeResult struct {
	nodes                           int64
	status                          string
	terminationReason               string
	stopSource                      string
	searchExhausted                 bool
	initialIncumbentFromRootPacking bool
	hasBest                         bool
	bestScore                       model.Score
	bestLayoutKey                   string
	bestHash                        string
	bestPlacements                  []model.Placement
	terminalCompletions             int64
	areaPrunes                      int64
	zeroDomainPrunes                int64
	transpositionPrunes             int64
	firstCompleteNodes              int64
	hasFirstComplete                bool
	firstBestNodes                  int64
	improvements                    []model.ConstellationCandidateCompletionScoreImprovement
}

type constellationRootMRVState struct {
	packingSeedState
	remainingMask uint64
	remainingArea int
}

type constellationSelectedRootOutcome struct {
	rootID        string
	nodesReserved int64
	result        constellationRootPackingResult
}

// constellationCandidateCompletionSnapshot is immutable diagnostic input saved
// while V4 construction is still in scope, then consumed after normal search.
type constellationCandidateCompletionSnapshot struct {
	candidates        map[string]constellationSkeleton
	selectedRoots     map[string]constellationSelectedRootOutcome
	selectedSkeletons map[string]constellationSkeleton
}

// constellationRootPackingCollector controls only where complete root layouts
// are emitted. It never participates in state expansion or ranking.
type constellationRootPackingCollector struct {
	promote bool
	shadow  *constellationShadowReference
}

type constellationShadowReference struct {
	anchoredByInstance map[string]string
	unanchoredByItem   map[string]map[string]int
	trace              model.ConstellationForcedCandidateShadowTrace
}

const highParentConsumptionProbeThresholdBps int64 = 5_000

func constellationSeedSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	potential *starPotentialContext,
	progress *progressTracker,
) (coverageSeedResult, model.ConstellationSeedDiagnostics) {
	policy := policyForConfig(config)
	diagnostics := model.ConstellationSeedDiagnostics{
		Version:          policy.ConstellationSeedVersion,
		ShareBps:         policy.ConstellationSeedShareBps,
		PackingBeamWidth: policy.ConstellationSeedPackingBeamWidth,
	}
	if policy.ConstellationSeedVersion == "" || nodeBudget <= 0 || potential == nil || config.priorityBounds == nil || !constellationSeedRuntimeEligible(catalog, instances, config) {
		return coverageSeedResult{}, diagnostics
	}

	sources, sourceItems := constellationSources(catalog, instances, config.Priorities)
	if len(sources) == 0 || len(sources) > 4 {
		return coverageSeedResult{}, diagnostics
	}
	targets := constellationTargets(instances, sources, potential, policy.ConstellationSeedTargetInstanceLimit)
	constructionBudget := nodeBudget * policy.ConstellationSeedConstructionBps / 10_000
	if constructionBudget <= 0 || constructionBudget >= nodeBudget {
		constructionBudget = nodeBudget
	}

	var nodes int64
	var symmetryPruned int64
	var generated int64
	var deduplicated int64
	var progressBatch int64
	exhausted := false
	constructionExhausted := false
	reportNode := func(construction bool) bool {
		if !chargeNode(config, tracePhaseConstellationSeed) {
			exhausted = true
			return false
		}
		nodes++
		if construction {
			diagnostics.ConstructionNodes++
		} else {
			diagnostics.PackingNodes++
		}
		if progress != nil {
			progressBatch++
			if progressBatch >= progressNodeInterval {
				progress.addNodes(ProgressPhaseSeed, progressBatch, false)
				progressBatch = 0
			}
		}
		return true
	}
	reportSweepNode := func() (bool, string) {
		ok, stopSource := chargeNodeWithReason(config, tracePhaseConstellationSeed)
		if !ok {
			exhausted = true
			return false, stopSource
		}
		nodes++
		diagnostics.PoolSweepNodes++
		if progress != nil {
			progressBatch++
			if progressBatch >= progressNodeInterval {
				progress.addNodes(ProgressPhaseSeed, progressBatch, false)
				progressBatch = 0
			}
		}
		return true, ""
	}
	reportCandidateOptimizationNode := func() (bool, string) {
		ok, stopSource := chargeNodeWithReason(config, tracePhaseConstellationSeed)
		if !ok {
			exhausted = true
			return false, stopSource
		}
		nodes++
		diagnostics.CandidateCompletionOptimizationNodes++
		if progress != nil {
			progressBatch++
			if progressBatch >= progressNodeInterval {
				progress.addNodes(ProgressPhaseSeed, progressBatch, false)
				progressBatch = 0
			}
		}
		return true, ""
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
		}
	}

	states := []constellationSeedState{{}}
	advanceSources := func(instance model.InventoryInstance, sourcesComplete bool) {
		next := make([]constellationSeedState, 0, policy.ConstellationSeedBeamWidth*2)
		for _, state := range states {
			if exhausted || diagnostics.ConstructionNodes >= constructionBudget {
				constructionExhausted = true
				break
			}
			for _, option := range constellationSourceOptions(optionsByInstance[instance.InstanceID], state.occupied, policy.ConstellationSeedSourceOptionLimit, instance, sources) {
				if !placementRespectsCanonicalCopyOrder(option, state.placed) {
					symmetryPruned++
					continue
				}
				if !reportNode(true) {
					break
				}
				generated++
				placed, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				next = append(next, constellationSeedState{
					occupied:  state.occupied | option.Mask,
					placed:    placed,
					score:     evaluateScoreForConfig(catalog, placed, config),
					potential: state.potential + potential.priorityForPlacement(option),
					exactKey:  constellationExactKey(placed),
				})
				if diagnostics.ConstructionNodes >= constructionBudget {
					constructionExhausted = true
					break
				}
			}
		}
		if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) && sourcesComplete {
			next = filterConstellationPriorityFeasibleStates(catalog, instances, optionsByInstance, gridMask, config, next)
		}
		states, deduplicated = selectConstellationStates(next, policy, sourceItems, len(sources), catalog, config, deduplicated)
	}
	for sourceIndex, source := range sources {
		advanceSources(source, sourceIndex == len(sources)-1)
		if exhausted || len(states) == 0 || (constructionExhausted && sourceIndex < len(sources)-1) {
			diagnostics.StatesGenerated = generated
			diagnostics.StatesDeduplicated = deduplicated
			diagnostics.SourceStatesRetained = len(states)
			flushProgress()
			return coverageSeedResult{NodesExplored: nodes, SymmetryPrunedBranches: symmetryPruned}, diagnostics
		}
	}
	diagnostics.SourceStatesRetained = len(states)

	skeletons := make(map[string]constellationSkeleton, policy.ConstellationSeedMaxSkeletons)
	candidateSkeletons := make(map[string]constellationSkeleton)
	targetAssignments := make(map[string]struct{})
	collectSkeletons := func(candidates []constellationSeedState) {
		for _, state := range candidates {
			if !constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) && len(skeletons) >= policy.ConstellationSeedMaxSkeletons {
				return
			}
			if !config.priorityBounds.reached(state.score) {
				continue
			}
			evaluation := evaluateLayoutForConfig(catalog, state.placed, config)
			signature := constellationSignature(state.placed, evaluation.Stars, sourceItems)
			sourceGeometryKey := constellationPrioritySourceGeometryKey(state.placed, sourceItems)
			targetAssignmentKey := constellationTargetAssignmentKey(state.placed, evaluation.Stars, sourceItems)
			targetAssignments[targetAssignmentKey] = struct{}{}
			candidate := constellationSkeleton{
				occupied:            state.occupied,
				placed:              append([]model.Placement(nil), state.placed...),
				score:               cloneScore(evaluation.Score),
				signature:           signature,
				exactKey:            state.exactKey,
				priorityLinks:       constellationPriorityLinks(state.placed, evaluation.Stars, sourceItems),
				sourceGeometryKey:   sourceGeometryKey,
				targetAssignmentKey: targetAssignmentKey,
			}
			if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
				candidate.sourceGeometryOrbitKey = constellationPrioritySourceGeometryOrbitKey(state.placed, sourceItems)
				if existing, exists := candidateSkeletons[candidate.exactKey]; !exists || constellationSkeletonLess(candidate, existing) {
					candidateSkeletons[candidate.exactKey] = candidate
				}
				continue
			}
			if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) {
				retainConstellationV2Skeleton(skeletons, candidate, policy.ConstellationSeedMaxSourceGeometries)
				continue
			}
			if _, exists := skeletons[signature]; exists {
				continue
			}
			skeletons[signature] = candidate
		}
	}
	collectSkeletons(states)

	for _, target := range targets {
		if exhausted || constructionExhausted || len(states) == 0 || (!constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) && len(skeletons) >= policy.ConstellationSeedMaxSkeletons) {
			break
		}
		next := make([]constellationSeedState, 0, policy.ConstellationSeedBeamWidth*3)
		for _, state := range states {
			// Deferring a target keeps it available to rooted packing later.
			next = append(next, state)
			if exhausted || diagnostics.ConstructionNodes >= constructionBudget {
				constructionExhausted = true
				break
			}
			targetOptions := constellationOptions(optionsByInstance[target.InstanceID], potential, policy.ConstellationSeedTargetOptionLimit)
			if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) {
				targetOptions = constellationOptionsForState(optionsByInstance[target.InstanceID], potential, policy.ConstellationSeedTargetOptionLimit, state.occupied)
			}
			for _, option := range targetOptions {
				if option.Mask&state.occupied != 0 {
					continue
				}
				if !placementRespectsCanonicalCopyOrder(option, state.placed) {
					symmetryPruned++
					continue
				}
				if !reportNode(true) {
					break
				}
				generated++
				placed, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				next = append(next, constellationSeedState{
					occupied:  state.occupied | option.Mask,
					placed:    placed,
					score:     evaluateScoreForConfig(catalog, placed, config),
					potential: state.potential + potential.priorityForPlacement(option),
					exactKey:  constellationExactKey(placed),
				})
				if diagnostics.ConstructionNodes >= constructionBudget {
					constructionExhausted = true
					break
				}
			}
		}
		if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) {
			next = filterConstellationPriorityFeasibleStates(catalog, instances, optionsByInstance, gridMask, config, next)
		}
		states, deduplicated = selectConstellationStates(next, policy, sourceItems, len(sources), catalog, config, deduplicated)
		collectSkeletons(states)
	}
	diagnostics.TargetInstancesConsidered = len(targets)
	diagnostics.TargetStatesRetained = len(states)

	diagnostics.StatesGenerated = generated
	diagnostics.StatesDeduplicated = deduplicated
	if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
		if policy.ConstellationSeedVariant == ConstellationSeedVariantV5 {
			skeletons = selectConstellationV5Skeletons(candidateSkeletons, policy.ConstellationSeedMaxSkeletons)
		} else if policy.ConstellationSeedVariant == ConstellationSeedVariantV51 {
			skeletons = selectConstellationV51Skeletons(candidateSkeletons, policy.ConstellationSeedMaxSkeletons)
		}
		if policy.ConstellationSeedVariant == ConstellationSeedVariantV5 || policy.ConstellationSeedVariant == ConstellationSeedVariantV51 {
			for _, skeleton := range skeletons {
				if skeleton.selectionPolicy != "v5_relaxation_frontier" && skeleton.selectionPolicy != "parent_guarded_frontier" {
					continue
				}
				diagnostics.RelaxationFrontierExists = true
				diagnostics.RelaxationFrontierSelected = true
				if skeleton.selectionPolicy == "parent_guarded_frontier" {
					diagnostics.RelaxationFrontierParentExactKey = skeleton.exactKey
				} else {
					diagnostics.RelaxationFrontierParentExactKey = skeleton.relaxedFromExactKey
				}
				diagnostics.RelaxationFrontierSize = skeleton.relaxationFrontierSize
				break
			}
		}
		if policy.ConstellationSeedVariant != ConstellationSeedVariantV5 && policy.ConstellationSeedVariant != ConstellationSeedVariantV51 {
			skeletons = selectConstellationV4Skeletons(candidateSkeletons, policy.ConstellationSeedMaxSkeletons)
		}
		diagnostics.CandidatePrioritySourceGeometryCount = len(candidateSourceGeometryKeys(candidateSkeletons))
		diagnostics.CandidatePrioritySourceGeometryOrbitCount = len(candidateSourceGeometryOrbitKeys(candidateSkeletons))
		diagnostics.CandidateRootFreeMaskCount = len(candidateRootFreeMasks(candidateSkeletons, gridMask))
		diagnostics.CandidateRootFreeMaskOrbitCount = len(candidateRootFreeMaskOrbitKeys(candidateSkeletons, gridMask))
	}
	diagnostics.SkeletonsReached = len(skeletons)
	diagnostics.SkeletonsDistinct = len(skeletons)
	diagnostics.PriorityTargetAssignmentCount = len(targetAssignments)
	if len(skeletons) == 0 {
		flushProgress()
		return coverageSeedResult{NodesExplored: nodes, SymmetryPrunedBranches: symmetryPruned}, diagnostics
	}

	roots := make([]constellationSkeleton, 0, len(skeletons))
	for _, skeleton := range skeletons {
		roots = append(roots, skeleton)
	}
	sort.Slice(roots, func(i, j int) bool {
		if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) {
			return constellationSkeletonLess(roots[i], roots[j])
		}
		if compare := comparePriorityCounts(roots[i].score.PriorityCounts, roots[j].score.PriorityCounts); compare != 0 {
			return compare > 0
		}
		return roots[i].signature < roots[j].signature
	})
	for index := range roots {
		roots[index].id = "skeleton-" + strconv.Itoa(index+1)
		roots[index].rootID = "root-" + strconv.Itoa(index+1)
		if roots[index].selectionPolicy == "v5_relaxation_frontier" || roots[index].selectionPolicy == "parent_guarded_frontier" {
			diagnostics.RelaxationFrontierRootID = roots[index].rootID
		}
		if config.Diagnostics {
			orbitKey := ""
			if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
				orbitKey = roots[index].sourceGeometryOrbitKey
			}
			diagnostics.Skeletons = append(diagnostics.Skeletons, model.ConstellationSkeletonDiagnostic{
				ID:                             roots[index].id,
				Signature:                      roots[index].signature,
				ExactKey:                       roots[index].exactKey,
				AnchorCount:                    len(roots[index].placed),
				Score:                          cloneScore(roots[index].score),
				PriorityLinks:                  append([]model.PlateauLink(nil), roots[index].priorityLinks...),
				PrioritySourceGeometryKey:      roots[index].sourceGeometryKey,
				PrioritySourceGeometryOrbitKey: orbitKey,
				PriorityTargetAssignmentKey:    roots[index].targetAssignmentKey,
				SelectionPolicy:                roots[index].selectionPolicy,
				RelaxedFromExactKey:            roots[index].relaxedFromExactKey,
				FrontierExactKey:               roots[index].frontierExactKey,
				RelaxationFrontierSize:         roots[index].relaxationFrontierSize,
			})
		}
	}

	results := make([]model.Solution, 0, config.TopN)
	var rootWinner model.Solution
	rootWinnerID := ""
	var candidates int
	var statesDeduplicated int64
	var hardPruned int64
	rootFreeMasks := make(map[uint64]struct{}, len(roots))
	selectedRootOutcomes := make(map[string]constellationSelectedRootOutcome, len(roots))
	packingNodesLeft := nodeBudget - diagnostics.ConstructionNodes
	recordRoot := func(index int, root constellationSkeleton, quota int64, rootResult constellationRootPackingResult, probeAvailable int64, familyID string, rounds []model.ConstellationRootPackingAllocationRound, familyReturned int64) int64 {
		selectedRootOutcomes[root.exactKey] = constellationSelectedRootOutcome{rootID: root.rootID, nodesReserved: quota, result: rootResult}
		if probeAvailable < 0 {
			probeAvailable = 0
		}
		var probeResult constellationCompletionProbeResult
		if config.Diagnostics && policy.ConstellationFeasibilityProbe {
			if len(rootResult.solutions) > 0 {
				probeResult = constellationCompletionProbeResult{
					terminationReason: "root_packing_witness",
					feasibilityStatus: "feasible",
					witnessHash:       rootResult.solutions[0].CanonicalLayoutHash,
				}
			} else {
				probeResult = constellationRootCompletionProbe(catalog, instances, optionsByInstance, root, gridMask, probeAvailable, reportNode)
			}
		}
		var optimizationProbeResult constellationCompletionOptimizationProbeResult
		optimizationProbeTargeted := constellationCompletionOptimizationProbeTargetsRoot(policy, index)
		optimizationProbeEligible := config.Diagnostics && policy.ConstellationCompletionOptimizationProbe && optimizationProbeTargeted && rootResult.packingStrategy == constellationPackingStrategyStateMRV && len(rootResult.solutions) > 0
		if optimizationProbeEligible {
			optimizationProbeResult = constellationRootCompletionOptimizationProbe(catalog, instances, optionsByInstance, root, config, gridMask, probeAvailable, reportNode, rootResult.solutions[0])
		}
		rootFreeMasks[rootResult.initialFreeMask] = struct{}{}
		candidates += rootResult.candidates
		statesDeduplicated += rootResult.deduplicated
		hardPruned += rootResult.hardPruned
		symmetryPruned += rootResult.symmetryPruned
		if len(rootResult.solutions) > 0 {
			diagnostics.RootsCompleted++
			results = mergeSolutions(results, rootResult.solutions, config.TopN)
			best := rootResult.solutions[0]
			if rootWinnerID == "" || compareScores(best.Evaluation.Score, rootWinner.Evaluation.Score) > 0 || (compareScores(best.Evaluation.Score, rootWinner.Evaluation.Score) == 0 && root.rootID < rootWinnerID) {
				rootWinner = best
				rootWinnerID = root.rootID
			}
		}
		if config.Diagnostics {
			orbitKey := ""
			if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
				orbitKey = root.sourceGeometryOrbitKey
			}
			rootDiagnostic := model.ConstellationRootDiagnostic{
				ID:                        root.rootID,
				SkeletonID:                root.id,
				NodesReserved:             quota,
				NodesConsumed:             rootResult.nodes,
				PackingBeamWidth:          policy.ConstellationSeedPackingBeamWidth,
				LayerWidths:               append([]model.PackingSeedLayerWidth(nil), rootResult.layerWidths...),
				BeamEvictions:             rootResult.beamEvictions,
				HardDeadPruned:            rootResult.hardPruned,
				SymmetryPruned:            rootResult.symmetryPruned,
				StatesDeduplicated:        rootResult.deduplicated,
				Completed:                 len(rootResult.solutions) > 0,
				CandidateCount:            rootResult.candidates,
				SourceGeometryKey:         root.sourceGeometryKey,
				SourceGeometryOrbitKey:    orbitKey,
				InitialOccupiedMaskHex:    fmtMask(rootResult.initialOccupiedMask),
				InitialFreeMaskHex:        fmtMask(rootResult.initialFreeMask),
				InitialFreeCellCount:      bits.OnesCount64(rootResult.initialFreeMask),
				AnchoredInstanceIDs:       append([]string(nil), rootResult.anchoredInstanceIDs...),
				RemainingPackingOrder:     append([]string(nil), rootResult.remainingPackingOrder...),
				InitialRestricted:         rootResult.initialRestricted,
				InitialFlexibility:        rootResult.initialFlexibility,
				InitialFragmentation:      rootResult.initialFragmentation,
				TerminationReason:         rootResult.terminationReason,
				RootPackingInputKey:       rootResult.packingInputKey,
				PackingStrategy:           rootResult.packingStrategy,
				SelectionPolicy:           root.selectionPolicy,
				RelaxedFromExactKey:       root.relaxedFromExactKey,
				FrontierExactKey:          root.frontierExactKey,
				RelaxationFrontierSize:    root.relaxationFrontierSize,
				ParentGuardedFrontier:     rootResult.parentGuardedFrontier,
				FirstCompleteNodes:        rootResult.firstCompleteNodes,
				DistinctNextItemsSelected: rootResult.distinctNextItems,
				MRVDepths:                 append([]model.ConstellationRootPackingDepthDiagnostic(nil), rootResult.mrvDepths...),
			}
			if familyID != "" {
				rootDiagnostic.FamilyID = familyID
				rootDiagnostic.FamilyAllocationRounds = append([]model.ConstellationRootPackingAllocationRound(nil), rounds...)
				rootDiagnostic.FamilyTotalQuota = quota
				rootDiagnostic.FamilyTotalConsumed = rootResult.nodes
				rootDiagnostic.FamilyTotalReturned = familyReturned
				rootDiagnostic.FamilyTerminationReason = rootResult.terminationReason
			}
			if policy.ConstellationFeasibilityProbe {
				searchExhausted := probeResult.searchExhausted
				probeConsumed := probeResult.nodes
				probeReturned := probeAvailable - probeConsumed
				rootDiagnostic.ProbeNodesAvailable = &probeAvailable
				rootDiagnostic.ProbeNodesConsumed = &probeConsumed
				rootDiagnostic.ProbeNodesReturned = &probeReturned
				rootDiagnostic.ProbeTerminationReason = probeResult.terminationReason
				rootDiagnostic.FeasibilityStatus = probeResult.feasibilityStatus
				rootDiagnostic.SearchExhausted = &searchExhausted
				rootDiagnostic.WitnessHash = probeResult.witnessHash
			}
			if policy.ConstellationCompletionOptimizationProbe {
				eligible := optimizationProbeEligible
				searchExhausted := optimizationProbeResult.searchExhausted
				initialIncumbent := optimizationProbeResult.initialIncumbentFromRootPacking
				optimizationProbeConsumed := optimizationProbeResult.nodes
				optimizationProbeReturned := probeAvailable - optimizationProbeConsumed
				rootDiagnostic.ExactCompletionEligible = &eligible
				if !eligible {
					if !optimizationProbeTargeted {
						rootDiagnostic.ExactCompletionSkipReason = "not_targeted"
					} else {
						rootDiagnostic.ExactCompletionSkipReason = "root_not_completed"
					}
				} else {
					rootDiagnostic.ExactCompletionNodesAvailable = &probeAvailable
					rootDiagnostic.ExactCompletionNodesConsumed = &optimizationProbeConsumed
					rootDiagnostic.ExactCompletionNodesReturned = &optimizationProbeReturned
					rootDiagnostic.ExactCompletionStatus = optimizationProbeResult.status
					rootDiagnostic.ExactCompletionTerminationReason = optimizationProbeResult.terminationReason
					rootDiagnostic.ExactCompletionStopSource = optimizationProbeResult.stopSource
					rootDiagnostic.ExactCompletionSearchExhausted = &searchExhausted
					rootDiagnostic.ExactCompletionInitialIncumbentFromRootPacking = &initialIncumbent
					rootDiagnostic.ExactCompletionTerminalCompletions = optimizationProbeResult.terminalCompletions
					rootDiagnostic.ExactCompletionAreaPrunes = optimizationProbeResult.areaPrunes
					rootDiagnostic.ExactCompletionZeroDomainPrunes = optimizationProbeResult.zeroDomainPrunes
					rootDiagnostic.ExactCompletionTranspositionPrunes = optimizationProbeResult.transpositionPrunes
					if optimizationProbeResult.hasBest {
						score := cloneScore(optimizationProbeResult.bestScore)
						rootDiagnostic.ExactCompletionBestScore = &score
						rootDiagnostic.ExactCompletionBestLayoutKey = optimizationProbeResult.bestLayoutKey
						rootDiagnostic.ExactCompletionBestHash = optimizationProbeResult.bestHash
					}
				}
			}
			if len(rootResult.solutions) > 0 {
				best := rootResult.solutions[0]
				score := cloneScore(best.Evaluation.Score)
				rootDiagnostic.BestScore = &score
				rootDiagnostic.CandidateLayoutKey = best.LayoutKey
				rootDiagnostic.CandidateHash = best.CanonicalLayoutHash
			}
			diagnostics.Roots = append(diagnostics.Roots, rootDiagnostic)
		}
		return probeResult.nodes + optimizationProbeResult.nodes
	}
	if constellationSeedUsesProgressiveRootScheduler(policy.ConstellationSeedVariant) {
		schedule := constellationProgressiveRootPacking(catalog, instances, optionsByInstance, roots, config, gridMask, packingNodesLeft, reportNode, func() bool { return exhausted })
		diagnostics.RootPackingScheduler = &schedule.policy
		for index, family := range schedule.families {
			familyReturned := int64(0)
			for _, allocation := range family.rounds {
				familyReturned += allocation.Returned
			}
			recordRoot(index, family.root, family.nodesReserved, family.result, 0, family.familyID, family.rounds, familyReturned)
		}
		packingNodesLeft -= schedule.nodesConsumed
	} else {
		for index, root := range roots {
			if packingNodesLeft <= 0 || exhausted {
				break
			}
			quota := packingNodesLeft / int64(len(roots)-index)
			if quota <= 0 {
				break
			}
			var rootResult constellationRootPackingResult
			if frontier, exists := candidateSkeletons[root.frontierExactKey]; root.frontierExactKey != "" && exists {
				rootResult = constellationParentGuardedFrontierPackingSearch(catalog, instances, optionsByInstance, root, frontier, config, gridMask, quota, reportNode)
			} else {
				rootResult = constellationRootPackingSearch(catalog, instances, optionsByInstance, root, config, gridMask, quota, reportNode)
			}
			probeAvailable := quota - rootResult.nodes
			packingNodesLeft -= rootResult.nodes + recordRoot(index, root, quota, rootResult, probeAvailable, "", nil, 0)
		}
	}
	if rootWinnerID != "" {
		score := cloneScore(rootWinner.Evaluation.Score)
		diagnostics.ConstellationRootWinnerID = rootWinnerID
		diagnostics.ConstellationRootWinnerScore = &score
		diagnostics.ConstellationRootWinnerHash = rootWinner.CanonicalLayoutHash
	}
	if len(results) > 0 {
		score := cloneScore(results[0].Evaluation.Score)
		diagnostics.ConstellationSeedFinalScore = &score
		diagnostics.ConstellationSeedFinalHash = results[0].CanonicalLayoutHash
	}
	if config.constellationCandidateCompletionSnapshot != nil {
		config.constellationCandidateCompletionSnapshot.candidates = cloneConstellationSkeletons(candidateSkeletons)
		config.constellationCandidateCompletionSnapshot.selectedRoots = cloneConstellationSelectedRootOutcomes(selectedRootOutcomes)
		selectedSkeletons := make(map[string]constellationSkeleton, len(roots))
		for _, root := range roots {
			selectedSkeletons[root.exactKey] = root
		}
		config.constellationCandidateCompletionSnapshot.selectedSkeletons = cloneConstellationSkeletons(selectedSkeletons)
	}
	if config.Diagnostics && policy.ConstellationCandidateCompletionOptimizationProbe && config.ConstellationCandidateCompletionOptimizationNodeBudget == 0 {
		optimization := constellationCandidateCompletionOptimization(catalog, instances, optionsByInstance, candidateSkeletons, selectedRootOutcomes, config, gridMask, packingNodesLeft, reportCandidateOptimizationNode)
		for _, attempt := range optimization.Attempts {
			packingNodesLeft -= attempt.NodesConsumed
		}
		diagnostics.CandidateCompletionOptimization = &optimization
	}
	if config.Diagnostics && policy.ConstellationCandidatePoolFeasibilitySweep {
		sweep := constellationCandidatePoolFeasibilitySweep(catalog, instances, optionsByInstance, candidateSkeletons, selectedRootOutcomes, config, gridMask, packingNodesLeft, reportSweepNode)
		packingNodesLeft -= sweep.NodesConsumed
		diagnostics.CandidatePoolFeasibilitySweep = &sweep
	}
	diagnostics.PriorityConstellations = len(roots)
	diagnostics.PrioritySourceGeometryCount = len(sourceGeometryKeys(roots))
	if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
		diagnostics.SelectedPrioritySourceGeometryCount = diagnostics.PrioritySourceGeometryCount
		diagnostics.SelectedPrioritySourceGeometryOrbitCount = len(sourceGeometryOrbitKeys(roots))
	}
	diagnostics.RootFreeMaskCount = len(rootFreeMasks)
	flushProgress()
	return coverageSeedResult{
		Solutions:              results,
		NodesExplored:          nodes,
		CandidateCount:         candidates,
		SymmetryPrunedBranches: symmetryPruned,
		StatesDeduplicated:     statesDeduplicated,
		HardPrunedNodes:        hardPruned,
	}, diagnostics
}

func constellationSeedRuntimeEligible(catalog model.Catalog, instances []model.InventoryInstance, config Config) bool {
	if config.AllowSkips || config.priorityBounds == nil || len(config.priorityBounds.ceiling) != 2 {
		return false
	}
	sources, _ := constellationSources(catalog, instances, config.Priorities)
	return len(sources) > 0 && len(sources) <= 4
}

func constellationSources(catalog model.Catalog, instances []model.InventoryInstance, priorities []string) ([]model.InventoryInstance, map[string]struct{}) {
	priorityOrder := make(map[string]int, len(priorities))
	sourceItems := make(map[string]struct{}, len(priorities))
	for index, priority := range priorities {
		kind, itemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			continue
		}
		if _, exists := catalog.Items[itemID]; !exists {
			continue
		}
		if _, exists := priorityOrder[itemID]; !exists {
			priorityOrder[itemID] = index
		}
		sourceItems[itemID] = struct{}{}
	}
	sources := make([]model.InventoryInstance, 0, len(instances))
	for _, instance := range instances {
		if _, source := sourceItems[instance.ItemID]; source {
			sources = append(sources, instance)
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		leftOrder, rightOrder := priorityOrder[sources[i].ItemID], priorityOrder[sources[j].ItemID]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return sources[i].OriginalIndex < sources[j].OriginalIndex
	})
	return sources, sourceItems
}

func constellationTargets(instances []model.InventoryInstance, sources []model.InventoryInstance, potential *starPotentialContext, limit int) []model.InventoryInstance {
	sourceIDs := make(map[string]struct{}, len(sources))
	var targetMask uint64
	for _, source := range sources {
		sourceIDs[source.InstanceID] = struct{}{}
		targetMask |= potential.instanceOutgoingTargets[source.InstanceID]
	}
	type weightedTarget struct {
		instance model.InventoryInstance
		weight   int
	}
	weighted := make([]weightedTarget, 0, len(instances))
	for _, instance := range instances {
		if _, source := sourceIDs[instance.InstanceID]; source || targetMask&(uint64(1)<<uint(instance.OriginalIndex)) == 0 {
			continue
		}
		weight := 0
		for _, source := range sources {
			if potential.instanceOutgoingTargets[source.InstanceID]&(uint64(1)<<uint(instance.OriginalIndex)) != 0 {
				weight++
			}
		}
		weighted = append(weighted, weightedTarget{instance: instance, weight: weight})
	}
	sort.Slice(weighted, func(i, j int) bool {
		if weighted[i].weight != weighted[j].weight {
			return weighted[i].weight > weighted[j].weight
		}
		return weighted[i].instance.OriginalIndex < weighted[j].instance.OriginalIndex
	})
	targets := make([]model.InventoryInstance, len(weighted))
	for index, candidate := range weighted {
		targets[index] = candidate.instance
	}
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	return targets
}

func constellationOptions(options []model.Placement, potential *starPotentialContext, limit int) []model.Placement {
	selected := append([]model.Placement(nil), options...)
	sort.Slice(selected, func(i, j int) bool {
		leftPriority := potential.priorityForPlacement(selected[i])
		rightPriority := potential.priorityForPlacement(selected[j])
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		return placementKey(selected[i]) < placementKey(selected[j])
	})
	if limit > 0 && len(selected) > limit {
		// Keep high-value options while reserving half the cap for positions
		// farther down the deterministic order. Priority sources otherwise
		// collapse into one overlapping local geometry before targets appear.
		topCount := limit / 2
		if topCount < 1 {
			topCount = 1
		}
		out := append([]model.Placement(nil), selected[:topCount]...)
		remainingSlots := limit - len(out)
		if remainingSlots > 0 {
			stride := (len(selected) - topCount + remainingSlots - 1) / remainingSlots
			for index := topCount; len(out) < limit && index < len(selected); index += stride {
				out = append(out, selected[index])
			}
		}
		selected = out
	}
	return selected
}

func constellationOptionsForState(options []model.Placement, potential *starPotentialContext, limit int, occupied uint64) []model.Placement {
	available := make([]model.Placement, 0, len(options))
	for _, option := range options {
		if option.Mask&occupied == 0 {
			available = append(available, option)
		}
	}
	return constellationOptions(available, potential, limit)
}

func filterConstellationPriorityFeasibleStates(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	gridMask uint64,
	config Config,
	states []constellationSeedState,
) []constellationSeedState {
	if config.priorityBounds == nil {
		return states
	}
	filtered := states[:0]
	for _, state := range states {
		placed := placementByInstanceID(state.placed)
		remaining := make([]model.InventoryInstance, 0, len(instances)-len(state.placed))
		for _, instance := range instances {
			if _, exists := placed[instance.InstanceID]; !exists {
				remaining = append(remaining, instance)
			}
		}
		partial := partialRepairState{
			FixedPlacements:  state.placed,
			RemovedInstances: remaining,
			FreeCells:        gridMask &^ state.occupied,
		}
		upper := partialRepairV3PriorityUpperBound(catalog, partial, optionsByInstance, config.Priorities)
		if partialRepairTargetVectorFeasible(upper, config.priorityBounds.ceiling) {
			filtered = append(filtered, state)
		}
	}
	return filtered
}

func constellationSourceOptions(options []model.Placement, occupied uint64, limit int, _ model.InventoryInstance, _ []model.InventoryInstance) []model.Placement {
	selected := make([]model.Placement, 0, len(options))
	for _, option := range options {
		if option.Mask&occupied == 0 {
			selected = append(selected, option)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return placementKey(selected[i]) < placementKey(selected[j]) })
	if limit <= 0 || len(selected) <= limit {
		return selected
	}
	if limit == 1 {
		return selected[:1]
	}
	// Identical priority sources are assigned in canonical placement-key order.
	// Sample across the full order so early copies do not consume only the same
	// high-potential pocket and make later copies impossible to place.
	out := make([]model.Placement, 0, limit)
	for index := 0; index < limit; index++ {
		selectedIndex := index * (len(selected) - 1) / (limit - 1)
		out = append(out, selected[selectedIndex])
	}
	return out
}

func selectConstellationStates(states []constellationSeedState, policy ResolvedSearchPolicy, sourceItems map[string]struct{}, sourceCount int, catalog model.Catalog, config Config, deduplicated int64) ([]constellationSeedState, int64) {
	if len(states) == 0 {
		return nil, deduplicated
	}
	byExact := make(map[string]constellationSeedState, len(states))
	for _, state := range states {
		if existing, exists := byExact[state.exactKey]; exists {
			deduplicated++
			if !constellationStateLess(state, existing) {
				continue
			}
		}
		byExact[state.exactKey] = state
	}
	states = states[:0]
	for _, state := range byExact {
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return constellationStateLess(states[i], states[j]) })
	beamWidth := policy.ConstellationSeedBeamWidth
	if beamWidth <= 0 || len(states) <= beamWidth {
		return states, deduplicated
	}
	if constellationSeedUsesSourceGeometry(policy.ConstellationSeedVariant) && constellationStatesHaveAllSources(states, sourceItems, sourceCount) {
		if constellationSeedUsesOrbitDiversity(policy.ConstellationSeedVariant) {
			return selectConstellationV4States(states, beamWidth, policy.ConstellationSeedSourceGeometryBeamCount, sourceItems), deduplicated
		}
		return selectConstellationV2States(states, beamWidth, policy.ConstellationSeedSourceGeometryBeamCount, sourceItems), deduplicated
	}
	selected := make([]constellationSeedState, 0, beamWidth)
	seenSignature := make(map[string]struct{}, beamWidth)
	for _, state := range states {
		if !config.priorityBounds.reached(state.score) {
			continue
		}
		evaluation := evaluateLayoutForConfig(catalog, state.placed, config)
		signature := constellationSignature(state.placed, evaluation.Stars, sourceItems)
		if _, exists := seenSignature[signature]; exists {
			continue
		}
		seenSignature[signature] = struct{}{}
		selected = append(selected, state)
		if len(selected) >= beamWidth {
			return selected, deduplicated
		}
	}
	for _, state := range states {
		if len(selected) >= beamWidth {
			break
		}
		selected = append(selected, state)
	}
	return selected, deduplicated
}

func constellationStatesHaveAllSources(states []constellationSeedState, sourceItems map[string]struct{}, sourceCount int) bool {
	if sourceCount <= 0 || len(states) == 0 {
		return false
	}
	for _, state := range states {
		count := 0
		for _, placement := range state.placed {
			if _, source := sourceItems[placement.ItemID]; source {
				count++
			}
		}
		if count < sourceCount {
			return false
		}
	}
	return true
}

func selectConstellationV2States(states []constellationSeedState, beamWidth int, maxGeometries int, sourceItems map[string]struct{}) []constellationSeedState {
	if maxGeometries <= 0 {
		maxGeometries = 1
	}
	byGeometry := make(map[string][]constellationSeedState)
	geometryOrder := make([]string, 0, maxGeometries)
	for _, state := range states {
		key := constellationPrioritySourceGeometryKey(state.placed, sourceItems)
		if _, exists := byGeometry[key]; !exists {
			if len(geometryOrder) >= maxGeometries {
				continue
			}
			geometryOrder = append(geometryOrder, key)
		}
		byGeometry[key] = append(byGeometry[key], state)
	}
	if len(geometryOrder) == 0 {
		return states[:minInt(len(states), beamWidth)]
	}
	perGeometry := (beamWidth + len(geometryOrder) - 1) / len(geometryOrder)
	selected := make([]constellationSeedState, 0, beamWidth)
	for _, key := range geometryOrder {
		group := byGeometry[key]
		if len(group) > perGeometry {
			group = group[:perGeometry]
		}
		selected = append(selected, group...)
	}
	if len(selected) >= beamWidth {
		return selected[:beamWidth]
	}
	seen := make(map[string]struct{}, len(selected))
	for _, state := range selected {
		seen[state.exactKey] = struct{}{}
	}
	for _, state := range states {
		if len(selected) >= beamWidth {
			break
		}
		if _, exists := seen[state.exactKey]; exists {
			continue
		}
		selected = append(selected, state)
		seen[state.exactKey] = struct{}{}
	}
	return selected
}

// selectConstellationV4States keeps the V3 beam budget but reserves the first
// slots for distinct top-bottom source-geometry orbits before raw geometries.
func selectConstellationV4States(states []constellationSeedState, beamWidth int, maxOrbits int, sourceItems map[string]struct{}) []constellationSeedState {
	if maxOrbits <= 0 {
		maxOrbits = 1
	}
	if maxOrbits > beamWidth {
		maxOrbits = beamWidth
	}
	selected := make([]constellationSeedState, 0, beamWidth)
	seenExact := make(map[string]struct{}, beamWidth)
	seenOrbits := make(map[string]struct{}, maxOrbits)
	for _, state := range states {
		if len(seenOrbits) >= maxOrbits {
			break
		}
		orbitKey := constellationPrioritySourceGeometryOrbitKey(state.placed, sourceItems)
		if _, exists := seenOrbits[orbitKey]; exists {
			continue
		}
		selected = append(selected, state)
		seenExact[state.exactKey] = struct{}{}
		seenOrbits[orbitKey] = struct{}{}
	}
	seenGeometries := make(map[string]struct{}, len(selected))
	for _, state := range selected {
		seenGeometries[constellationPrioritySourceGeometryKey(state.placed, sourceItems)] = struct{}{}
	}
	for _, state := range states {
		if len(selected) >= beamWidth {
			break
		}
		if _, exists := seenExact[state.exactKey]; exists {
			continue
		}
		geometryKey := constellationPrioritySourceGeometryKey(state.placed, sourceItems)
		if _, exists := seenGeometries[geometryKey]; exists {
			continue
		}
		selected = append(selected, state)
		seenExact[state.exactKey] = struct{}{}
		seenGeometries[geometryKey] = struct{}{}
	}
	for _, state := range states {
		if len(selected) >= beamWidth {
			break
		}
		if _, exists := seenExact[state.exactKey]; exists {
			continue
		}
		selected = append(selected, state)
		seenExact[state.exactKey] = struct{}{}
	}
	return selected
}

func constellationStateLess(left, right constellationSeedState) bool {
	if compare := comparePriorityCounts(left.score.PriorityCounts, right.score.PriorityCounts); compare != 0 {
		return compare > 0
	}
	if left.score.StarCount != right.score.StarCount {
		return left.score.StarCount > right.score.StarCount
	}
	if left.potential != right.potential {
		return left.potential > right.potential
	}
	if len(left.placed) != len(right.placed) {
		return len(left.placed) > len(right.placed)
	}
	return left.exactKey < right.exactKey
}

func constellationExactKey(placements []model.Placement) string {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		parts = append(parts, placement.InstanceID+"|"+placementKey(placement))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func constellationSignature(placements []model.Placement, stars []model.StarActivation, sourceItems map[string]struct{}) string {
	physical := physicalInstanceIDs(placements)
	placedByID := placementByInstanceID(placements)
	parts := make([]string, 0, len(placements)+len(stars))
	for _, placement := range placements {
		if _, source := sourceItems[placement.ItemID]; source {
			parts = append(parts, "source|"+placement.ItemID+"|"+canonicalPlacementKey(placement))
		}
	}
	for _, star := range stars {
		source, exists := placedByID[star.SourceInstance]
		if !exists {
			continue
		}
		if _, sourcePriority := sourceItems[source.ItemID]; !sourcePriority {
			continue
		}
		parts = append(parts, "link|"+physical[star.SourceInstance]+">"+physical[star.TargetInstance]+"@"+strconv.Itoa(star.StarPosition.Row)+","+strconv.Itoa(star.StarPosition.Col))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func constellationPrioritySourceGeometryKey(placements []model.Placement, sourceItems map[string]struct{}) string {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		if _, source := sourceItems[placement.ItemID]; source {
			parts = append(parts, canonicalPlacementKey(placement))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

// constellationPrioritySourceGeometryOrbitKey is a diversity label, not a
// legal placement transform. It canonicalizes only identity and top-bottom
// reflection of priority-source cells and stars.
func constellationPrioritySourceGeometryOrbitKey(placements []model.Placement, sourceItems map[string]struct{}) string {
	identity := constellationPrioritySourceGeometryKey(placements, sourceItems)
	reflected := constellationReflectedPrioritySourceGeometryKey(placements, sourceItems)
	if reflected < identity {
		return reflected
	}
	return identity
}

func constellationReflectedPrioritySourceGeometryKey(placements []model.Placement, sourceItems map[string]struct{}) string {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		if _, source := sourceItems[placement.ItemID]; !source {
			continue
		}
		reflected := model.Placement{ItemID: placement.ItemID}
		reflected.Cells = make([]model.Coord, 0, len(placement.Cells))
		for _, cell := range placement.Cells {
			reflected.Cells = append(reflected.Cells, topBottomReflectedCoord(cell))
		}
		reflected.StarPositions = make([]model.StarPosition, 0, len(placement.StarPositions))
		for _, star := range placement.StarPositions {
			reflected.StarPositions = append(reflected.StarPositions, model.StarPosition{Position: topBottomReflectedCoord(star.Position)})
		}
		parts = append(parts, canonicalPlacementKey(reflected))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func topBottomReflectedCoord(coord model.Coord) model.Coord {
	return model.Coord{Row: geometry.GridRows - 1 - coord.Row, Col: coord.Col}
}

func topBottomReflectedMask(mask uint64) uint64 {
	const rowMask = (uint64(1) << geometry.GridCols) - 1
	var reflected uint64
	for row := 0; row < geometry.GridRows; row++ {
		rowBits := (mask >> uint(row*geometry.GridCols)) & rowMask
		reflected |= rowBits << uint((geometry.GridRows-1-row)*geometry.GridCols)
	}
	return reflected
}

func topBottomMaskOrbitKey(mask uint64) string {
	identity := fmtMask(mask)
	reflected := fmtMask(topBottomReflectedMask(mask))
	if reflected < identity {
		return reflected
	}
	return identity
}

func constellationTargetAssignmentKey(placements []model.Placement, stars []model.StarActivation, sourceItems map[string]struct{}) string {
	physical := physicalInstanceIDs(placements)
	placedByID := placementByInstanceID(placements)
	parts := make([]string, 0, len(stars))
	for _, star := range stars {
		source, exists := placedByID[star.SourceInstance]
		if !exists {
			continue
		}
		if _, priority := sourceItems[source.ItemID]; !priority {
			continue
		}
		parts = append(parts, physical[star.SourceInstance]+">"+physical[star.TargetInstance]+"@"+strconv.Itoa(star.StarPosition.Row)+","+strconv.Itoa(star.StarPosition.Col))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func constellationSkeletonLess(left constellationSkeleton, right constellationSkeleton) bool {
	if compare := compareScores(left.score, right.score); compare != 0 {
		return compare > 0
	}
	if left.signature != right.signature {
		return left.signature < right.signature
	}
	return left.exactKey < right.exactKey
}

func retainConstellationV2Skeleton(skeletons map[string]constellationSkeleton, candidate constellationSkeleton, maxGeometries int) {
	if maxGeometries <= 0 {
		return
	}
	if existing, exists := skeletons[candidate.sourceGeometryKey]; exists {
		if constellationSkeletonLess(candidate, existing) {
			skeletons[candidate.sourceGeometryKey] = candidate
		}
		return
	}
	if len(skeletons) < maxGeometries {
		skeletons[candidate.sourceGeometryKey] = candidate
		return
	}
	var worstKey string
	var worst constellationSkeleton
	for key, existing := range skeletons {
		if worstKey == "" || constellationSkeletonLess(worst, existing) || (!constellationSkeletonLess(existing, worst) && !constellationSkeletonLess(worst, existing) && key > worstKey) {
			worstKey = key
			worst = existing
		}
	}
	if constellationSkeletonLess(candidate, worst) {
		delete(skeletons, worstKey)
		skeletons[candidate.sourceGeometryKey] = candidate
	}
}

func selectConstellationV4Skeletons(candidates map[string]constellationSkeleton, maxSkeletons int) map[string]constellationSkeleton {
	if maxSkeletons <= 0 || len(candidates) == 0 {
		return map[string]constellationSkeleton{}
	}
	ordered := make([]constellationSkeleton, 0, len(candidates))
	for _, candidate := range candidates {
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool { return constellationSkeletonLess(ordered[i], ordered[j]) })
	selected := make(map[string]constellationSkeleton, maxSkeletons)
	seenExact := make(map[string]struct{}, maxSkeletons)
	seenOrbits := make(map[string]struct{}, maxSkeletons)
	appendCandidate := func(candidate constellationSkeleton) {
		selected[candidate.exactKey] = candidate
		seenExact[candidate.exactKey] = struct{}{}
	}
	for _, candidate := range ordered {
		if len(selected) >= maxSkeletons {
			break
		}
		orbitKey := constellationSkeletonOrbitKey(candidate)
		if _, exists := seenOrbits[orbitKey]; exists {
			continue
		}
		appendCandidate(candidate)
		seenOrbits[orbitKey] = struct{}{}
	}
	seenGeometries := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		seenGeometries[candidate.sourceGeometryKey] = struct{}{}
	}
	for _, candidate := range ordered {
		if len(selected) >= maxSkeletons {
			break
		}
		if _, exists := seenExact[candidate.exactKey]; exists {
			continue
		}
		if _, exists := seenGeometries[candidate.sourceGeometryKey]; exists {
			continue
		}
		appendCandidate(candidate)
		seenGeometries[candidate.sourceGeometryKey] = struct{}{}
	}
	seenAssignments := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		seenAssignments[candidate.targetAssignmentKey] = struct{}{}
	}
	for _, candidate := range ordered {
		if len(selected) >= maxSkeletons {
			break
		}
		if _, exists := seenExact[candidate.exactKey]; exists {
			continue
		}
		if _, exists := seenAssignments[candidate.targetAssignmentKey]; exists {
			continue
		}
		appendCandidate(candidate)
		seenAssignments[candidate.targetAssignmentKey] = struct{}{}
	}
	for _, candidate := range ordered {
		if len(selected) >= maxSkeletons {
			break
		}
		if _, exists := seenExact[candidate.exactKey]; exists {
			continue
		}
		appendCandidate(candidate)
	}
	return selected
}

// selectConstellationV5Skeletons preserves V4's four-root orbit-diverse set,
// then replaces at most one strict representative with the maximally relaxed
// representative from the same priority constellation family.
func selectConstellationV5Skeletons(candidates map[string]constellationSkeleton, maxSkeletons int) map[string]constellationSkeleton {
	selected := selectConstellationV4Skeletons(candidates, maxSkeletons)
	if len(selected) == 0 {
		return selected
	}
	strict := make([]constellationSkeleton, 0, len(selected))
	for _, candidate := range selected {
		candidate.selectionPolicy = "v5_v4_orbit_representative"
		selected[candidate.exactKey] = candidate
		strict = append(strict, candidate)
	}
	sort.Slice(strict, func(i, j int) bool { return constellationSkeletonLess(strict[i], strict[j]) })
	for _, candidate := range strict {
		frontier := constellationRelaxationFrontier(candidates, candidate)
		if len(frontier) == 0 {
			continue
		}
		relaxed := frontier[0]
		relaxed.selectionPolicy = "v5_relaxation_frontier"
		relaxed.relaxedFromExactKey = candidate.exactKey
		relaxed.relaxationFrontierSize = len(frontier)
		delete(selected, candidate.exactKey)
		selected[relaxed.exactKey] = relaxed
		return selected
	}
	return selected
}

// selectConstellationV51Skeletons keeps the strict V4 representative in its
// allocation slot and attaches one maximally relaxed family member. The later
// slot executor gives both members the same frozen quota envelope.
func selectConstellationV51Skeletons(candidates map[string]constellationSkeleton, maxSkeletons int) map[string]constellationSkeleton {
	selected := selectConstellationV4Skeletons(candidates, maxSkeletons)
	if len(selected) == 0 {
		return selected
	}
	strict := make([]constellationSkeleton, 0, len(selected))
	for _, candidate := range selected {
		candidate.selectionPolicy = "v5.1_v4_orbit_representative"
		selected[candidate.exactKey] = candidate
		strict = append(strict, candidate)
	}
	sort.Slice(strict, func(i, j int) bool { return constellationSkeletonLess(strict[i], strict[j]) })
	for _, candidate := range strict {
		frontier := constellationRelaxationFrontier(candidates, candidate)
		if len(frontier) == 0 {
			continue
		}
		guarded := candidate
		guarded.selectionPolicy = "parent_guarded_frontier"
		guarded.frontierExactKey = frontier[0].exactKey
		guarded.relaxationFrontierSize = len(frontier)
		selected[guarded.exactKey] = guarded
		return selected
	}
	return selected
}

// constellationRelaxationFrontier returns the minimal anchor sets A for a
// strict root B. Family membership preserves source geometry, target
// assignment, priority counts, and stars; every A anchor must be an identical
// placement in B.
func constellationRelaxationFrontier(candidates map[string]constellationSkeleton, strict constellationSkeleton) []constellationSkeleton {
	frontier := make([]constellationSkeleton, 0)
	for _, candidate := range candidates {
		if constellationSkeletonStrictlyRelaxes(candidate, strict) {
			frontier = append(frontier, candidate)
		}
	}
	minimal := make([]constellationSkeleton, 0, len(frontier))
	for _, candidate := range frontier {
		isMinimal := true
		for _, other := range frontier {
			if constellationSkeletonStrictlyRelaxes(other, candidate) {
				isMinimal = false
				break
			}
		}
		if isMinimal {
			minimal = append(minimal, candidate)
		}
	}
	sort.Slice(minimal, func(i, j int) bool { return constellationSkeletonLess(minimal[i], minimal[j]) })
	return minimal
}

func constellationSkeletonStrictlyRelaxes(relaxed constellationSkeleton, strict constellationSkeleton) bool {
	if relaxed.sourceGeometryKey != strict.sourceGeometryKey || relaxed.targetAssignmentKey != strict.targetAssignmentKey || relaxed.score.StarCount != strict.score.StarCount || !samePriorityCounts(relaxed.score.PriorityCounts, strict.score.PriorityCounts) || len(relaxed.placed) >= len(strict.placed) {
		return false
	}
	strictAnchors := make(map[string]string, len(strict.placed))
	for _, placement := range strict.placed {
		strictAnchors[placement.InstanceID] = placementKey(placement)
	}
	for _, placement := range relaxed.placed {
		if strictAnchors[placement.InstanceID] != placementKey(placement) {
			return false
		}
	}
	return true
}

func samePriorityCounts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func constellationSkeletonOrbitKey(skeleton constellationSkeleton) string {
	if skeleton.sourceGeometryOrbitKey != "" {
		return skeleton.sourceGeometryOrbitKey
	}
	return skeleton.sourceGeometryKey
}

func constellationCompletionOptimizationProbeTargetsRoot(policy ResolvedSearchPolicy, rootIndex int) bool {
	switch policy.ConstellationCompletionOptimizationProbeScope {
	case constellationOptimizationScopeAll:
		return true
	case constellationOptimizationScopeFirstTwo:
		return rootIndex < 2
	default:
		return false
	}
}

func sourceGeometryKeys(roots []constellationSkeleton) map[string]struct{} {
	keys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		keys[root.sourceGeometryKey] = struct{}{}
	}
	return keys
}

func sourceGeometryOrbitKeys(roots []constellationSkeleton) map[string]struct{} {
	keys := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		keys[constellationSkeletonOrbitKey(root)] = struct{}{}
	}
	return keys
}

func candidateSourceGeometryKeys(candidates map[string]constellationSkeleton) map[string]struct{} {
	keys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		keys[candidate.sourceGeometryKey] = struct{}{}
	}
	return keys
}

func candidateSourceGeometryOrbitKeys(candidates map[string]constellationSkeleton) map[string]struct{} {
	keys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		keys[constellationSkeletonOrbitKey(candidate)] = struct{}{}
	}
	return keys
}

func candidateRootFreeMasks(candidates map[string]constellationSkeleton, gridMask uint64) map[uint64]struct{} {
	masks := make(map[uint64]struct{}, len(candidates))
	for _, candidate := range candidates {
		masks[gridMask&^candidate.occupied] = struct{}{}
	}
	return masks
}

func candidateRootFreeMaskOrbitKeys(candidates map[string]constellationSkeleton, gridMask uint64) map[string]struct{} {
	keys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		keys[topBottomMaskOrbitKey(gridMask&^candidate.occupied)] = struct{}{}
	}
	return keys
}

func constellationRootPackingSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func(bool) bool,
) constellationRootPackingResult {
	if policyForConfig(config).ConstellationSeedPackingStrategy == constellationPackingStrategyStateMRV {
		return constellationRootPackingMRV(catalog, instances, optionsByInstance, root, config, gridMask, nodeBudget, reportNode)
	}
	initialOccupied := root.occupied
	initialFree := gridMask &^ initialOccupied
	anchoredIDs := make([]string, 0, len(root.placed))
	for _, placement := range root.placed {
		anchoredIDs = append(anchoredIDs, placement.InstanceID)
	}
	sort.Strings(anchoredIDs)
	if nodeBudget <= 0 {
		return constellationRootPackingResult{
			initialOccupiedMask: initialOccupied,
			initialFreeMask:     initialFree,
			anchoredInstanceIDs: anchoredIDs,
			terminationReason:   "no_budget",
		}
	}
	placedByID := placementByInstanceID(root.placed)
	remaining := make([]model.InventoryInstance, 0, len(instances)-len(root.placed))
	for _, instance := range instances {
		if _, placed := placedByID[instance.InstanceID]; !placed {
			remaining = append(remaining, instance)
		}
	}
	if len(remaining) == 0 {
		evaluation := evaluateLayoutForConfig(catalog, root.placed, config)
		return constellationRootPackingResult{solutions: []model.Solution{{
			Placements:          append([]model.Placement(nil), root.placed...),
			Evaluation:          evaluation,
			LayoutKey:           layoutKey(root.placed, instances),
			CanonicalLayoutHash: canonicalLayoutHash(root.placed),
		}}, candidates: 1,
			initialOccupiedMask: initialOccupied,
			initialFreeMask:     initialFree,
			anchoredInstanceIDs: anchoredIDs,
			terminationReason:   "completed"}
	}
	remaining = packingSeedOrder(remaining, optionsByInstance)
	remainingOrder := make([]string, 0, len(remaining))
	for _, instance := range remaining {
		remainingOrder = append(remainingOrder, instance.InstanceID)
	}
	beamWidth := policyForConfig(config).ConstellationSeedPackingBeamWidth
	if config.forcedRootPackingReplay && config.ConstellationForcedCandidateRootedPackingBeamWidth > 0 {
		beamWidth = config.ConstellationForcedCandidateRootedPackingBeamWidth
	}
	if beamWidth <= 0 {
		beamWidth = policyForConfig(config).PackingSeedBeamWidth
	}
	initialRestricted, initialFlexibility, feasible := packingFeasibility(remaining, optionsByInstance, root.occupied, root.placed)
	initialFragmentation := freeSpaceFragmentation(gridMask, root.occupied)
	packingInputKey := constellationRootPackingInputKey(root.sourceGeometryKey, initialOccupied, initialFree, anchoredIDs, remainingOrder)
	if !feasible {
		return constellationRootPackingResult{
			hardPruned:            1,
			initialOccupiedMask:   initialOccupied,
			initialFreeMask:       initialFree,
			anchoredInstanceIDs:   anchoredIDs,
			remainingPackingOrder: remainingOrder,
			initialRestricted:     initialRestricted,
			initialFlexibility:    initialFlexibility,
			initialFragmentation:  initialFragmentation,
			terminationReason:     "hard_dead",
			packingInputKey:       packingInputKey,
		}
	}
	states := []packingSeedState{{
		occupied:      root.occupied,
		placed:        append([]model.Placement(nil), root.placed...),
		restricted:    initialRestricted,
		flexibility:   initialFlexibility,
		fragmentation: initialFragmentation,
		score:         root.score,
		key:           root.signature,
	}}
	remainingCells := remainingCellCounts(catalog, remaining)
	var nodes int64
	var deduplicated int64
	var hardPruned int64
	var symmetryPruned int64
	var beamEvictions int64
	var layerWidths []model.PackingSeedLayerWidth
	exhausted := false
	for index, instance := range remaining {
		if exhausted || len(states) == 0 {
			break
		}
		nextByClass := make(map[string]packingSeedState, beamWidth*4)
		for _, state := range states {
			if exhausted || nodes >= nodeBudget {
				exhausted = true
				break
			}
			if remainingCells[index] > bits.OnesCount64(gridMask&^state.occupied) {
				hardPruned++
				continue
			}
			for _, option := range optionsByInstance[instance.InstanceID] {
				if option.Mask&state.occupied != 0 {
					continue
				}
				if !placementRespectsCanonicalCopyOrder(option, state.placed) {
					symmetryPruned++
					continue
				}
				if !reportNode(false) {
					exhausted = true
					break
				}
				nodes++
				nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				nextOccupied := state.occupied | option.Mask
				restricted, flexibility, nextFeasible := packingFeasibility(remaining[index+1:], optionsByInstance, nextOccupied, nextPlaced)
				if !nextFeasible {
					hardPruned++
					continue
				}
				candidate := packingSeedState{
					occupied:      nextOccupied,
					placed:        nextPlaced,
					restricted:    restricted,
					flexibility:   flexibility,
					fragmentation: freeSpaceFragmentation(gridMask, nextOccupied),
					score:         evaluateScoreForConfig(catalog, nextPlaced, config),
					key:           root.signature + "|" + coverageSeedAppendKey(state.key, option),
				}
				classKey := root.signature + "|" + packingStateClassKey(candidate)
				if previous, exists := nextByClass[classKey]; exists {
					deduplicated++
					if !constellationRootPackingStateLess(config, candidate, previous) {
						continue
					}
				}
				nextByClass[classKey] = candidate
				if nodes >= nodeBudget {
					exhausted = true
					break
				}
			}
		}
		next := make([]packingSeedState, 0, len(nextByClass))
		for _, state := range nextByClass {
			next = append(next, state)
		}
		sort.Slice(next, func(i, j int) bool { return constellationRootPackingStateLess(config, next[i], next[j]) })
		if len(next) > beamWidth {
			beamEvictions += int64(len(next) - beamWidth)
			next = next[:beamWidth]
		}
		states = next
		layerWidths = append(layerWidths, model.PackingSeedLayerWidth{Depth: index + 1, States: len(states)})
	}
	results := make([]model.Solution, 0, config.TopN)
	candidates := 0
	for _, state := range states {
		if len(state.placed) != len(instances) {
			continue
		}
		candidates++
		results = collectConstellationRootPackingSolution(catalog, results, state.placed, instances, root.rootID, config)
	}
	allComplete := len(states) > 0
	for _, state := range states {
		if len(state.placed) != len(instances) {
			allComplete = false
			break
		}
	}
	terminationReason := "completed"
	if exhausted && !allComplete {
		terminationReason = "budget_exhausted"
	} else if len(states) == 0 {
		terminationReason = "no_states"
	}
	return constellationRootPackingResult{
		solutions:             results,
		nodes:                 nodes,
		candidates:            candidates,
		deduplicated:          deduplicated,
		hardPruned:            hardPruned,
		symmetryPruned:        symmetryPruned,
		beamEvictions:         beamEvictions,
		layerWidths:           layerWidths,
		initialOccupiedMask:   initialOccupied,
		initialFreeMask:       initialFree,
		anchoredInstanceIDs:   anchoredIDs,
		remainingPackingOrder: remainingOrder,
		initialRestricted:     initialRestricted,
		initialFlexibility:    initialFlexibility,
		initialFragmentation:  initialFragmentation,
		terminationReason:     terminationReason,
		packingInputKey:       packingInputKey,
	}
}

// constellationParentGuardedFrontierPackingSearch executes two independent
// rooted packings inside one normal allocation slot. The strict parent runs
// first; the relaxed frontier receives only the parent's returned quota.
func constellationParentGuardedFrontierPackingSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	parent constellationSkeleton,
	frontier constellationSkeleton,
	config Config,
	gridMask uint64,
	quota int64,
	reportNode func(bool) bool,
) constellationRootPackingResult {
	if !constellationSkeletonStrictlyRelaxes(frontier, parent) {
		return constellationRootPackingSearch(catalog, instances, optionsByInstance, parent, config, gridMask, quota, reportNode)
	}
	policy := policyForConfig(config)
	parentConfig := config
	parentConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false}
	parentResult := constellationRootPackingSearch(catalog, instances, optionsByInstance, parent, parentConfig, gridMask, quota, reportNode)
	frontierQuota := quota - parentResult.nodes
	if frontierQuota < 0 {
		frontierQuota = 0
	}
	frontierResult := constellationRootPackingResult{}
	var frontierSolutions []model.Solution
	if frontierQuota > 0 {
		frontierConfig := config
		frontierConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false}
		frontierResult = constellationRootPackingSearch(catalog, instances, optionsByInstance, frontier, frontierConfig, gridMask, frontierQuota, reportNode)
		frontierSolutions = frontierResult.solutions
	}
	attempt := model.ConstellationParentFrontierHedgeAttempt{
		FamilySlotID:         parent.rootID,
		SlotCount:            policy.ConstellationSeedMaxSkeletons,
		FamilyMemberCount:    2,
		RootMemberExecutions: 1,
		ParentExactKey:       parent.exactKey,
		FrontierExactKey:     frontier.exactKey,
		TotalQuota:           quota,
		Parent:               constellationParentFrontierHedgeMember(policy, quota),
	}
	attempt.Parent = constellationParentFrontierHedgeMemberFromResult(attempt.Parent, parentResult, quota)
	attempt.Frontier = constellationParentFrontierHedgeMember(policy, frontierQuota)
	attempt.Frontier.ResidualFractionBps = fractionBps(frontierQuota, quota)
	if frontierQuota == 0 {
		attempt.Frontier.SkippedReason = "no_residual_quota"
	} else {
		attempt.Frontier = constellationParentFrontierHedgeMemberFromResult(attempt.Frontier, frontierResult, frontierQuota)
		attempt.Frontier.ResidualFractionBps = fractionBps(frontierQuota, quota)
		attempt.RootMemberExecutions = 2
	}
	attempt.FamilyConsumed = attempt.Parent.Consumed + attempt.Frontier.Consumed
	attempt.FamilyReturned = quota - attempt.FamilyConsumed
	if attempt.FamilyReturned < 0 {
		attempt.FamilyReturned = 0
	}
	constellationParentFrontierFamilyBest(&attempt, parentResult.solutions, frontierSolutions)
	results := mergeSolutions(parentResult.solutions, frontierSolutions, config.TopN)
	if config.constellationRootOrigins != nil {
		for _, solution := range results {
			config.constellationRootOrigins[solution.CanonicalLayoutHash] = parent.rootID
		}
	}
	combined := parentResult
	combined.solutions = results
	combined.nodes = attempt.FamilyConsumed
	combined.candidates = parentResult.candidates + frontierResult.candidates
	combined.deduplicated = parentResult.deduplicated + frontierResult.deduplicated
	combined.hardPruned = parentResult.hardPruned + frontierResult.hardPruned
	combined.symmetryPruned = parentResult.symmetryPruned + frontierResult.symmetryPruned
	combined.beamEvictions = parentResult.beamEvictions + frontierResult.beamEvictions
	combined.firstCompleteNodes = parentResult.firstCompleteNodes
	if combined.firstCompleteNodes == 0 && frontierResult.firstCompleteNodes > 0 {
		combined.firstCompleteNodes = parentResult.nodes + frontierResult.firstCompleteNodes
	}
	combined.distinctNextItems = parentResult.distinctNextItems + frontierResult.distinctNextItems
	if frontierQuota > 0 {
		combined.layerWidths = frontierResult.layerWidths
		combined.mrvDepths = frontierResult.mrvDepths
	}
	if len(results) > 0 {
		combined.terminationReason = "completed"
	} else if frontierQuota == 0 {
		combined.terminationReason = parentResult.terminationReason
	} else {
		combined.terminationReason = frontierResult.terminationReason
	}
	combined.parentGuardedFrontier = &attempt
	return combined
}

// constellationRootPackingMRVLegacy preserves the pre-session implementation
// as a local reference while PackingSession is introduced.
func constellationRootPackingMRVLegacy(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func(bool) bool,
) constellationRootPackingResult {
	initialOccupied := root.occupied
	initialFree := gridMask &^ initialOccupied
	anchoredIDs := make([]string, 0, len(root.placed))
	for _, placement := range root.placed {
		anchoredIDs = append(anchoredIDs, placement.InstanceID)
	}
	sort.Strings(anchoredIDs)
	result := constellationRootPackingResult{
		initialOccupiedMask: initialOccupied,
		initialFreeMask:     initialFree,
		anchoredInstanceIDs: anchoredIDs,
		packingStrategy:     constellationPackingStrategyStateMRV,
	}
	var shadow *constellationShadowReference
	if config.constellationRootPackingCollector != nil {
		shadow = config.constellationRootPackingCollector.shadow
	}
	if nodeBudget <= 0 {
		result.terminationReason = "no_budget"
		return result
	}

	placedByID := placementByInstanceID(root.placed)
	remaining := make([]model.InventoryInstance, 0, len(instances)-len(root.placed))
	for _, instance := range instances {
		if _, placed := placedByID[instance.InstanceID]; !placed {
			remaining = append(remaining, instance)
		}
	}
	if len(remaining) == 0 {
		evaluation := evaluateLayoutForConfig(catalog, root.placed, config)
		result.solutions = []model.Solution{{
			Placements:          append([]model.Placement(nil), root.placed...),
			Evaluation:          evaluation,
			LayoutKey:           layoutKey(root.placed, instances),
			CanonicalLayoutHash: canonicalLayoutHash(root.placed),
		}}
		result.candidates = 1
		result.terminationReason = "completed"
		return result
	}
	remaining = packingSeedOrder(remaining, optionsByInstance)
	remainingOrder := make([]string, 0, len(remaining))
	var remainingMask uint64
	remainingArea := 0
	for _, instance := range remaining {
		remainingOrder = append(remainingOrder, instance.InstanceID)
		remainingMask |= uint64(1) << uint(instance.OriginalIndex)
		remainingArea += len(catalog.Items[instance.ItemID].Shape)
	}
	beamWidth := policyForConfig(config).ConstellationSeedPackingBeamWidth
	if config.forcedRootPackingReplay && config.ConstellationForcedCandidateRootedPackingBeamWidth > 0 {
		beamWidth = config.ConstellationForcedCandidateRootedPackingBeamWidth
	}
	if beamWidth <= 0 {
		beamWidth = policyForConfig(config).PackingSeedBeamWidth
	}
	initialRestricted, initialFlexibility, feasible := packingFeasibility(remaining, optionsByInstance, root.occupied, root.placed)
	initialFragmentation := freeSpaceFragmentation(gridMask, root.occupied)
	packingInputKey := constellationRootPackingInputKey(root.sourceGeometryKey, initialOccupied, initialFree, anchoredIDs, remainingOrder)
	result.remainingPackingOrder = remainingOrder
	result.initialRestricted = initialRestricted
	result.initialFlexibility = initialFlexibility
	result.initialFragmentation = initialFragmentation
	result.packingInputKey = packingInputKey
	if !feasible {
		result.hardPruned = 1
		result.terminationReason = "hard_dead"
		return result
	}

	states := []constellationRootMRVState{{
		packingSeedState: packingSeedState{
			occupied:      root.occupied,
			placed:        append([]model.Placement(nil), root.placed...),
			restricted:    initialRestricted,
			flexibility:   initialFlexibility,
			fragmentation: initialFragmentation,
			score:         root.score,
			key:           root.signature,
		},
		remainingMask: remainingMask,
		remainingArea: remainingArea,
	}}
	var nodes int64
	var deduplicated int64
	var hardPruned int64
	var symmetryPruned int64
	var beamEvictions int64
	var layerWidths []model.PackingSeedLayerWidth
	var mrvDepths []model.ConstellationRootPackingDepthDiagnostic
	selectedItemIDs := make(map[string]struct{})
	firstCompleteNodes := int64(0)
	exhausted := false
	for depth := 1; depth <= len(remaining); depth++ {
		if exhausted || len(states) == 0 {
			break
		}
		depthDiagnostic := model.ConstellationRootPackingDepthDiagnostic{
			Depth:                     depth,
			SelectedInstanceHistogram: make(map[string]int64),
			StatesBeforeExpansion:     len(states),
		}
		var shadowDepth *model.ConstellationForcedCandidateShadowDepth
		if shadow != nil {
			compatibleBefore := 0
			for _, state := range states {
				if shadow.compatible(state.placed) {
					compatibleBefore++
				}
			}
			shadowDepth = &model.ConstellationForcedCandidateShadowDepth{
				Depth:                         depth,
				StatesBeforeExpansion:         len(states),
				StatesBeforeWitnessCompatible: compatibleBefore,
			}
		}
		nextByClass := make(map[string]constellationRootMRVState, beamWidth*4)
		for _, state := range states {
			if exhausted || nodes >= nodeBudget {
				exhausted = true
				break
			}
			if state.remainingArea > bits.OnesCount64(gridMask&^state.occupied) {
				hardPruned++
				continue
			}
			selectedIndex, selectedOptions, selected := constellationRootMRVSelection(remaining, state.remainingMask, optionsByInstance, state.occupied, state.placed)
			if !selected {
				hardPruned++
				depthDiagnostic.ZeroDomainPrunes++
				continue
			}
			selectedInstance := remaining[selectedIndex]
			if shadowDepth != nil {
				shadowDepth.ShadowSymmetryPruned += constellationShadowSymmetryPrunedOptions(selectedInstance, optionsByInstance[selectedInstance.InstanceID], state.occupied, state.placed, shadow)
			}
			selectedLegal := len(selectedOptions)
			depthDiagnostic.SelectedInstanceHistogram[selectedInstance.InstanceID]++
			if depthDiagnostic.MinLegalPlacements == 0 || selectedLegal < depthDiagnostic.MinLegalPlacements {
				depthDiagnostic.MinLegalPlacements = selectedLegal
			}
			if selectedLegal > depthDiagnostic.MaxLegalPlacements {
				depthDiagnostic.MaxLegalPlacements = selectedLegal
			}
			selectedItemIDs[selectedInstance.ItemID] = struct{}{}
			nextRemainingMask := state.remainingMask &^ (uint64(1) << uint(selectedInstance.OriginalIndex))
			nextRemainingArea := state.remainingArea - len(catalog.Items[selectedInstance.ItemID].Shape)
			for _, option := range selectedOptions {
				if !reportNode(false) {
					exhausted = true
					break
				}
				nodes++
				nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				shadowCompatible := shadow != nil && shadow.compatible(nextPlaced)
				nextOccupied := state.occupied | option.Mask
				restricted, flexibility, nextFeasible := constellationRootMRVFeasibility(remaining, nextRemainingMask, optionsByInstance, nextOccupied, nextPlaced)
				if !nextFeasible {
					hardPruned++
					depthDiagnostic.ZeroDomainPrunes++
					if shadowCompatible {
						shadowDepth.ShadowFeasibilityPruned++
					}
					continue
				}
				if nextRemainingMask == 0 && firstCompleteNodes == 0 {
					firstCompleteNodes = nodes
				}
				candidate := constellationRootMRVState{
					packingSeedState: packingSeedState{
						occupied:      nextOccupied,
						placed:        nextPlaced,
						restricted:    restricted,
						flexibility:   flexibility,
						fragmentation: freeSpaceFragmentation(gridMask, nextOccupied),
						score:         evaluateScoreForConfig(catalog, nextPlaced, config),
						key:           root.signature + "|" + coverageSeedAppendKey(state.key, option),
					},
					remainingMask: nextRemainingMask,
					remainingArea: nextRemainingArea,
				}
				if shadowDepth != nil {
					shadowDepth.Generated++
					if shadowCompatible {
						shadowDepth.GeneratedWitnessCompatible++
					}
				}
				classKey := constellationRootMRVStateKey(candidate)
				if previous, exists := nextByClass[classKey]; exists {
					deduplicated++
					if !constellationRootPackingStateLess(config, candidate.packingSeedState, previous.packingSeedState) {
						continue
					}
				}
				nextByClass[classKey] = candidate
				if nodes >= nodeBudget {
					exhausted = true
					break
				}
			}
		}
		depthDiagnostic.StatesAfterDedup = len(nextByClass)
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
			depthDiagnostic.BeamEvictions = int64(len(next) - beamWidth)
			beamEvictions += depthDiagnostic.BeamEvictions
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
			if shadow.trace.FirstLossStage == "" && shadowDepth.StatesBeforeWitnessCompatible > 0 && shadowDepth.RetainedWitnessCompatible == 0 {
				shadow.trace.FirstLossDepth = depth
				switch {
				case shadowDepth.GeneratedWitnessCompatible == 0 && shadowDepth.ShadowSymmetryPruned > 0:
					shadow.trace.FirstLossStage = "symmetry"
				case shadowDepth.GeneratedWitnessCompatible == 0 && shadowDepth.ShadowFeasibilityPruned > 0:
					shadow.trace.FirstLossStage = "feasibility"
				case shadowDepth.GeneratedWitnessCompatible == 0:
					shadow.trace.FirstLossStage = "generation"
				case shadowDepth.DedupWitnessCompatible == 0:
					shadow.trace.FirstLossStage = "dedup"
				default:
					shadow.trace.FirstLossStage = "beam"
					shadow.trace.FirstBeamEvictionDepth = depth
				}
			}
			shadow.trace.Depths = append(shadow.trace.Depths, *shadowDepth)
		}
		depthDiagnostic.StatesRetained = len(next)
		mrvDepths = append(mrvDepths, depthDiagnostic)
		states = next
		layerWidths = append(layerWidths, model.PackingSeedLayerWidth{Depth: depth, States: len(states)})
	}

	results := make([]model.Solution, 0, config.TopN)
	candidates := 0
	for _, state := range states {
		if state.remainingMask != 0 {
			continue
		}
		candidates++
		results = collectConstellationRootPackingSolution(catalog, results, state.placed, instances, root.rootID, config)
	}
	result.solutions = results
	result.nodes = nodes
	result.candidates = candidates
	result.deduplicated = deduplicated
	result.hardPruned = hardPruned
	result.symmetryPruned = symmetryPruned
	result.beamEvictions = beamEvictions
	result.layerWidths = layerWidths
	result.firstCompleteNodes = firstCompleteNodes
	result.distinctNextItems = len(selectedItemIDs)
	result.mrvDepths = mrvDepths
	if shadow != nil {
		trace := shadow.trace
		result.shadowTrace = &trace
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

func constellationRootPackingComparatorComponents(config Config, maxPriorities int, includeKey bool) []packingSeedComparatorComponent {
	policy := policyForConfig(config)
	if policy.ConstellationSeedVariant == ConstellationSeedVariantV5 || policy.ConstellationSeedVariant == ConstellationSeedVariantV51 || (config.forcedRootPackingReplay && resolvedConstellationForcedCandidateRootedPackingRanking(policy.ConstellationForcedCandidateRootedPackingRanking) == constellationRootPackingRankingPriorityScoreFirst) {
		return packingSeedPriorityScoreFirstComparatorComponents(maxPriorities, includeKey)
	}
	return packingSeedStateComparatorComponents(maxPriorities, includeKey)
}

func constellationRootPackingStateLess(config Config, left, right packingSeedState) bool {
	maxPriorities := len(left.score.PriorityCounts)
	if len(right.score.PriorityCounts) > maxPriorities {
		maxPriorities = len(right.score.PriorityCounts)
	}
	return packingSeedStateVariantFirstDecisive(left, right, constellationRootPackingComparatorComponents(config, maxPriorities, true)).compare > 0
}

func constellationRootPackingStatePrimaryCompare(config Config, left, right packingSeedState) int {
	maxPriorities := len(left.score.PriorityCounts)
	if len(right.score.PriorityCounts) > maxPriorities {
		maxPriorities = len(right.score.PriorityCounts)
	}
	return packingSeedStateVariantFirstDecisive(left, right, constellationRootPackingComparatorComponents(config, maxPriorities, false)).compare
}

func constellationShadowRankingAutopsy(states []constellationRootMRVState, shadowIndex int, beamWidth int, config Config) *model.ConstellationForcedCandidateShadowRankingAutopsy {
	if shadowIndex < 0 || shadowIndex >= len(states) {
		return nil
	}
	maxPriorities := 0
	for _, state := range states {
		if len(state.score.PriorityCounts) > maxPriorities {
			maxPriorities = len(state.score.PriorityCounts)
		}
	}
	components := constellationRootPackingComparatorComponents(config, maxPriorities, true)
	shadow := states[shadowIndex].packingSeedState
	ranking := resolvedConstellationForcedCandidateRootedPackingRanking(policyForConfig(config).ConstellationForcedCandidateRootedPackingRanking)
	comparator := "packing_seed_state_less/v1"
	version := "v4.8"
	if config.forcedRootPackingReplay && ranking == constellationRootPackingRankingPriorityScoreFirst {
		comparator = "priority_score_first/v1"
		version = "v4.10"
	}
	autopsy := &model.ConstellationForcedCandidateShadowRankingAutopsy{
		Version:                 version,
		Comparator:              comparator,
		ShadowComponents:        constellationRankingComponents(shadow, maxPriorities),
		StrictlyPrecedingStates: 0,
	}
	if len(states) > 0 {
		best := constellationRankingComponents(states[0].packingSeedState, maxPriorities)
		autopsy.BestComponents = &best
		cutoffIndex := beamWidth - 1
		if cutoffIndex >= 0 && cutoffIndex < len(states) {
			cutoff := constellationRankingComponents(states[cutoffIndex].packingSeedState, maxPriorities)
			autopsy.CutoffComponents = &cutoff
		}
	}

	autopsy.FirstDecisiveComponents = constellationDecisiveComponents(states, shadow, components)

	denominator := len(states) - 1
	if denominator < 1 {
		denominator = 1
	}
	for _, component := range components {
		percentile := model.ConstellationShadowComponentPercentile{Component: component.name}
		if component.hasPriority {
			priorityIndex := component.priorityIndex
			percentile.PriorityIndex = &priorityIndex
		}
		for index, state := range states {
			if index == shadowIndex {
				continue
			}
			compare := packingSeedStateComponentCompare(state.packingSeedState, shadow, component)
			if compare > 0 {
				percentile.BetterCount++
			} else if compare == 0 {
				percentile.EqualCount++
			}
		}
		percentile.BetterPercentileBps = percentile.BetterCount * 10_000 / denominator
		autopsy.ShadowComponentPercentiles = append(autopsy.ShadowComponentPercentiles, percentile)
	}

	for index := 0; index < shadowIndex; index++ {
		decision := packingSeedStateVariantFirstDecisive(states[index].packingSeedState, shadow, components)
		if decision.compare > 0 {
			autopsy.StrictlyPrecedingStates++
			continue
		}
		if decision.compare == 0 {
			autopsy.FullComparatorTieBeforeStates++
		}
	}
	autopsy.StrictlyPrecedingPercentileBps = autopsy.StrictlyPrecedingStates * 10_000 / denominator
	return autopsy
}

func constellationShadowCounterfactualReranking(states []constellationRootMRVState, shadow *constellationShadowReference, beamWidth int) *model.ConstellationForcedCandidateCounterfactualReranking {
	if shadow == nil || len(states) == 0 {
		return nil
	}
	maxPriorities := 0
	for _, state := range states {
		if len(state.score.PriorityCounts) > maxPriorities {
			maxPriorities = len(state.score.PriorityCounts)
		}
	}
	reranking := &model.ConstellationForcedCandidateCounterfactualReranking{
		Version:             "v4.9",
		PrioritySchemaWidth: maxPriorities,
		FrontierStates:      len(states),
		BeamWidth:           beamWidth,
	}
	for _, variant := range packingCounterfactualVariants(maxPriorities) {
		compatible := make([]int, 0)
		for index, state := range states {
			if shadow.compatible(state.placed) {
				compatible = append(compatible, index)
			}
		}
		record := model.ConstellationForcedCandidateCounterfactualVariant{
			ID:                   variant.id,
			ComparatorTuple:      variant.tuple,
			CompatibleStateCount: len(compatible),
		}
		if len(compatible) == 0 {
			reranking.Variants = append(reranking.Variants, record)
			continue
		}
		bestIndex := compatible[0]
		for _, index := range compatible[1:] {
			if packingSeedStateVariantFirstDecisive(states[index].packingSeedState, states[bestIndex].packingSeedState, variant.components).compare > 0 {
				bestIndex = index
			}
		}
		bestState := states[bestIndex].packingSeedState
		shadowComponents := constellationRankingComponents(bestState, maxPriorities)
		record.ShadowComponents = &shadowComponents
		bestComponents := constellationRankingComponents(states[0].packingSeedState, maxPriorities)
		record.BestComponents = &bestComponents
		if beamWidth > 0 && beamWidth <= len(states) {
			cutoffComponents := constellationRankingComponents(states[beamWidth-1].packingSeedState, maxPriorities)
			record.CutoffComponents = &cutoffComponents
		}
		keyless := make([]packingSeedComparatorComponent, 0, len(variant.components))
		for _, component := range variant.components {
			if !component.key {
				keyless = append(keyless, component)
			}
		}
		for _, state := range states {
			full := packingSeedStateVariantFirstDecisive(state.packingSeedState, bestState, variant.components).compare
			if full > 0 {
				record.StrictlyPrecedingStates++
			}
			if full == 0 {
				record.FullComparatorTieCount++
			}
			withoutKey := packingSeedStateVariantFirstDecisive(state.packingSeedState, bestState, keyless).compare
			if withoutKey > 0 {
				record.KeylessRankStart++
			}
			if withoutKey == 0 {
				record.KeylessTupleTieCount++
			}
		}
		record.FullRankStart = record.StrictlyPrecedingStates + 1
		record.FullRankEnd = record.FullRankStart + record.FullComparatorTieCount - 1
		record.KeylessRankStart++
		record.KeylessRankEnd = record.KeylessRankStart + record.KeylessTupleTieCount - 1
		record.ActualBeamFit = record.FullRankStart <= beamWidth
		record.ActualBeamFitAmbiguous = record.ActualBeamFit && record.FullRankEnd > beamWidth
		record.FullPossibleBeamFit = record.FullRankStart <= beamWidth
		record.FullGuaranteedBeamFit = record.FullRankEnd <= beamWidth
		record.KeylessPossibleBeamFit = record.KeylessRankStart <= beamWidth
		record.KeylessGuaranteedBeamFit = record.KeylessRankEnd <= beamWidth
		record.KeylessTieCrossesBeam = record.KeylessRankStart <= beamWidth && record.KeylessRankEnd > beamWidth
		record.FirstDecisiveComponents = constellationDecisiveComponents(states, bestState, variant.components)
		reranking.Variants = append(reranking.Variants, record)
	}
	return reranking
}

func constellationDecisiveComponents(states []constellationRootMRVState, shadow packingSeedState, components []packingSeedComparatorComponent) []model.ConstellationShadowDecisiveComponent {
	prefix := make([]bool, len(states))
	for index := range prefix {
		prefix[index] = true
	}
	result := make([]model.ConstellationShadowDecisiveComponent, 0, len(components))
	for _, component := range components {
		decisive := model.ConstellationShadowDecisiveComponent{Component: component.name}
		if component.hasPriority {
			priorityIndex := component.priorityIndex
			decisive.PriorityIndex = &priorityIndex
		}
		advantages := make([]int, 0)
		nextPrefix := make([]bool, len(states))
		for index, state := range states {
			if !prefix[index] {
				continue
			}
			decisive.PrefixEqualCount++
			compare := packingSeedStateComponentCompare(state.packingSeedState, shadow, component)
			switch {
			case compare > 0:
				decisive.BetterAtComponentCount++
				if !component.key {
					advantages = append(advantages, compare)
				}
			case compare < 0:
				decisive.WorseAtComponentCount++
			default:
				decisive.EqualAtComponentCount++
				nextPrefix[index] = true
			}
		}
		if len(advantages) > 0 {
			p50 := constellationNearestRank(advantages, 50)
			p90 := constellationNearestRank(advantages, 90)
			p100 := constellationNearestRank(advantages, 100)
			decisive.AdvantageP50 = &p50
			decisive.AdvantageP90 = &p90
			decisive.AdvantageP100 = &p100
		}
		result = append(result, decisive)
		prefix = nextPrefix
	}
	return result
}

func constellationRankingComponents(state packingSeedState, maxPriorities int) model.ConstellationForcedCandidateRankingComponents {
	priorities := make([]int, maxPriorities)
	copy(priorities, state.score.PriorityCounts)
	return model.ConstellationForcedCandidateRankingComponents{
		Restricted:                    state.restricted,
		Flexibility:                   state.flexibility,
		Fragmentation:                 state.fragmentation,
		PriorityCounts:                priorities,
		CraftCount:                    state.score.CraftCount,
		StarCount:                     state.score.StarCount,
		ItemCount:                     state.score.ItemCount,
		StarTargetBreadth:             state.score.StarTargetBreadth,
		StarReciprocalPairs:           state.score.StarReciprocalPairs,
		StarSourceDefinitionDiversity: state.score.StarSourceDefinitionDiversity,
		Key:                           state.key,
	}
}

func constellationNearestRank(values []int, percentile int) int {
	ordered := append([]int(nil), values...)
	sort.Ints(ordered)
	index := (len(ordered)*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	return ordered[index-1]
}

func collectConstellationRootPackingSolution(catalog model.Catalog, results []model.Solution, placements []model.Placement, instances []model.InventoryInstance, rootID string, config Config) []model.Solution {
	localConfig := config
	promote := config.constellationRootPackingCollector == nil || config.constellationRootPackingCollector.promote
	if promote && config.constellationRootOrigins != nil {
		config.constellationRootOrigins[canonicalLayoutHash(placements)] = rootID
	}
	if !promote {
		// Suppress only output hooks; candidate filtering and local ordering remain identical.
		localConfig.trace = nil
		localConfig.constellationRootOrigins = nil
	}
	return insertCandidateWithScoreOnlyFilter(catalog, results, placements, instances, localConfig)
}

func newConstellationShadowReference(candidate constellationSkeleton, witness model.Solution, semanticFingerprint string) *constellationShadowReference {
	anchoredByInstance := make(map[string]string, len(candidate.placed))
	anchoredIDs := make(map[string]struct{}, len(candidate.placed))
	for _, placement := range candidate.placed {
		anchoredByInstance[placement.InstanceID] = placementKey(placement)
		anchoredIDs[placement.InstanceID] = struct{}{}
	}
	unanchoredByItem := make(map[string]map[string]int)
	for _, placement := range witness.Placements {
		if _, anchored := anchoredIDs[placement.InstanceID]; anchored {
			continue
		}
		if unanchoredByItem[placement.ItemID] == nil {
			unanchoredByItem[placement.ItemID] = make(map[string]int)
		}
		unanchoredByItem[placement.ItemID][placementKey(placement)]++
	}
	return &constellationShadowReference{
		anchoredByInstance: anchoredByInstance,
		unanchoredByItem:   unanchoredByItem,
		trace: model.ConstellationForcedCandidateShadowTrace{
			SemanticFingerprint: semanticFingerprint,
			WitnessHash:         witness.CanonicalLayoutHash,
			ValidationStatus:    "accepted",
			Canonicalization:    "anchored_literal_unanchored_item_multiset/v1",
		},
	}
}

func constellationShadowSymmetryPrunedOptions(instance model.InventoryInstance, options []model.Placement, occupied uint64, placements []model.Placement, shadow *constellationShadowReference) int64 {
	if shadow == nil {
		return 0
	}
	var count int64
	for _, option := range options {
		if option.Mask&occupied != 0 || placementRespectsCanonicalCopyOrder(option, placements) {
			continue
		}
		nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
		if shadow.compatible(nextPlaced) {
			count++
		}
	}
	return count
}

func (reference *constellationShadowReference) compatible(placements []model.Placement) bool {
	if reference == nil {
		return false
	}
	used := make(map[string]map[string]int)
	for _, placement := range placements {
		key := placementKey(placement)
		if anchoredKey, anchored := reference.anchoredByInstance[placement.InstanceID]; anchored {
			if key != anchoredKey {
				return false
			}
			continue
		}
		available := reference.unanchoredByItem[placement.ItemID]
		if available == nil || available[key] == 0 {
			return false
		}
		if used[placement.ItemID] == nil {
			used[placement.ItemID] = make(map[string]int)
		}
		used[placement.ItemID][key]++
		if used[placement.ItemID][key] > available[key] {
			return false
		}
	}
	return true
}

func constellationRootMRVSelection(
	remaining []model.InventoryInstance,
	remainingMask uint64,
	optionsByInstance map[string][]model.Placement,
	occupied uint64,
	placements []model.Placement,
) (int, []model.Placement, bool) {
	selectedIndex := -1
	var selectedOptions []model.Placement
	for index, instance := range remaining {
		if remainingMask&(uint64(1)<<uint(instance.OriginalIndex)) == 0 {
			continue
		}
		legal := make([]model.Placement, 0, len(optionsByInstance[instance.InstanceID]))
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied == 0 && placementRespectsCanonicalCopyOrder(option, placements) {
				legal = append(legal, option)
			}
		}
		if len(legal) == 0 {
			return 0, nil, false
		}
		if selectedIndex < 0 || len(legal) < len(selectedOptions) {
			selectedIndex = index
			selectedOptions = legal
		}
	}
	return selectedIndex, selectedOptions, selectedIndex >= 0
}

func constellationRootMRVFeasibility(
	remaining []model.InventoryInstance,
	remainingMask uint64,
	optionsByInstance map[string][]model.Placement,
	occupied uint64,
	placements []model.Placement,
) (restricted int, flexibility int, feasible bool) {
	if remainingMask == 0 {
		return 0, 0, true
	}
	restricted = int(^uint(0) >> 1)
	for _, instance := range remaining {
		if remainingMask&(uint64(1)<<uint(instance.OriginalIndex)) == 0 {
			continue
		}
		legal := 0
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied == 0 && placementRespectsCanonicalCopyOrder(option, placements) {
				legal++
			}
		}
		if legal == 0 {
			return 0, 0, false
		}
		if legal < restricted {
			restricted = legal
		}
		flexibility += legal
	}
	return restricted, flexibility, true
}

func constellationRootMRVStateKey(state constellationRootMRVState) string {
	return fmtMask(state.occupied) + "|" + strconv.FormatUint(state.remainingMask, 16) + "|" + constellationExactKey(state.placed)
}

// constellationRootCompletionOptimizationProbe exhaustively compares all
// completions under fixed root anchors. It is diagnostic-only and deliberately
// avoids score upper-bound pruning.
func constellationRootCompletionOptimizationProbe(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func(bool) bool,
	initial model.Solution,
) constellationCompletionOptimizationProbeResult {
	return constellationRootCompletionOptimizationProbeWithCharge(catalog, instances, optionsByInstance, root, config, gridMask, nodeBudget, func() (bool, string) {
		if !reportNode(false) {
			return false, ledgerStopSourceStage
		}
		return true, ""
	}, initial)
}

func constellationRootCompletionOptimizationProbeWithCharge(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func() (bool, string),
	initial model.Solution,
) constellationCompletionOptimizationProbeResult {
	result := constellationCompletionOptimizationProbeResult{}
	if len(initial.Placements) > 0 {
		result.initialIncumbentFromRootPacking = true
		result.hasBest = true
		result.bestScore = cloneScore(initial.Evaluation.Score)
		result.bestLayoutKey = initial.LayoutKey
		result.bestHash = initial.CanonicalLayoutHash
		result.bestPlacements = append([]model.Placement(nil), initial.Placements...)
	}
	placedByID := placementByInstanceID(root.placed)
	remaining := make([]model.InventoryInstance, 0, len(instances)-len(root.placed))
	remainingArea := 0
	for _, instance := range instances {
		if _, placed := placedByID[instance.InstanceID]; placed {
			continue
		}
		remaining = append(remaining, instance)
		remainingArea += len(catalog.Items[instance.ItemID].Shape)
	}
	if len(remaining) == 0 {
		if !result.hasBest {
			result.hasBest = true
			result.bestScore = evaluateScoreForConfig(catalog, root.placed, config)
			result.bestLayoutKey = layoutKey(root.placed, instances)
			result.bestHash = canonicalLayoutHash(root.placed)
			result.bestPlacements = append([]model.Placement(nil), root.placed...)
		}
		result.status = "optimal_proven"
		result.terminationReason = "root_complete"
		result.searchExhausted = true
		result.firstCompleteNodes = 0
		result.hasFirstComplete = true
		return result
	}
	if remainingArea > bits.OnesCount64(gridMask&^root.occupied) {
		result.status = "infeasible_proven"
		result.terminationReason = "initial_area_shortfall"
		result.searchExhausted = true
		return result
	}
	if _, _, feasible := packingFeasibility(remaining, optionsByInstance, root.occupied, root.placed); !feasible {
		result.status = "infeasible_proven"
		result.terminationReason = "initial_domain_wipeout"
		result.searchExhausted = true
		return result
	}
	if nodeBudget <= 0 {
		result.status = "unknown_budget"
		result.terminationReason = "quota_exhausted"
		result.stopSource = "local_quota"
		return result
	}

	remaining = packingSeedOrder(remaining, optionsByInstance)
	var remainingMask uint64
	for _, instance := range remaining {
		remainingMask |= uint64(1) << uint(instance.OriginalIndex)
	}
	seen := make(map[string]struct{})
	exhausted := false
	var visit func(uint64, uint64, []model.Placement, int)
	visit = func(remainingMask uint64, occupied uint64, placements []model.Placement, area int) {
		if exhausted {
			return
		}
		if remainingMask == 0 {
			result.terminalCompletions++
			if !result.hasFirstComplete {
				result.firstCompleteNodes = result.nodes
				result.hasFirstComplete = true
			}
			score := evaluateScoreForConfig(catalog, placements, config)
			candidateLayoutKey := layoutKey(placements, instances)
			compare := 0
			if result.hasBest {
				compare = compareScores(score, result.bestScore)
			}
			if !result.hasBest || compare > 0 || (compare == 0 && candidateLayoutKey < result.bestLayoutKey) {
				if !result.hasBest || compare > 0 {
					if result.firstBestNodes == 0 {
						result.firstBestNodes = result.nodes
					}
					result.improvements = append(result.improvements, model.ConstellationCandidateCompletionScoreImprovement{
						Nodes:     result.nodes,
						Score:     cloneScore(score),
						LayoutKey: candidateLayoutKey,
						Hash:      canonicalLayoutHash(placements),
					})
				}
				result.hasBest = true
				result.bestScore = cloneScore(score)
				result.bestLayoutKey = candidateLayoutKey
				result.bestHash = canonicalLayoutHash(placements)
				result.bestPlacements = append([]model.Placement(nil), placements...)
			}
			return
		}
		if area > bits.OnesCount64(gridMask&^occupied) {
			result.areaPrunes++
			return
		}
		state := constellationRootMRVState{
			packingSeedState: packingSeedState{occupied: occupied, placed: placements},
			remainingMask:    remainingMask,
		}
		stateKey := constellationRootMRVStateKey(state)
		if _, exists := seen[stateKey]; exists {
			result.transpositionPrunes++
			return
		}
		seen[stateKey] = struct{}{}

		selectedIndex, selectedOptions, selected := constellationRootMRVSelection(remaining, remainingMask, optionsByInstance, occupied, placements)
		if !selected {
			result.zeroDomainPrunes++
			return
		}
		selectedInstance := remaining[selectedIndex]
		nextRemainingMask := remainingMask &^ (uint64(1) << uint(selectedInstance.OriginalIndex))
		nextArea := area - len(catalog.Items[selectedInstance.ItemID].Shape)
		for _, option := range selectedOptions {
			if result.nodes >= nodeBudget {
				result.stopSource = "local_quota"
				exhausted = true
				return
			}
			if ok, reason := reportNode(); !ok {
				result.stopSource = reason
				exhausted = true
				return
			}
			result.nodes++
			nextPlacements, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
			visit(nextRemainingMask, occupied|option.Mask, nextPlacements, nextArea)
			if exhausted {
				return
			}
		}
	}
	visit(remainingMask, root.occupied, append([]model.Placement(nil), root.placed...), remainingArea)
	if exhausted {
		result.status = "unknown_budget"
		result.terminationReason = "ledger_exhausted"
		if result.stopSource == "local_quota" {
			result.terminationReason = "quota_exhausted"
		}
		return result
	}
	result.searchExhausted = true
	if result.hasBest {
		result.status = "optimal_proven"
		result.terminationReason = "search_exhausted"
		return result
	}
	result.status = "infeasible_proven"
	result.terminationReason = "search_exhausted"
	return result
}

// constellationRootCompletionProbe is intentionally feasibility-only. It never
// evaluates a score or returns a layout to the candidate pipeline.
func constellationRootCompletionProbe(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	gridMask uint64,
	nodeBudget int64,
	reportNode func(bool) bool,
) constellationCompletionProbeResult {
	return constellationRootCompletionProbeWithCharge(catalog, instances, optionsByInstance, root, gridMask, nodeBudget, func() (bool, string) {
		if !reportNode(false) {
			return false, ledgerStopSourceStage
		}
		return true, ""
	})
}

func constellationRootCompletionProbeWithCharge(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	root constellationSkeleton,
	gridMask uint64,
	nodeBudget int64,
	reportNode func() (bool, string),
) constellationCompletionProbeResult {
	placedByID := placementByInstanceID(root.placed)
	remaining := make([]model.InventoryInstance, 0, len(instances)-len(root.placed))
	remainingArea := 0
	for _, instance := range instances {
		if _, placed := placedByID[instance.InstanceID]; placed {
			continue
		}
		remaining = append(remaining, instance)
		remainingArea += len(catalog.Items[instance.ItemID].Shape)
	}
	if len(remaining) == 0 {
		return constellationCompletionProbeResult{
			terminationReason: "root_complete",
			feasibilityStatus: "feasible",
			witnessHash:       canonicalLayoutHash(root.placed),
		}
	}
	if remainingArea > bits.OnesCount64(gridMask&^root.occupied) {
		return constellationCompletionProbeResult{
			terminationReason: "initial_area_shortfall",
			feasibilityStatus: "infeasible_proven",
			searchExhausted:   true,
		}
	}
	if _, _, feasible := packingFeasibility(remaining, optionsByInstance, root.occupied, root.placed); !feasible {
		return constellationCompletionProbeResult{
			terminationReason: "initial_domain_wipeout",
			feasibilityStatus: "infeasible_proven",
			searchExhausted:   true,
		}
	}
	if nodeBudget <= 0 {
		return constellationCompletionProbeResult{
			terminationReason: "quota_exhausted",
			feasibilityStatus: "unknown_budget",
			stopSource:        "local_quota",
		}
	}

	var nodes int64
	exhausted := false
	stopSource := ""
	witnessHash := ""
	dead := make(map[string]struct{})
	var visit func([]model.InventoryInstance, uint64, []model.Placement, int) bool
	visit = func(remaining []model.InventoryInstance, occupied uint64, placements []model.Placement, area int) bool {
		if exhausted {
			return false
		}
		if len(remaining) == 0 {
			witnessHash = canonicalLayoutHash(placements)
			return true
		}
		if area > bits.OnesCount64(gridMask&^occupied) {
			return false
		}
		stateKey := constellationExactKey(placements)
		if _, knownDead := dead[stateKey]; knownDead {
			return false
		}

		selectedIndex := -1
		var selectedOptions []model.Placement
		for index, instance := range remaining {
			legal := make([]model.Placement, 0, len(optionsByInstance[instance.InstanceID]))
			for _, option := range optionsByInstance[instance.InstanceID] {
				if option.Mask&occupied != 0 || !placementRespectsCanonicalCopyOrder(option, placements) {
					continue
				}
				legal = append(legal, option)
			}
			if len(legal) == 0 {
				dead[stateKey] = struct{}{}
				return false
			}
			if selectedIndex < 0 || len(legal) < len(selectedOptions) || (len(legal) == len(selectedOptions) && instance.OriginalIndex < remaining[selectedIndex].OriginalIndex) {
				selectedIndex = index
				selectedOptions = legal
			}
		}

		nextRemaining := append([]model.InventoryInstance(nil), remaining[:selectedIndex]...)
		nextRemaining = append(nextRemaining, remaining[selectedIndex+1:]...)
		nextArea := area - len(catalog.Items[remaining[selectedIndex].ItemID].Shape)
		for _, option := range selectedOptions {
			if nodes >= nodeBudget {
				stopSource = "local_quota"
				exhausted = true
				return false
			}
			if ok, reason := reportNode(); !ok {
				stopSource = reason
				exhausted = true
				return false
			}
			nodes++
			nextPlacements, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
			if visit(nextRemaining, occupied|option.Mask, nextPlacements, nextArea) {
				return true
			}
			if exhausted {
				return false
			}
		}
		dead[stateKey] = struct{}{}
		return false
	}

	if visit(remaining, root.occupied, append([]model.Placement(nil), root.placed...), remainingArea) {
		return constellationCompletionProbeResult{
			nodes:             nodes,
			terminationReason: "witness_found",
			feasibilityStatus: "feasible",
			witnessHash:       witnessHash,
		}
	}
	if exhausted {
		terminationReason := "ledger_exhausted"
		if stopSource == "local_quota" {
			terminationReason = "quota_exhausted"
		}
		return constellationCompletionProbeResult{
			nodes:             nodes,
			terminationReason: terminationReason,
			feasibilityStatus: "unknown_budget",
			stopSource:        stopSource,
		}
	}
	return constellationCompletionProbeResult{
		nodes:             nodes,
		terminationReason: "search_exhausted",
		feasibilityStatus: "infeasible_proven",
		searchExhausted:   true,
	}
}

func constellationCandidatePoolFeasibilitySweep(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	candidates map[string]constellationSkeleton,
	selectedRoots map[string]constellationSelectedRootOutcome,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func() (bool, string),
) model.ConstellationCandidatePoolFeasibilitySweep {
	stageID := config.stageID
	if stageID == "" {
		stageID = "single"
	}
	sweep := model.ConstellationCandidatePoolFeasibilitySweep{
		StageID:          stageID,
		SweepOrderPolicy: "source_geometry_orbit_then_canonical",
		NodesAvailable:   nodeBudget,
	}
	ordered, candidateRanks := orderedConstellationCandidatePool(candidates)
	if len(ordered) == 0 {
		sweep.NodesReturned = nodeBudget
		return sweep
	}
	sweep.CandidateCount = len(ordered)
	records := make([]model.ConstellationCandidateFeasibilityRecord, len(ordered))
	pending := make([]int, 0, len(ordered))
	for index, candidate := range ordered {
		record := constellationCandidateFeasibilityRecord(stageID, candidateRanks[candidate.exactKey], index+1, candidate, instances, gridMask)
		if selected, exists := selectedRoots[candidate.exactKey]; exists {
			record.SelectedRootID = selected.rootID
			sweep.SelectedRootCount++
			if len(selected.result.solutions) > 0 {
				record.AttemptKind = "root_packing_witness"
				record.FeasibilityStatus = "feasible"
				record.TerminationReason = "root_packing_witness"
				record.WitnessHash = selected.result.solutions[0].CanonicalLayoutHash
				sweep.FeasibleCount++
				records[index] = record
				continue
			}
		}
		record.AttemptKind = "exact_mrv"
		pending = append(pending, index)
		records[index] = record
	}

	nodesLeft := nodeBudget
	ledgerStopSource := ""
	for pendingIndex, recordIndex := range pending {
		record := records[recordIndex]
		if ledgerStopSource != "" {
			record.FeasibilityStatus = "unknown_budget"
			record.TerminationReason = "ledger_exhausted"
			record.StopSource = ledgerStopSource
			records[recordIndex] = record
			continue
		}
		remainingCandidates := len(pending) - pendingIndex
		quota := int64(0)
		if nodesLeft > 0 {
			quota = nodesLeft / int64(remainingCandidates)
			if quota < 1 {
				quota = 1
			}
			if quota > nodesLeft {
				quota = nodesLeft
			}
		}
		record.NodesAvailable = quota
		probe := constellationRootCompletionProbeWithCharge(catalog, instances, optionsByInstance, ordered[recordIndex], gridMask, quota, reportNode)
		record.NodesConsumed = probe.nodes
		record.NodesReturned = quota - probe.nodes
		record.FeasibilityStatus = probe.feasibilityStatus
		record.TerminationReason = probe.terminationReason
		record.StopSource = probe.stopSource
		record.SearchExhausted = probe.searchExhausted
		record.WitnessHash = probe.witnessHash
		nodesLeft -= probe.nodes
		sweep.NodesConsumed += probe.nodes
		switch record.FeasibilityStatus {
		case "feasible":
			sweep.FeasibleCount++
		case "infeasible_proven":
			sweep.InfeasibleProvenCount++
		default:
			sweep.UnknownBudgetCount++
		}
		if probe.stopSource == ledgerStopSourceGlobal || probe.stopSource == ledgerStopSourceStage {
			ledgerStopSource = probe.stopSource
		}
		records[recordIndex] = record
	}
	sweep.FeasibleCount = 0
	sweep.InfeasibleProvenCount = 0
	sweep.UnknownBudgetCount = 0
	for _, record := range records {
		switch record.FeasibilityStatus {
		case "feasible":
			sweep.FeasibleCount++
		case "infeasible_proven":
			sweep.InfeasibleProvenCount++
		default:
			sweep.UnknownBudgetCount++
		}
	}
	sweep.NodesReturned = nodesLeft
	sweep.Candidates = records
	sweep.Orbits = constellationCandidateFeasibilityOrbits(records)
	return sweep
}

func orderedConstellationCandidatePool(candidates map[string]constellationSkeleton) ([]constellationSkeleton, map[string]int) {
	canonical := make([]constellationSkeleton, 0, len(candidates))
	for _, candidate := range candidates {
		canonical = append(canonical, candidate)
	}
	sort.Slice(canonical, func(i, j int) bool { return constellationSkeletonLess(canonical[i], canonical[j]) })
	candidateRanks := make(map[string]int, len(canonical))
	for index, candidate := range canonical {
		candidateRanks[candidate.exactKey] = index + 1
	}
	ordered := make([]constellationSkeleton, 0, len(canonical))
	seenExact := make(map[string]struct{}, len(canonical))
	seenOrbits := make(map[string]struct{}, len(canonical))
	for _, candidate := range canonical {
		orbitKey := constellationSkeletonOrbitKey(candidate)
		if _, exists := seenOrbits[orbitKey]; exists {
			continue
		}
		ordered = append(ordered, candidate)
		seenExact[candidate.exactKey] = struct{}{}
		seenOrbits[orbitKey] = struct{}{}
	}
	for _, candidate := range canonical {
		if _, exists := seenExact[candidate.exactKey]; exists {
			continue
		}
		ordered = append(ordered, candidate)
	}
	return ordered, candidateRanks
}

func constellationCandidateFeasibilityRecord(
	stageID string,
	candidateRank int,
	sweepRank int,
	candidate constellationSkeleton,
	instances []model.InventoryInstance,
	gridMask uint64,
) model.ConstellationCandidateFeasibilityRecord {
	anchoredIDs := make([]string, 0, len(candidate.placed))
	placed := placementByInstanceID(candidate.placed)
	remainingIDs := make([]string, 0, len(instances)-len(candidate.placed))
	for _, placement := range candidate.placed {
		anchoredIDs = append(anchoredIDs, placement.InstanceID)
	}
	sort.Strings(anchoredIDs)
	for _, instance := range instances {
		if _, exists := placed[instance.InstanceID]; !exists {
			remainingIDs = append(remainingIDs, instance.InstanceID)
		}
	}
	freeMask := gridMask &^ candidate.occupied
	return model.ConstellationCandidateFeasibilityRecord{
		StageID:                stageID,
		CandidateID:            constellationCandidateID(candidate.exactKey),
		CandidateRank:          candidateRank,
		SweepRank:              sweepRank,
		ExactAnchorKey:         candidate.exactKey,
		Signature:              candidate.signature,
		PartialScore:           cloneScore(candidate.score),
		SourceGeometryKey:      candidate.sourceGeometryKey,
		SourceGeometryOrbitKey: constellationSkeletonOrbitKey(candidate),
		TargetAssignmentKey:    candidate.targetAssignmentKey,
		OccupiedMaskHex:        fmtMask(candidate.occupied),
		FreeMaskHex:            fmtMask(freeMask),
		FreeMaskOrbitKey:       topBottomMaskOrbitKey(freeMask),
		AnchoredInstanceIDs:    anchoredIDs,
		RemainingInstanceIDs:   remainingIDs,
	}
}

func constellationCandidateID(exactKey string) string {
	digest := sha256.Sum256([]byte(exactKey))
	return hex.EncodeToString(digest[:])
}

func cloneConstellationSkeletons(source map[string]constellationSkeleton) map[string]constellationSkeleton {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]constellationSkeleton, len(source))
	for key, skeleton := range source {
		copy := skeleton
		copy.placed = append([]model.Placement(nil), skeleton.placed...)
		copy.priorityLinks = append([]model.PlateauLink(nil), skeleton.priorityLinks...)
		copy.score = cloneScore(skeleton.score)
		cloned[key] = copy
	}
	return cloned
}

func cloneConstellationSelectedRootOutcomes(source map[string]constellationSelectedRootOutcome) map[string]constellationSelectedRootOutcome {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]constellationSelectedRootOutcome, len(source))
	for key, outcome := range source {
		copy := outcome
		copy.result.solutions = cloneStageSolutions(outcome.result.solutions)
		cloned[key] = copy
	}
	return cloned
}

func constellationCandidateFeasibilityOrbits(records []model.ConstellationCandidateFeasibilityRecord) []model.ConstellationCandidateFeasibilityOrbit {
	type aggregate struct {
		orbit          model.ConstellationCandidateFeasibilityOrbit
		rawGeometries  map[string]struct{}
		freeMasks      map[string]struct{}
		freeMaskOrbits map[string]struct{}
	}
	byOrbit := make(map[string]*aggregate)
	for _, record := range records {
		entry := byOrbit[record.SourceGeometryOrbitKey]
		if entry == nil {
			entry = &aggregate{
				orbit:          model.ConstellationCandidateFeasibilityOrbit{StageID: record.StageID, SourceGeometryOrbitKey: record.SourceGeometryOrbitKey},
				rawGeometries:  make(map[string]struct{}),
				freeMasks:      make(map[string]struct{}),
				freeMaskOrbits: make(map[string]struct{}),
			}
			byOrbit[record.SourceGeometryOrbitKey] = entry
		}
		entry.orbit.CandidateCount++
		if record.SelectedRootID != "" {
			entry.orbit.SelectedRootCount++
		}
		entry.rawGeometries[record.SourceGeometryKey] = struct{}{}
		entry.freeMasks[record.FreeMaskHex] = struct{}{}
		entry.freeMaskOrbits[record.FreeMaskOrbitKey] = struct{}{}
		entry.orbit.NodesConsumed += record.NodesConsumed
		switch record.FeasibilityStatus {
		case "feasible":
			entry.orbit.FeasibleCount++
			if entry.orbit.BestPartialScoreFeasible == nil || compareScores(record.PartialScore, *entry.orbit.BestPartialScoreFeasible) > 0 {
				score := cloneScore(record.PartialScore)
				entry.orbit.BestPartialScoreFeasible = &score
			}
		case "infeasible_proven":
			entry.orbit.InfeasibleProvenCount++
		default:
			entry.orbit.UnknownBudgetCount++
		}
	}
	result := make([]model.ConstellationCandidateFeasibilityOrbit, 0, len(byOrbit))
	for _, entry := range byOrbit {
		entry.orbit.DistinctRawGeometries = len(entry.rawGeometries)
		entry.orbit.DistinctFreeMasks = len(entry.freeMasks)
		entry.orbit.DistinctFreeMaskOrbits = len(entry.freeMaskOrbits)
		result = append(result, entry.orbit)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StageID != result[j].StageID {
			return result[i].StageID < result[j].StageID
		}
		return result[i].SourceGeometryOrbitKey < result[j].SourceGeometryOrbitKey
	})
	return result
}

func constellationCandidateCompletionOptimization(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	candidates map[string]constellationSkeleton,
	selectedRoots map[string]constellationSelectedRootOutcome,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	reportNode func() (bool, string),
) model.ConstellationCandidateCompletionOptimization {
	policy := policyForConfig(config)
	stageID := config.stageID
	if stageID == "" {
		stageID = "single"
	}
	optimization := model.ConstellationCandidateCompletionOptimization{
		RequestedCandidateID: policy.ConstellationCandidateCompletionOptimizationCandidateID,
		RequestedStageID:     policy.ConstellationCandidateCompletionOptimizationStage,
	}
	if optimization.RequestedStageID != "" && optimization.RequestedStageID != stageID {
		optimization.Attempts = append(optimization.Attempts, model.ConstellationCandidateCompletionAttempt{
			StageID:         stageID,
			CandidateID:     optimization.RequestedCandidateID,
			SelectionStatus: "stage_not_selected",
		})
		return optimization
	}
	ordered, candidateRanks := orderedConstellationCandidatePool(candidates)
	matches := make([]int, 0, 1)
	for index, candidate := range ordered {
		if constellationCandidateID(candidate.exactKey) == optimization.RequestedCandidateID {
			matches = append(matches, index)
		}
	}
	if len(matches) == 0 {
		optimization.Attempts = append(optimization.Attempts, model.ConstellationCandidateCompletionAttempt{
			StageID:         stageID,
			CandidateID:     optimization.RequestedCandidateID,
			SelectionStatus: "target_not_found",
		})
		return optimization
	}
	if len(matches) > 1 {
		optimization.Attempts = append(optimization.Attempts, model.ConstellationCandidateCompletionAttempt{
			StageID:         stageID,
			CandidateID:     optimization.RequestedCandidateID,
			SelectionStatus: "candidate_id_collision",
		})
		return optimization
	}
	candidateIndex := matches[0]
	candidate := ordered[candidateIndex]
	metadata := constellationCandidateFeasibilityRecord(stageID, candidateRanks[candidate.exactKey], candidateIndex+1, candidate, instances, gridMask)
	partialScore := cloneScore(metadata.PartialScore)
	semanticFingerprint := constellationCandidateSemanticFingerprint(catalog, instances, candidate, config, gridMask)
	attempt := model.ConstellationCandidateCompletionAttempt{
		StageID:                stageID,
		CandidateID:            metadata.CandidateID,
		CandidateRank:          metadata.CandidateRank,
		SweepRank:              metadata.SweepRank,
		SelectionStatus:        "accepted",
		ExactAnchorKey:         metadata.ExactAnchorKey,
		Signature:              metadata.Signature,
		PartialScore:           &partialScore,
		SourceGeometryKey:      metadata.SourceGeometryKey,
		SourceGeometryOrbitKey: metadata.SourceGeometryOrbitKey,
		TargetAssignmentKey:    metadata.TargetAssignmentKey,
		AnchoredInstanceIDs:    metadata.AnchoredInstanceIDs,
		RemainingInstanceIDs:   metadata.RemainingInstanceIDs,
		InitialOccupiedMaskHex: metadata.OccupiedMaskHex,
		InitialFreeMaskHex:     metadata.FreeMaskHex,
		NodesAvailable:         nodeBudget,
		SemanticFingerprint:    semanticFingerprint,
	}
	initial := model.Solution{}
	if selected, exists := selectedRoots[candidate.exactKey]; exists && len(selected.result.solutions) > 0 {
		initial = selected.result.solutions[0]
		attempt.InitialIncumbentAvailable = true
		attempt.InitialIncumbentSource = "selected_root_packing"
		attempt.InitialIncumbentHash = initial.CanonicalLayoutHash
	}
	if config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey != "" {
		witness, witnessErr := constellationCandidateWitnessFromLayoutKey(config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey, config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint, semanticFingerprint, candidate, instances, optionsByInstance, catalog, config)
		if witnessErr != nil {
			attempt.InitialWitnessRejectionReason = witnessErr.Error()
		} else if len(initial.Placements) == 0 || compareScores(witness.Evaluation.Score, initial.Evaluation.Score) > 0 || (compareScores(witness.Evaluation.Score, initial.Evaluation.Score) == 0 && witness.LayoutKey < initial.LayoutKey) {
			initial = witness
			attempt.InitialIncumbentAvailable = true
			attempt.InitialIncumbentSource = "validated_external_witness"
			attempt.InitialIncumbentHash = witness.CanonicalLayoutHash
		}
		if witnessErr == nil {
			attempt.InitialWitness = constellationCandidateCompletionWitness(semanticFingerprint, metadata.CandidateID, metadata.ExactAnchorKey, initial)
		}
	}
	result := constellationRootCompletionOptimizationProbeWithCharge(catalog, instances, optionsByInstance, candidate, config, gridMask, nodeBudget, reportNode, initial)
	attempt.NodesConsumed = result.nodes
	attempt.NodesReturned = nodeBudget - result.nodes
	attempt.Status = result.status
	attempt.TerminationReason = result.terminationReason
	attempt.StopSource = result.stopSource
	attempt.SearchExhausted = result.searchExhausted
	attempt.TerminalCompletions = result.terminalCompletions
	attempt.AreaPrunes = result.areaPrunes
	attempt.ZeroDomainPrunes = result.zeroDomainPrunes
	attempt.TranspositionPrunes = result.transpositionPrunes
	if result.hasFirstComplete {
		firstComplete := result.firstCompleteNodes
		attempt.FirstCompleteNodes = &firstComplete
	}
	if attempt.InitialIncumbentAvailable {
		firstIncumbent := int64(0)
		attempt.FirstIncumbentNodes = &firstIncumbent
	} else if result.hasFirstComplete {
		firstIncumbent := result.firstCompleteNodes
		attempt.FirstIncumbentNodes = &firstIncumbent
	}
	attempt.FirstBestNodes = result.firstBestNodes
	attempt.ScoreImprovements = append([]model.ConstellationCandidateCompletionScoreImprovement(nil), result.improvements...)
	if result.hasBest {
		score := cloneScore(result.bestScore)
		attempt.BestScore = &score
		attempt.BestLayoutKey = result.bestLayoutKey
		attempt.BestHash = result.bestHash
		bestSolution := model.Solution{Placements: result.bestPlacements, Evaluation: model.Evaluation{Score: result.bestScore}, LayoutKey: result.bestLayoutKey, CanonicalLayoutHash: result.bestHash}
		attempt.BestWitness = constellationCandidateCompletionWitness(semanticFingerprint, metadata.CandidateID, metadata.ExactAnchorKey, bestSolution)
		if attempt.InitialIncumbentAvailable && compareScores(initial.Evaluation.Score, result.bestScore) == 0 {
			finalBest := int64(0)
			attempt.FinalBestFirstSeenNodes = &finalBest
		} else {
			for _, improvement := range result.improvements {
				if compareScores(improvement.Score, result.bestScore) == 0 {
					finalBest := improvement.Nodes
					attempt.FinalBestFirstSeenNodes = &finalBest
					break
				}
			}
			if attempt.FinalBestFirstSeenNodes == nil && result.hasFirstComplete && result.terminationReason == "root_complete" {
				finalBest := int64(0)
				attempt.FinalBestFirstSeenNodes = &finalBest
			}
		}
	}
	optimization.Attempts = append(optimization.Attempts, attempt)
	return optimization
}

func constellationForcedCandidateRootedPacking(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	candidates map[string]constellationSkeleton,
	selectedRoots map[string]constellationSelectedRootOutcome,
	config Config,
	gridMask uint64,
) model.ConstellationForcedCandidateRootedPacking {
	policy := policyForConfig(config)
	stageID := config.stageID
	if stageID == "" {
		stageID = "single"
	}
	probe := model.ConstellationForcedCandidateRootedPacking{
		RequestedCandidateID: policy.ConstellationForcedCandidateRootedPackingCandidateID,
		RequestedSlot:        policy.ConstellationForcedCandidateRootedPackingSlot,
		RequestedStageID:     policy.ConstellationForcedCandidateRootedPackingStage,
	}
	if probe.RequestedStageID != "" && probe.RequestedStageID != stageID {
		probe.Attempts = append(probe.Attempts, model.ConstellationForcedCandidateRootedPackingAttempt{
			StageID: stageID, CandidateID: probe.RequestedCandidateID, ForcedRootSlot: probe.RequestedSlot, SelectionStatus: "stage_not_selected",
		})
		return probe
	}
	ordered, candidateRanks := orderedConstellationCandidatePool(candidates)
	matchIndex := -1
	for index, candidate := range ordered {
		if constellationCandidateID(candidate.exactKey) != probe.RequestedCandidateID {
			continue
		}
		if matchIndex >= 0 {
			probe.Attempts = append(probe.Attempts, model.ConstellationForcedCandidateRootedPackingAttempt{
				StageID: stageID, CandidateID: probe.RequestedCandidateID, ForcedRootSlot: probe.RequestedSlot, SelectionStatus: "candidate_id_collision",
			})
			return probe
		}
		matchIndex = index
	}
	if matchIndex < 0 {
		probe.Attempts = append(probe.Attempts, model.ConstellationForcedCandidateRootedPackingAttempt{
			StageID: stageID, CandidateID: probe.RequestedCandidateID, ForcedRootSlot: probe.RequestedSlot, SelectionStatus: "target_not_found",
		})
		return probe
	}
	replacedRootID := "root-" + strconv.Itoa(probe.RequestedSlot)
	replaced, exists := selectedRootOutcomeByID(selectedRoots, replacedRootID)
	if !exists {
		probe.Attempts = append(probe.Attempts, model.ConstellationForcedCandidateRootedPackingAttempt{
			StageID: stageID, CandidateID: probe.RequestedCandidateID, ForcedRootSlot: probe.RequestedSlot, SelectionStatus: "root_slot_unavailable",
		})
		return probe
	}
	candidate := ordered[matchIndex]
	metadata := constellationCandidateFeasibilityRecord(stageID, candidateRanks[candidate.exactKey], matchIndex+1, candidate, instances, gridMask)
	attempt := model.ConstellationForcedCandidateRootedPackingAttempt{
		StageID:                   stageID,
		CandidateID:               metadata.CandidateID,
		CandidateRank:             metadata.CandidateRank,
		SweepRank:                 metadata.SweepRank,
		SelectionStatus:           "accepted",
		ForcedRootSlot:            probe.RequestedSlot,
		BaselinePackingBeamWidth:  policy.ConstellationSeedPackingBeamWidth,
		EffectivePackingBeamWidth: policy.ConstellationSeedPackingBeamWidth,
		PackingRanking:            resolvedConstellationForcedCandidateRootedPackingRanking(policy.ConstellationForcedCandidateRootedPackingRanking),
		PackingStrategy:           policy.ConstellationSeedPackingStrategy,
		ReplacedRootID:            replaced.rootID,
		NormalSlotNodesReserved:   replaced.nodesReserved,
		NormalSlotNodesConsumed:   replaced.result.nodes,
		NormalSlotNodesReturned:   replaced.nodesReserved - replaced.result.nodes,
		ExactAnchorKey:            metadata.ExactAnchorKey,
		PartialScore:              &metadata.PartialScore,
		SourceGeometryKey:         metadata.SourceGeometryKey,
		SourceGeometryOrbitKey:    metadata.SourceGeometryOrbitKey,
		TargetAssignmentKey:       metadata.TargetAssignmentKey,
		AnchoredInstanceIDs:       metadata.AnchoredInstanceIDs,
		NodesAvailable:            replaced.nodesReserved,
		WitnessUsed:               false,
		ExactSearchUsed:           false,
	}
	if attempt.EffectivePackingBeamWidth <= 0 {
		attempt.EffectivePackingBeamWidth = policy.PackingSeedBeamWidth
	}
	if policy.ConstellationForcedCandidateRootedPackingBeamWidth > 0 {
		attempt.EffectivePackingBeamWidth = policy.ConstellationForcedCandidateRootedPackingBeamWidth
	}
	var shadow *constellationShadowReference
	if config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey != "" {
		semanticFingerprint := constellationCandidateSemanticFingerprint(catalog, instances, candidate, config, gridMask)
		witness, witnessErr := constellationCandidateWitnessFromLayoutKey(config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey, config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint, semanticFingerprint, candidate, instances, optionsByInstance, catalog, config)
		if witnessErr != nil {
			attempt.ShadowWitnessTrace = &model.ConstellationForcedCandidateShadowTrace{
				SemanticFingerprint: semanticFingerprint,
				ValidationStatus:    "rejected",
				ValidationReason:    witnessErr.Error(),
				Canonicalization:    "anchored_literal_unanchored_item_multiset/v1",
			}
		} else {
			shadow = newConstellationShadowReference(candidate, witness, semanticFingerprint)
			attempt.ShadowReferenceUsed = true
		}
	}
	if !config.ledger.configureDiagnosticMax(replaced.nodesReserved) {
		attempt.SelectionStatus = "diagnostic_budget_unavailable"
		probe.Attempts = append(probe.Attempts, attempt)
		return probe
	}
	packingConfig := config
	packingConfig.forcedRootPackingReplay = true
	packingConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false, shadow: shadow}
	rootResult := constellationRootPackingSearch(catalog, instances, optionsByInstance, candidate, packingConfig, gridMask, replaced.nodesReserved, func(bool) bool {
		ok, _ := chargeDiagnosticNodeWithReason(config)
		return ok
	})
	attempt.NodesConsumed = rootResult.nodes
	attempt.NodesReturned = replaced.nodesReserved - rootResult.nodes
	attempt.Completed = len(rootResult.solutions) > 0
	attempt.CandidateCount = rootResult.candidates
	attempt.TerminationReason = rootResult.terminationReason
	attempt.FirstCompleteNodes = rootResult.firstCompleteNodes
	attempt.BeamEvictions = rootResult.beamEvictions
	attempt.HardDeadPruned = rootResult.hardPruned
	attempt.SymmetryPruned = rootResult.symmetryPruned
	attempt.StatesDeduplicated = rootResult.deduplicated
	attempt.MRVDepths = append([]model.ConstellationRootPackingDepthDiagnostic(nil), rootResult.mrvDepths...)
	if shadow != nil {
		attempt.ShadowWitnessTrace = rootResult.shadowTrace
		if rootResult.shadowTrace != nil {
			for _, depth := range rootResult.shadowTrace.Depths {
				if depth.BestPrecutRank > 0 && (attempt.MaxShadowPrecutRank == nil || depth.BestPrecutRank > *attempt.MaxShadowPrecutRank) {
					rank := depth.BestPrecutRank
					attempt.MaxShadowPrecutRank = &rank
				}
				if depth.BestRetainedRank > 0 && (attempt.MaxShadowRetainedRank == nil || depth.BestRetainedRank > *attempt.MaxShadowRetainedRank) {
					rank := depth.BestRetainedRank
					attempt.MaxShadowRetainedRank = &rank
				}
			}
		}
	}
	if rootResult.terminationReason == "budget_exhausted" && rootResult.nodes == replaced.nodesReserved {
		attempt.StopSource = "local_quota"
	}
	if len(rootResult.solutions) > 0 {
		best := rootResult.solutions[0]
		score := cloneScore(best.Evaluation.Score)
		attempt.BestScore = &score
		attempt.BestLayoutKey = best.LayoutKey
		attempt.BestHash = best.CanonicalLayoutHash
	}
	probe.Attempts = append(probe.Attempts, attempt)
	return probe
}

// constellationParentFrontierHedge replays the family selected by normal V5
// after that lane is sealed. Its members share one frozen diagnostic quota but
// never share an incumbent, collector, or any search guidance.
func constellationParentFrontierHedge(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	snapshot *constellationCandidateCompletionSnapshot,
	config Config,
	gridMask uint64,
) model.ConstellationParentFrontierHedge {
	policy := policyForConfig(config)
	stageID := config.stageID
	if stageID == "" {
		stageID = "single"
	}
	probe := model.ConstellationParentFrontierHedge{
		RequestedStageID:                       policy.ConstellationParentFrontierHedgeProbeStage,
		HighParentConsumptionProbeThresholdBps: highParentConsumptionProbeThresholdBps,
	}
	if probe.RequestedStageID != "" && probe.RequestedStageID != stageID {
		probe.Attempts = append(probe.Attempts, model.ConstellationParentFrontierHedgeAttempt{StageID: stageID, SelectionStatus: "stage_not_selected"})
		return probe
	}
	if snapshot == nil || len(snapshot.selectedSkeletons) == 0 {
		probe.Attempts = append(probe.Attempts, model.ConstellationParentFrontierHedgeAttempt{StageID: stageID, SelectionStatus: "selection_snapshot_unavailable"})
		return probe
	}
	var frontier constellationSkeleton
	foundFrontier := false
	for _, selected := range snapshot.selectedSkeletons {
		if selected.selectionPolicy != "v5_relaxation_frontier" {
			continue
		}
		if foundFrontier && selected.rootID < frontier.rootID {
			frontier = selected
			continue
		}
		if !foundFrontier {
			frontier = selected
			foundFrontier = true
		}
	}
	if !foundFrontier {
		probe.Attempts = append(probe.Attempts, model.ConstellationParentFrontierHedgeAttempt{StageID: stageID, SelectionStatus: "no_frontier"})
		return probe
	}
	parent, exists := snapshot.candidates[frontier.relaxedFromExactKey]
	if !exists || !constellationSkeletonStrictlyRelaxes(frontier, parent) {
		probe.Attempts = append(probe.Attempts, model.ConstellationParentFrontierHedgeAttempt{StageID: stageID, SelectionStatus: "parent_not_found"})
		return probe
	}
	normalSlot, exists := snapshot.selectedRoots[frontier.exactKey]
	if !exists || normalSlot.nodesReserved <= 0 {
		probe.Attempts = append(probe.Attempts, model.ConstellationParentFrontierHedgeAttempt{StageID: stageID, SelectionStatus: "normal_slot_unavailable"})
		return probe
	}
	attempt := model.ConstellationParentFrontierHedgeAttempt{
		StageID:                 stageID,
		SelectionStatus:         "accepted",
		FamilySlotID:            normalSlot.rootID,
		SlotCount:               len(snapshot.selectedSkeletons),
		FamilyMemberCount:       2,
		ParentExactKey:          parent.exactKey,
		FrontierExactKey:        frontier.exactKey,
		TotalQuota:              normalSlot.nodesReserved,
		NormalSlotNodesConsumed: normalSlot.result.nodes,
		NormalSlotNodesReturned: normalSlot.nodesReserved - normalSlot.result.nodes,
		Parent:                  constellationParentFrontierHedgeMember(policy, normalSlot.nodesReserved),
	}
	if !config.ledger.configureDiagnosticMax(attempt.TotalQuota) {
		attempt.SelectionStatus = "diagnostic_budget_unavailable"
		probe.Attempts = append(probe.Attempts, attempt)
		return probe
	}
	packingConfig := config
	packingConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false}
	parentResult := constellationRootPackingSearch(catalog, instances, optionsByInstance, parent, packingConfig, gridMask, attempt.TotalQuota, func(bool) bool {
		ok, _ := chargeDiagnosticNodeWithReason(config)
		return ok
	})
	attempt.Parent = constellationParentFrontierHedgeMemberFromResult(attempt.Parent, parentResult, attempt.TotalQuota)
	frontierQuota := attempt.Parent.Returned
	attempt.Frontier = constellationParentFrontierHedgeMember(policy, frontierQuota)
	attempt.Frontier.ResidualFractionBps = fractionBps(frontierQuota, attempt.TotalQuota)
	var frontierSolutions []model.Solution
	if frontierQuota == 0 {
		attempt.Frontier.SkippedReason = "no_residual_quota"
	} else {
		// Use a fresh local collector so the parent cannot guide frontier search.
		frontierConfig := config
		frontierConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false}
		frontierResult := constellationRootPackingSearch(catalog, instances, optionsByInstance, frontier, frontierConfig, gridMask, frontierQuota, func(bool) bool {
			ok, _ := chargeDiagnosticNodeWithReason(config)
			return ok
		})
		frontierSolutions = frontierResult.solutions
		attempt.Frontier = constellationParentFrontierHedgeMemberFromResult(attempt.Frontier, frontierResult, frontierQuota)
		attempt.Frontier.ResidualFractionBps = fractionBps(frontierQuota, attempt.TotalQuota)
		attempt.RootMemberExecutions = 2
	}
	if attempt.RootMemberExecutions == 0 {
		attempt.RootMemberExecutions = 1
	}
	attempt.FamilyConsumed = attempt.Parent.Consumed + attempt.Frontier.Consumed
	attempt.FamilyReturned = attempt.TotalQuota - attempt.FamilyConsumed
	attempt.HypotheticalReturnDelta = attempt.FamilyReturned - attempt.NormalSlotNodesReturned
	if attempt.FamilyReturned < 0 {
		attempt.FamilyReturned = 0
	}
	constellationParentFrontierFamilyBest(&attempt, parentResult.solutions, frontierSolutions)
	probe.Attempts = append(probe.Attempts, attempt)
	return probe
}

func constellationParentFrontierHedgeMember(policy ResolvedSearchPolicy, reserved int64) model.ConstellationParentFrontierHedgeMember {
	return model.ConstellationParentFrontierHedgeMember{
		Policy:           policy.ConstellationSeedVariant + "_priority_score_first",
		PackingBeamWidth: policy.ConstellationSeedPackingBeamWidth,
		PackingStrategy:  policy.ConstellationSeedPackingStrategy,
		Reserved:         reserved,
	}
}

func constellationParentFrontierHedgeMemberFromResult(member model.ConstellationParentFrontierHedgeMember, result constellationRootPackingResult, reserved int64) model.ConstellationParentFrontierHedgeMember {
	member.Invoked = true
	member.Consumed = result.nodes
	member.Returned = reserved - result.nodes
	if member.Returned < 0 {
		member.Returned = 0
	}
	member.ConsumedFractionBps = fractionBps(member.Consumed, reserved)
	member.Completed = len(result.solutions) > 0
	member.TerminationReason = result.terminationReason
	member.FirstCompleteNodes = result.firstCompleteNodes
	if len(result.solutions) > 0 {
		best := result.solutions[0]
		score := cloneScore(best.Evaluation.Score)
		member.BestScore = &score
		member.BestHash = best.CanonicalLayoutHash
	}
	return member
}

func constellationParentFrontierFamilyBest(attempt *model.ConstellationParentFrontierHedgeAttempt, parentSolutions []model.Solution, frontierSolutions []model.Solution) {
	var best model.Solution
	winner := ""
	if len(parentSolutions) > 0 {
		best = parentSolutions[0]
		winner = "parent"
	}
	if len(frontierSolutions) > 0 && (winner == "" || SolutionLess(frontierSolutions[0], best)) {
		best = frontierSolutions[0]
		winner = "frontier"
	}
	if winner == "" {
		attempt.FamilyWinnerMember = "none"
		return
	}
	score := cloneScore(best.Evaluation.Score)
	attempt.FamilyBestScore = &score
	attempt.FamilyBestHash = best.CanonicalLayoutHash
	attempt.FamilyWinnerMember = winner
}

func fractionBps(numerator int64, denominator int64) int64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return numerator * 10_000 / denominator
}

func selectedRootOutcomeByID(outcomes map[string]constellationSelectedRootOutcome, rootID string) (constellationSelectedRootOutcome, bool) {
	for _, outcome := range outcomes {
		if outcome.rootID == rootID {
			return outcome, true
		}
	}
	return constellationSelectedRootOutcome{}, false
}

func constellationCandidateSemanticFingerprint(catalog model.Catalog, instances []model.InventoryInstance, candidate constellationSkeleton, config Config, gridMask uint64) string {
	type inventoryIdentity struct {
		InstanceID    string `json:"instance_id"`
		ItemID        string `json:"item_id"`
		OriginalIndex int    `json:"original_index"`
	}
	inventory := make([]inventoryIdentity, 0, len(instances))
	for _, instance := range instances {
		inventory = append(inventory, inventoryIdentity{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex})
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		panic(fmt.Sprintf("marshal candidate witness catalog: %v", err))
	}
	catalogDigest := sha256.Sum256(catalogJSON)
	value := struct {
		SchemaVersion     string                `json:"schema_version"`
		CatalogSHA256     string                `json:"catalog_sha256"`
		GridMask          string                `json:"grid_mask"`
		Inventory         []inventoryIdentity   `json:"inventory"`
		PrioritySemantics string                `json:"priority_semantics"`
		Priorities        []string              `json:"priorities"`
		CoverageGroups    []model.CoverageGroup `json:"coverage_groups"`
		CandidateID       string                `json:"candidate_id"`
		ExactAnchorKey    string                `json:"exact_anchor_key"`
		MRVVersion        string                `json:"mrv_version"`
	}{
		SchemaVersion:     "candidate-witness/v1",
		CatalogSHA256:     hex.EncodeToString(catalogDigest[:]),
		GridMask:          fmtMask(gridMask),
		Inventory:         inventory,
		PrioritySemantics: string(config.PrioritySemantics),
		Priorities:        append([]string(nil), config.Priorities...),
		CoverageGroups:    append([]model.CoverageGroup(nil), config.CoverageGroups...),
		CandidateID:       constellationCandidateID(candidate.exactKey),
		ExactAnchorKey:    candidate.exactKey,
		MRVVersion:        "state_mrv_exact/v1",
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal candidate witness semantics: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func constellationCandidateCompletionWitness(semanticFingerprint string, candidateID string, exactAnchorKey string, solution model.Solution) *model.ConstellationCandidateCompletionWitness {
	if len(solution.Placements) == 0 {
		return nil
	}
	placements := make([]model.ConstellationCandidateWitnessPlacement, 0, len(solution.Placements))
	for _, placement := range solution.Placements {
		placements = append(placements, model.ConstellationCandidateWitnessPlacement{
			InstanceID: placement.InstanceID,
			ItemID:     placement.ItemID,
			Rotation:   placement.Rotation,
			Origin:     placement.Origin,
		})
	}
	sort.Slice(placements, func(i, j int) bool { return placements[i].InstanceID < placements[j].InstanceID })
	return &model.ConstellationCandidateCompletionWitness{
		SchemaVersion:       "candidate-witness/v1",
		SemanticFingerprint: semanticFingerprint,
		CandidateID:         candidateID,
		ExactAnchorKey:      exactAnchorKey,
		Score:               cloneScore(solution.Evaluation.Score),
		LayoutKey:           solution.LayoutKey,
		CanonicalLayoutHash: solution.CanonicalLayoutHash,
		Placements:          placements,
	}
}

func constellationCandidateWitnessFromLayoutKey(layoutKeyValue string, suppliedFingerprint string, semanticFingerprint string, candidate constellationSkeleton, instances []model.InventoryInstance, optionsByInstance map[string][]model.Placement, catalog model.Catalog, config Config) (model.Solution, error) {
	if suppliedFingerprint == "" || suppliedFingerprint != semanticFingerprint {
		return model.Solution{}, fmt.Errorf("semantic fingerprint mismatch")
	}
	entries := strings.Split(strings.TrimSuffix(layoutKeyValue, ";"), ";")
	if len(entries) != len(instances) {
		return model.Solution{}, fmt.Errorf("witness layout must contain every inventory instance")
	}
	placements := make([]model.Placement, 0, len(entries))
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		parts := strings.Split(entry, "|")
		if len(parts) != 5 {
			return model.Solution{}, fmt.Errorf("invalid witness layout entry")
		}
		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 0 || index >= len(instances) {
			return model.Solution{}, fmt.Errorf("invalid witness inventory index")
		}
		if _, exists := seen[index]; exists {
			return model.Solution{}, fmt.Errorf("duplicate witness inventory index")
		}
		seen[index] = struct{}{}
		instance := instances[index]
		if instance.ItemID != parts[1] {
			return model.Solution{}, fmt.Errorf("witness item identity mismatch")
		}
		rotation, rotationErr := strconv.Atoi(parts[2])
		row, rowErr := strconv.Atoi(parts[3])
		col, colErr := strconv.Atoi(parts[4])
		if rotationErr != nil || rowErr != nil || colErr != nil {
			return model.Solution{}, fmt.Errorf("invalid witness placement coordinates")
		}
		found := false
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Rotation == rotation && option.Origin == (model.Coord{Row: row, Col: col}) {
				placements = append(placements, option)
				found = true
				break
			}
		}
		if !found {
			return model.Solution{}, fmt.Errorf("witness placement is unavailable")
		}
	}
	sortPlacementsByOriginal(placements)
	for _, anchor := range candidate.placed {
		matched := false
		for _, placement := range placements {
			if placement.InstanceID == anchor.InstanceID && placementKey(placement) == placementKey(anchor) {
				matched = true
				break
			}
		}
		if !matched {
			return model.Solution{}, fmt.Errorf("witness does not preserve candidate anchors")
		}
	}
	evaluation := evaluateLayoutForConfig(catalog, placements, config)
	solution := model.Solution{
		Placements:          placements,
		Evaluation:          evaluation,
		LayoutKey:           layoutKey(placements, instances),
		CanonicalLayoutHash: canonicalLayoutHash(placements),
	}
	if solution.LayoutKey != layoutKeyValue {
		return model.Solution{}, fmt.Errorf("witness layout key mismatch")
	}
	return solution, nil
}

func fmtMask(mask uint64) string {
	return fmt.Sprintf("%016x", mask)
}

func constellationRootPackingInputKey(sourceGeometryKey string, occupied uint64, free uint64, anchored []string, remaining []string) string {
	return strings.Join([]string{
		"source=" + sourceGeometryKey,
		"occupied=" + fmtMask(occupied),
		"free=" + fmtMask(free),
		"anchors=" + strings.Join(anchored, ","),
		"remaining=" + strings.Join(remaining, ","),
	}, "|")
}

func constellationPriorityLinks(placements []model.Placement, stars []model.StarActivation, sourceItems map[string]struct{}) []model.PlateauLink {
	placedByID := placementByInstanceID(placements)
	links := make([]model.PlateauLink, 0, len(stars))
	for _, star := range stars {
		source, exists := placedByID[star.SourceInstance]
		if !exists {
			continue
		}
		if _, priority := sourceItems[source.ItemID]; !priority {
			continue
		}
		links = append(links, model.PlateauLink{SourceInstance: star.SourceInstance, TargetInstance: star.TargetInstance, StarPosition: star.StarPosition})
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].SourceInstance != links[j].SourceInstance {
			return links[i].SourceInstance < links[j].SourceInstance
		}
		if links[i].TargetInstance != links[j].TargetInstance {
			return links[i].TargetInstance < links[j].TargetInstance
		}
		if links[i].StarPosition.Row != links[j].StarPosition.Row {
			return links[i].StarPosition.Row < links[j].StarPosition.Row
		}
		return links[i].StarPosition.Col < links[j].StarPosition.Col
	})
	return links
}

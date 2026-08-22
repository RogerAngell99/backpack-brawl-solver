package solver

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"backpack-brawl-solver/internal/model"
)

const (
	diagnosticPhaseCount                         = 14
	tracePhaseCoverageSeed                       = "coverage_seed"
	tracePhasePackingSeed                        = "packing_seed"
	tracePhaseStarSeed                           = "star_seed"
	tracePhaseConstellationSeed                  = "constellation_seed_v1"
	tracePhaseConstellationCandidateOptimization = "constellation_candidate_completion_optimization"
	tracePhaseConstellationForcedRootedPacking   = "constellation_forced_candidate_rooted_packing"
	tracePhaseConstellationParentFrontierHedge   = "constellation_parent_frontier_hedge"
	tracePhasePreRepair                          = "pre_repair"
	tracePhaseDFS                                = "dfs"
	tracePhasePostRepair                         = "post_repair"
	tracePhaseCompletion                         = "completion"
	tracePhaseRefine                             = "refine"
	tracePhasePlateauLNS                         = "plateau_lns"
	tracePhasePlateauRefine                      = "plateau_refine"
)

var tracePhases = []string{
	tracePhaseCoverageSeed,
	tracePhasePackingSeed,
	tracePhaseStarSeed,
	tracePhaseConstellationSeed,
	tracePhaseConstellationCandidateOptimization,
	tracePhaseConstellationForcedRootedPacking,
	tracePhaseConstellationParentFrontierHedge,
	tracePhasePreRepair,
	tracePhaseDFS,
	tracePhasePostRepair,
	tracePhaseCompletion,
	tracePhaseRefine,
	tracePhasePlateauLNS,
	tracePhasePlateauRefine,
}

func withTracePhase(config Config, phase string) Config {
	config.tracePhase = phase
	return config
}

type diagnosticTrace struct {
	started                   time.Time
	configuredBudget          int64
	ledger                    *nodeLedger
	stageID                   string
	priorityBounds            *priorityBoundContext
	starBounds                *starUpperBoundContext
	reference                 *model.Solution
	includeConstellationPhase bool

	charged      atomic.Int64
	uncharged    atomic.Int64
	phaseNode    [diagnosticPhaseCount]atomic.Int64
	phaseMove    [diagnosticPhaseCount]atomic.Int64
	phaseInvoked [diagnosticPhaseCount]atomic.Bool

	mu                        sync.Mutex
	events                    []model.IncumbentEvent
	phaseCandidates           [diagnosticPhaseCount]int64
	phaseBest                 [diagnosticPhaseCount]model.Score
	phaseHasBest              [diagnosticPhaseCount]bool
	phasePlans                [diagnosticPhaseCount]diagnosticPhasePlan
	hasBest                   bool
	bestScore                 model.Score
	firstCompleteRecorded     bool
	firstFullyPackedRecorded  bool
	firstPriorityRecorded     bool
	firstPriorityBudget       int64
	lastScoreImprovement      int64
	priorityEvaluations       int64
	priorityLayouts           map[[sha256.Size]byte]struct{}
	priorityStarHistogram     map[int]int64
	priorityBestScore         model.Score
	priorityHasBest           bool
	prioritySamples           []model.PlateauSample
	priorityLinkFrequency     map[string]int64
	prioritySourceFrequency   map[string]int64
	priorityTargetFrequency   map[string]int64
	priorityStarsBySource     map[string]int64
	priorityStarsBySourceItem map[string]int64
	priorityScoreFrequency    map[string]model.ScoreFrequency
	referenceEvaluations      int64
	minimumReferenceDistance  *model.ReferenceDistance
}

// diagnosticPhasePlan records scheduler decisions only. It intentionally does
// not reserve ledger capacity; charged work remains ledger-authoritative.
type diagnosticPhasePlan struct {
	planned           bool
	eligible          bool
	skipReason        string
	terminationReason string
	nodesReserved     int64
	nodesReturned     int64
	returnTarget      string
	startStageNodes   int64
	endStageNodes     int64
}

func newDiagnosticTrace(
	started time.Time,
	configuredBudget int64,
	ledger *nodeLedger,
	stageID string,
	priorityBounds *priorityBoundContext,
	starBounds *starUpperBoundContext,
	reference *model.Solution,
	includeConstellationPhase ...bool,
) *diagnosticTrace {
	includeConstellation := len(includeConstellationPhase) > 0 && includeConstellationPhase[0]
	return &diagnosticTrace{
		started:                   started,
		configuredBudget:          configuredBudget,
		ledger:                    ledger,
		stageID:                   stageID,
		priorityBounds:            priorityBounds,
		starBounds:                starBounds,
		reference:                 reference,
		includeConstellationPhase: includeConstellation,
		priorityLayouts:           make(map[[sha256.Size]byte]struct{}),
		priorityStarHistogram:     make(map[int]int64),
		priorityLinkFrequency:     make(map[string]int64),
		prioritySourceFrequency:   make(map[string]int64),
		priorityTargetFrequency:   make(map[string]int64),
		priorityStarsBySource:     make(map[string]int64),
		priorityStarsBySourceItem: make(map[string]int64),
		priorityScoreFrequency:    make(map[string]model.ScoreFrequency),
	}
}

func tracePhaseIndex(phase string) int {
	for index, candidate := range tracePhases {
		if phase == candidate {
			return index
		}
	}
	return -1
}

func (trace *diagnosticTrace) addCharged(phase string, delta int64) {
	if trace == nil || delta <= 0 {
		return
	}
	trace.charged.Add(delta)
	if index := tracePhaseIndex(phase); index >= 0 {
		trace.phaseNode[index].Add(delta)
		trace.phaseInvoked[index].Store(true)
	}
}

func (trace *diagnosticTrace) addUncharged(phase string, delta int64) {
	if trace == nil || delta <= 0 {
		return
	}
	trace.uncharged.Add(delta)
	if index := tracePhaseIndex(phase); index >= 0 {
		trace.phaseMove[index].Add(delta)
		trace.phaseInvoked[index].Store(true)
	}
}

func (trace *diagnosticTrace) planPhase(phase string, eligible bool, skipReason string, nodesReserved int64) {
	if trace == nil {
		return
	}
	index := tracePhaseIndex(phase)
	if index < 0 {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.phasePlans[index] = diagnosticPhasePlan{
		planned:         true,
		eligible:        eligible,
		skipReason:      skipReason,
		nodesReserved:   nodesReserved,
		startStageNodes: trace.charged.Load(),
	}
}

func (trace *diagnosticTrace) invokePhase(phase string) {
	if trace == nil {
		return
	}
	if index := tracePhaseIndex(phase); index >= 0 {
		trace.phaseInvoked[index].Store(true)
	}
}

func (trace *diagnosticTrace) finishPhase(phase string, terminationReason string, nodesReturned int64, returnTarget string) {
	if trace == nil {
		return
	}
	index := tracePhaseIndex(phase)
	if index < 0 {
		return
	}
	if nodesReturned < 0 {
		nodesReturned = 0
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	plan := trace.phasePlans[index]
	plan.planned = true
	plan.terminationReason = terminationReason
	plan.nodesReturned = nodesReturned
	plan.returnTarget = returnTarget
	plan.endStageNodes = trace.charged.Load()
	trace.phasePlans[index] = plan
}

// observeCandidate is deliberately independent from the throttled UI progress
// reporter. It runs at the candidate evaluation that changed the incumbent.
func (trace *diagnosticTrace) observeCandidate(
	phase string,
	placements []model.Placement,
	instances []model.InventoryInstance,
	score model.Score,
	terminal bool,
	evaluation *model.Evaluation,
) {
	if trace == nil {
		return
	}
	phaseIndex := tracePhaseIndex(phase)
	if phaseIndex < 0 {
		return
	}

	digest := canonicalLayoutDigest(placements)
	hash := hex.EncodeToString(digest[:])
	fullyPacked := len(placements) == len(instances)
	upperBounds := trace.starUpperBounds(placements, instances)

	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.phaseCandidates[phaseIndex]++
	if !trace.phaseHasBest[phaseIndex] || compareScores(score, trace.phaseBest[phaseIndex]) > 0 {
		trace.phaseBest[phaseIndex] = cloneScore(score)
		trace.phaseHasBest[phaseIndex] = true
	}

	priorityReached := trace.priorityBounds != nil && trace.priorityBounds.reached(score)
	if priorityReached {
		trace.priorityEvaluations++
		trace.priorityLayouts[digest] = struct{}{}
		trace.priorityStarHistogram[score.StarCount]++
		if !trace.priorityHasBest || compareScores(score, trace.priorityBestScore) > 0 {
			trace.priorityBestScore = cloneScore(score)
			trace.priorityHasBest = true
		}
		if evaluation != nil {
			trace.observePrioritySample(plateauSample(model.Solution{
				Placements:          append([]model.Placement(nil), placements...),
				Evaluation:          cloneEvaluation(*evaluation),
				LayoutKey:           layoutKey(placements, instances),
				CanonicalLayoutHash: hash,
			}))
		}
	}

	reasons := make([]string, 0, 3)
	if terminal && !trace.firstCompleteRecorded {
		trace.firstCompleteRecorded = true
		reasons = append(reasons, "first_complete")
	}
	if fullyPacked && !trace.firstFullyPackedRecorded {
		trace.firstFullyPackedRecorded = true
		reasons = append(reasons, "first_fully_packed")
	}
	if priorityReached && !trace.firstPriorityRecorded {
		trace.firstPriorityRecorded = true
		trace.firstPriorityBudget = trace.charged.Load()
		reasons = append(reasons, "first_priority_ceiling")
	}
	if !trace.hasBest || compareScores(score, trace.bestScore) > 0 {
		if len(reasons) == 0 {
			reasons = append(reasons, "score_improvement")
		}
		trace.hasBest = true
		trace.bestScore = cloneScore(score)
		trace.lastScoreImprovement = trace.charged.Load()
	}
	if len(reasons) == 0 {
		return
	}

	budget := trace.ledger.snapshot(trace.stageID)
	event := model.IncumbentEvent{
		Sequence:                int64(len(trace.events) + 1),
		Reasons:                 reasons,
		Phase:                   phase,
		GlobalBudgetConsumed:    trace.charged.Load(),
		StageBudgetConsumed:     budget.StageCharged,
		ExecutionBudgetConsumed: budget.ExecutionCharged,
		UnchargedWork:           trace.uncharged.Load(),
		ElapsedMS:               time.Since(trace.started).Milliseconds(),
		ConfiguredMaxNodes:      trace.configuredBudget,
		Score:                   cloneScore(score),
		LayoutKey:               layoutKey(placements, instances),
		CanonicalLayoutHash:     hash,
		StarUpperBounds:         upperBounds,
	}
	event.PhaseLocalNodes = trace.phaseNode[phaseIndex].Load()
	event.PhaseLocalMoves = trace.phaseMove[phaseIndex].Load()
	event.CompletionMoves = trace.phaseMove[tracePhaseIndex(tracePhaseCompletion)].Load()
	event.RefineMoves = trace.phaseMove[tracePhaseIndex(tracePhaseRefine)].Load()
	trace.events = append(trace.events, event)
}

func (trace *diagnosticTrace) starUpperBounds(placements []model.Placement, instances []model.InventoryInstance) model.StarUpperBounds {
	if trace == nil || trace.starBounds == nil {
		return model.StarUpperBounds{}
	}
	return trace.starBounds.forPlacements(placements, instances)
}

func (trace *diagnosticTrace) apply(stats *model.SearchStats) {
	if trace == nil || stats == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()

	stats.DiagnosticsEnabled = true
	stats.GlobalBudgetConsumed = trace.charged.Load()
	if trace.configuredBudget > stats.GlobalBudgetConsumed {
		stats.UnusedGlobalNodes = trace.configuredBudget - stats.GlobalBudgetConsumed
	}
	stats.UnchargedWork = trace.uncharged.Load()
	stats.IncumbentTrace = append([]model.IncumbentEvent(nil), trace.events...)
	stats.PhaseWork = make([]model.SearchPhaseWork, 0, len(tracePhases))
	for index, phase := range tracePhases {
		if (phase == tracePhaseConstellationSeed && !trace.includeConstellationPhase && !trace.phasePlans[index].planned) || ((phase == tracePhaseConstellationCandidateOptimization || phase == tracePhaseConstellationForcedRootedPacking) && !trace.phasePlans[index].planned) {
			continue
		}
		plan := trace.phasePlans[index]
		consumed := trace.phaseNode[index].Load()
		phaseWork := model.SearchPhaseWork{
			Phase:             phase,
			ChargedNodes:      consumed,
			UnchargedMoves:    trace.phaseMove[index].Load(),
			Candidates:        trace.phaseCandidates[index],
			Eligible:          plan.eligible,
			Invoked:           trace.phaseInvoked[index].Load(),
			SkipReason:        plan.skipReason,
			TerminationReason: plan.terminationReason,
			NodesReserved:     plan.nodesReserved,
			NodesConsumed:     consumed,
			NodesReturned:     plan.nodesReturned,
			ReturnTarget:      plan.returnTarget,
			StartStageNodes:   plan.startStageNodes,
			EndStageNodes:     plan.endStageNodes,
		}
		if trace.phaseHasBest[index] {
			bestScore := cloneScore(trace.phaseBest[index])
			phaseWork.BestScore = &bestScore
		}
		stats.PhaseWork = append(stats.PhaseWork, phaseWork)
	}
	stats.StarUpperBounds = trace.starUpperBounds(nil, nil)
	for _, event := range trace.events {
		for _, reason := range event.Reasons {
			switch reason {
			case "first_complete":
				stats.FirstCompletePhase = event.Phase
				stats.FirstCompleteNodes = event.GlobalBudgetConsumed
				stats.FirstCompleteMS = event.ElapsedMS
			case "first_fully_packed":
				stats.FirstFullyPackedPhase = event.Phase
				stats.FirstFullyPackedNodes = event.GlobalBudgetConsumed
				stats.FirstFullyPackedMS = event.ElapsedMS
			}
		}
	}
	if trace.priorityEvaluations > 0 {
		stats.PriorityCeilingStats = &model.PriorityCeilingStats{
			Evaluations:          trace.priorityEvaluations,
			CanonicalLayoutCount: len(trace.priorityLayouts),
			StarMin:              histogramPercentile(trace.priorityStarHistogram, trace.priorityEvaluations, 0),
			StarP50:              histogramPercentile(trace.priorityStarHistogram, trace.priorityEvaluations, 0.50),
			StarP90:              histogramPercentile(trace.priorityStarHistogram, trace.priorityEvaluations, 0.90),
			StarMax:              histogramPercentile(trace.priorityStarHistogram, trace.priorityEvaluations, 1),
			BestScore:            cloneScore(trace.priorityBestScore),
		}
	}
	if len(trace.prioritySamples) > 0 || trace.referenceEvaluations > 0 {
		archive := model.PlateauArchiveStats{
			Samples:                  append([]model.PlateauSample(nil), trace.prioritySamples...),
			LinkFrequency:            cloneFrequency(trace.priorityLinkFrequency),
			SourceFrequency:          cloneFrequency(trace.prioritySourceFrequency),
			TargetFrequency:          cloneFrequency(trace.priorityTargetFrequency),
			StarsBySource:            cloneFrequency(trace.priorityStarsBySource),
			StarsBySourceItem:        cloneFrequency(trace.priorityStarsBySourceItem),
			ScoreDistribution:        sortedScoreFrequency(trace.priorityScoreFrequency),
			ReferenceEvaluations:     trace.referenceEvaluations,
			MinimumReferenceDistance: cloneReferenceDistance(trace.minimumReferenceDistance),
		}
		stats.PlateauArchive = archive
	}
	stats.Plateau = trace.plateauLocked()
}

func (trace *diagnosticTrace) observePrioritySample(sample model.PlateauSample) {
	for _, link := range sample.LiteralLinks {
		key := link.SourceInstance + ">" + link.TargetInstance + "@" + strconv.Itoa(link.StarPosition.Row) + "," + strconv.Itoa(link.StarPosition.Col)
		trace.priorityLinkFrequency[key]++
		trace.prioritySourceFrequency[link.SourceInstance]++
		trace.priorityTargetFrequency[link.TargetInstance]++
	}
	for _, count := range sample.StarsBySource {
		trace.priorityStarsBySource[count.SourceInstance] += int64(count.TargetCount)
	}
	placements := placementByInstanceID(sample.Placements)
	for _, link := range sample.LiteralLinks {
		if source, exists := placements[link.SourceInstance]; exists {
			trace.priorityStarsBySourceItem[source.ItemID]++
		}
	}
	if trace.reference != nil {
		delta := applyReferenceDelta(&sample, *trace.reference)
		trace.referenceEvaluations++
		distance := referenceDistance(sample, delta)
		if trace.minimumReferenceDistance == nil || referenceDistanceLess(distance, *trace.minimumReferenceDistance) {
			trace.minimumReferenceDistance = &distance
		}
	}
	frequencyKey := scoreFrequencyKey(sample.Score)
	frequency := trace.priorityScoreFrequency[frequencyKey]
	frequency.Score = cloneScore(sample.Score)
	frequency.Count++
	trace.priorityScoreFrequency[frequencyKey] = frequency

	for index, existing := range trace.prioritySamples {
		if existing.CanonicalLayoutHash == sample.CanonicalLayoutHash {
			if plateauSampleLess(sample, existing) {
				trace.prioritySamples[index] = sample
			}
			return
		}
	}
	trace.prioritySamples = append(trace.prioritySamples, sample)
	sort.Slice(trace.prioritySamples, func(i, j int) bool { return plateauSampleLess(trace.prioritySamples[i], trace.prioritySamples[j]) })
	if len(trace.prioritySamples) > plateauDiagnosticSampleLimit {
		trace.prioritySamples = trace.prioritySamples[:plateauDiagnosticSampleLimit]
	}
}

func cloneReferenceDistance(distance *model.ReferenceDistance) *model.ReferenceDistance {
	if distance == nil {
		return nil
	}
	cloned := *distance
	return &cloned
}

func plateauSampleLess(left model.PlateauSample, right model.PlateauSample) bool {
	if compare := compareScores(left.Score, right.Score); compare != 0 {
		return compare > 0
	}
	if left.CanonicalLinkSignature != right.CanonicalLinkSignature {
		return left.CanonicalLinkSignature < right.CanonicalLinkSignature
	}
	return left.CanonicalLayoutHash < right.CanonicalLayoutHash
}

func cloneFrequency(values map[string]int64) map[string]int64 {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]int64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func sortedScoreFrequency(values map[string]model.ScoreFrequency) []model.ScoreFrequency {
	frequencies := make([]model.ScoreFrequency, 0, len(values))
	for _, frequency := range values {
		frequencies = append(frequencies, frequency)
	}
	sort.Slice(frequencies, func(i, j int) bool {
		if compare := compareScores(frequencies[i].Score, frequencies[j].Score); compare != 0 {
			return compare > 0
		}
		return scoreFrequencyKey(frequencies[i].Score) < scoreFrequencyKey(frequencies[j].Score)
	})
	return frequencies
}

func applyReferenceDelta(sample *model.PlateauSample, reference model.Solution) model.ReferenceDelta {
	if sample == nil {
		return model.ReferenceDelta{}
	}
	delta := calculateReferenceDelta(*sample, reference)
	sample.ReferenceDelta = &delta
	sample.DeltaToReference = scoreDeltaSummary(reference.Evaluation.Score, sample.Score)
	referenceLinks := make(map[string]model.PlateauLink, len(reference.Evaluation.Stars))
	referenceBySource := map[string]int{}
	for _, star := range reference.Evaluation.Stars {
		link := model.PlateauLink{SourceInstance: star.SourceInstance, TargetInstance: star.TargetInstance, StarPosition: star.StarPosition}
		referenceLinks[literalLinkToken(link)] = link
		referenceBySource[star.SourceInstance]++
	}
	candidateLinks := make(map[string]struct{}, len(sample.LiteralLinks))
	candidateBySource := map[string]int{}
	for _, link := range sample.LiteralLinks {
		candidateLinks[literalLinkToken(link)] = struct{}{}
		candidateBySource[link.SourceInstance]++
	}
	for key, link := range referenceLinks {
		if _, present := candidateLinks[key]; !present {
			sample.MissingReferenceLinks = append(sample.MissingReferenceLinks, link)
		}
	}
	sort.Slice(sample.MissingReferenceLinks, func(i, j int) bool {
		left := sample.MissingReferenceLinks[i]
		right := sample.MissingReferenceLinks[j]
		if left.SourceInstance != right.SourceInstance {
			return left.SourceInstance < right.SourceInstance
		}
		if left.TargetInstance != right.TargetInstance {
			return left.TargetInstance < right.TargetInstance
		}
		if left.StarPosition.Row != right.StarPosition.Row {
			return left.StarPosition.Row < right.StarPosition.Row
		}
		return left.StarPosition.Col < right.StarPosition.Col
	})
	sample.SourceDeltaToReference = make(map[string]int)
	for source, count := range candidateBySource {
		sample.SourceDeltaToReference[source] = count - referenceBySource[source]
	}
	for source, count := range referenceBySource {
		if _, present := sample.SourceDeltaToReference[source]; !present {
			sample.SourceDeltaToReference[source] = -count
		}
	}
	if len(sample.SourceDeltaToReference) == 0 {
		sample.SourceDeltaToReference = nil
	}
	return delta
}

type referencePlacementOrigin struct {
	itemID string
	origin model.Coord
}

func calculateReferenceDelta(sample model.PlateauSample, reference model.Solution) model.ReferenceDelta {
	referenceOrigins := placementCountsByOrigin(reference.Placements)
	candidateOrigins := placementCountsByOrigin(sample.Placements)
	referenceItems := make(map[string]int)
	candidateItems := make(map[string]int)
	matchingOrigins := make(map[string]int)
	for origin, referenceCount := range referenceOrigins {
		referenceItems[origin.itemID] += referenceCount
		if candidateCount := candidateOrigins[origin]; candidateCount < referenceCount {
			matchingOrigins[origin.itemID] += candidateCount
		} else {
			matchingOrigins[origin.itemID] += referenceCount
		}
	}
	for origin, candidateCount := range candidateOrigins {
		candidateItems[origin.itemID] += candidateCount
	}
	moved := 0
	for itemID, referenceCount := range referenceItems {
		candidateCount := candidateItems[itemID]
		if referenceCount > candidateCount {
			moved += referenceCount - matchingOrigins[itemID]
		} else {
			moved += candidateCount - matchingOrigins[itemID]
		}
	}
	for itemID, candidateCount := range candidateItems {
		if _, present := referenceItems[itemID]; !present {
			moved += candidateCount
		}
	}

	referenceRotations := placementRotationsByOrigin(reference.Placements)
	candidateRotations := placementRotationsByOrigin(sample.Placements)
	rotationChanges := 0
	for origin, referenceCounts := range referenceRotations {
		candidateCounts := candidateRotations[origin]
		referenceTotal := 0
		for _, count := range referenceCounts {
			referenceTotal += count
		}
		matchingRotations := 0
		for rotation, count := range referenceCounts {
			candidateCount := candidateCounts[rotation]
			if candidateCount < count {
				matchingRotations += candidateCount
			} else {
				matchingRotations += count
			}
		}
		candidateTotal := 0
		for _, count := range candidateCounts {
			candidateTotal += count
		}
		sharedTotal := referenceTotal
		if candidateTotal < sharedTotal {
			sharedTotal = candidateTotal
		}
		rotationChanges += sharedTotal - matchingRotations
	}

	exactLost, exactGained := linkTokenDifference(
		literalLinkTokens(literalLinks(reference.Evaluation.Stars)),
		literalLinkTokens(sample.LiteralLinks),
	)
	canonicalLost, canonicalGained := linkTokenDifference(
		canonicalLinkTokens(reference.Placements, reference.Evaluation.Stars),
		canonicalPlateauLinkTokens(sample.Placements, sample.LiteralLinks),
	)
	return model.ReferenceDelta{
		MovedItems:              moved,
		RotationChanges:         rotationChanges,
		ExactLiteralLinksLost:   exactLost,
		ExactLiteralLinksGained: exactGained,
		CanonicalLinksLost:      canonicalLost,
		CanonicalLinksGained:    canonicalGained,
		StructuralDistance:      moved + rotationChanges + canonicalLost + canonicalGained,
	}
}

func placementCountsByOrigin(placements []model.Placement) map[referencePlacementOrigin]int {
	counts := make(map[referencePlacementOrigin]int, len(placements))
	for _, placement := range placements {
		counts[referencePlacementOrigin{itemID: placement.ItemID, origin: placement.Origin}]++
	}
	return counts
}

func placementRotationsByOrigin(placements []model.Placement) map[referencePlacementOrigin]map[int]int {
	rotations := make(map[referencePlacementOrigin]map[int]int, len(placements))
	for _, placement := range placements {
		origin := referencePlacementOrigin{itemID: placement.ItemID, origin: placement.Origin}
		if rotations[origin] == nil {
			rotations[origin] = make(map[int]int)
		}
		rotations[origin][placement.Rotation]++
	}
	return rotations
}

func linkTokenDifference(reference []string, candidate []string) (lost int, gained int) {
	referenceCounts := make(map[string]int, len(reference))
	for _, token := range reference {
		referenceCounts[token]++
	}
	candidateCounts := make(map[string]int, len(candidate))
	for _, token := range candidate {
		candidateCounts[token]++
	}
	for token, referenceCount := range referenceCounts {
		if candidateCount := candidateCounts[token]; candidateCount < referenceCount {
			lost += referenceCount - candidateCount
		}
	}
	for token, candidateCount := range candidateCounts {
		if referenceCount := referenceCounts[token]; referenceCount < candidateCount {
			gained += candidateCount - referenceCount
		}
	}
	return lost, gained
}

func referenceDistance(sample model.PlateauSample, delta model.ReferenceDelta) model.ReferenceDistance {
	return model.ReferenceDistance{
		Delta:               delta,
		LayoutKey:           sample.LayoutKey,
		CanonicalLayoutHash: sample.CanonicalLayoutHash,
	}
}

func referenceDistanceLess(left model.ReferenceDistance, right model.ReferenceDistance) bool {
	if left.Delta.StructuralDistance != right.Delta.StructuralDistance {
		return left.Delta.StructuralDistance < right.Delta.StructuralDistance
	}
	leftCanonicalDelta := left.Delta.CanonicalLinksLost + left.Delta.CanonicalLinksGained
	rightCanonicalDelta := right.Delta.CanonicalLinksLost + right.Delta.CanonicalLinksGained
	if leftCanonicalDelta != rightCanonicalDelta {
		return leftCanonicalDelta < rightCanonicalDelta
	}
	if left.LayoutKey != right.LayoutKey {
		return left.LayoutKey < right.LayoutKey
	}
	return left.CanonicalLayoutHash < right.CanonicalLayoutHash
}

func (trace *diagnosticTrace) plateauLocked() model.PlateauStats {
	if trace.configuredBudget <= 0 || !trace.firstPriorityRecorded {
		return model.PlateauStats{}
	}
	firstPercent := float64(trace.firstPriorityBudget) * 100 / float64(trace.configuredBudget)
	lastPercent := float64(trace.lastScoreImprovement) * 100 / float64(trace.configuredBudget)
	timing := "middle"
	if firstPercent < 25 {
		timing = "early"
	} else if firstPercent > 75 {
		timing = "late"
	}
	return model.PlateauStats{
		FirstPriorityCeilingBudgetPercent: firstPercent,
		LastScoreImprovementBudgetPercent: lastPercent,
		Timing:                            timing,
		Strong:                            firstPercent <= 40 && lastPercent <= 40 && len(trace.priorityLayouts) >= 8,
	}
}

func histogramPercentile(histogram map[int]int64, total int64, percentile float64) int {
	if total <= 0 || len(histogram) == 0 {
		return 0
	}
	values := make([]int, 0, len(histogram))
	for value := range histogram {
		values = append(values, value)
	}
	sort.Ints(values)
	threshold := int64(math.Ceil(float64(total) * percentile))
	if threshold < 1 {
		threshold = 1
	}
	var seen int64
	for _, value := range values {
		seen += histogram[value]
		if seen >= threshold {
			return value
		}
	}
	return values[len(values)-1]
}

func cloneScore(score model.Score) model.Score {
	score.PriorityCounts = append([]int(nil), score.PriorityCounts...)
	return score
}

func canonicalLayoutDigest(placements []model.Placement) [sha256.Size]byte {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		parts = append(parts, canonicalPlacementKey(placement))
	}
	sort.Strings(parts)
	return sha256.Sum256([]byte(strings.Join(parts, ";")))
}

func canonicalLayoutHash(placements []model.Placement) string {
	digest := canonicalLayoutDigest(placements)
	return hex.EncodeToString(digest[:])
}

// canonicalPlacementKey excludes copy labels and rotations. Cells and star
// positions retain the physical information used by scoring and symmetry.
func canonicalPlacementKey(placement model.Placement) string {
	cells := append([]model.Coord(nil), placement.Cells...)
	stars := make([]model.Coord, 0, len(placement.StarPositions))
	for _, star := range placement.StarPositions {
		stars = append(stars, star.Position)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Row != cells[j].Row {
			return cells[i].Row < cells[j].Row
		}
		return cells[i].Col < cells[j].Col
	})
	sort.Slice(stars, func(i, j int) bool {
		if stars[i].Row != stars[j].Row {
			return stars[i].Row < stars[j].Row
		}
		return stars[i].Col < stars[j].Col
	})
	var builder strings.Builder
	builder.Grow(len(placement.ItemID) + len(cells)*8 + len(stars)*8 + 4)
	builder.WriteString(placement.ItemID)
	builder.WriteByte('|')
	for _, cell := range cells {
		builder.WriteString(strconv.Itoa(cell.Row))
		builder.WriteByte(',')
		builder.WriteString(strconv.Itoa(cell.Col))
		builder.WriteByte(';')
	}
	builder.WriteByte('|')
	for _, star := range stars {
		builder.WriteString(strconv.Itoa(star.Row))
		builder.WriteByte(',')
		builder.WriteString(strconv.Itoa(star.Col))
		builder.WriteByte(';')
	}
	return builder.String()
}

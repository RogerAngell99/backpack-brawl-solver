package solver

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"backpack-brawl-solver/internal/model"
)

const (
	plateauArchiveCapacity             = 32
	plateauDiagnosticSampleLimit       = 64
	plateauLNSBudgetPercent      int64 = 10
	plateauRefineBudgetPercent   int64 = 5
	plateauWalkMaxDepth                = 3
)

type plateauArchiveEntry struct {
	solution  model.Solution
	signature string
	origin    string
}

// plateauArchive is intentionally independent from diagnostics. It retains
// only priority-ceiling candidates and is never an incumbent tie breaker.
type plateauArchive struct {
	mu                    sync.Mutex
	priorityBounds        *priorityBoundContext
	entries               []plateauArchiveEntry
	capacity              int
	admissions            int64
	rejections            int64
	baseOrigins           map[string]struct{}
	operatorStats         map[string]model.PlateauOperatorStats
	ubBuckets             map[int]model.UBStarsBucket
	partialUBBuckets      map[string]model.PartialUBStarsBucket
	closureStats          map[int]model.PlateauClosureStats
	closureMandatorySizes map[int]map[int]int64
	closureOptionalSizes  map[int]map[int]int64
	levelStats            map[string]model.PlateauLevelStats
}

func newPlateauArchive(priorityBounds *priorityBoundContext) *plateauArchive {
	return newPlateauArchiveWithCapacity(priorityBounds, plateauArchiveCapacity)
}

func newPlateauArchiveWithCapacity(priorityBounds *priorityBoundContext, capacity int) *plateauArchive {
	if priorityBounds == nil {
		return nil
	}
	if capacity <= 0 {
		capacity = plateauArchiveCapacity
	}
	return &plateauArchive{
		priorityBounds:        priorityBounds,
		capacity:              capacity,
		baseOrigins:           make(map[string]struct{}),
		operatorStats:         make(map[string]model.PlateauOperatorStats),
		ubBuckets:             make(map[int]model.UBStarsBucket),
		partialUBBuckets:      make(map[string]model.PartialUBStarsBucket),
		closureStats:          make(map[int]model.PlateauClosureStats),
		closureMandatorySizes: make(map[int]map[int]int64),
		closureOptionalSizes:  make(map[int]map[int]int64),
		levelStats:            make(map[string]model.PlateauLevelStats),
	}
}

func (archive *plateauArchive) observe(solution model.Solution, origin string) {
	if archive == nil || !archive.priorityBounds.reached(solution.Evaluation.Score) {
		return
	}
	entry := plateauArchiveEntry{
		solution:  clonePlateauSolution(solution),
		signature: canonicalLinkSignature(solution.Placements, solution.Evaluation.Stars),
		origin:    origin,
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	for _, existing := range archive.entries {
		if existing.solution.CanonicalLayoutHash == entry.solution.CanonicalLayoutHash {
			archive.rejections++
			return
		}
	}
	archive.entries = append(archive.entries, entry)
	archive.entries = selectPlateauEntries(archive.entries, archive.capacity)
	for _, retained := range archive.entries {
		if retained.solution.CanonicalLayoutHash == entry.solution.CanonicalLayoutHash {
			archive.admissions++
			return
		}
	}
	archive.rejections++
}

func (archive *plateauArchive) bases() []model.Solution {
	if archive == nil {
		return nil
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	bases := make([]model.Solution, 0, len(archive.entries))
	for _, entry := range archive.entries {
		bases = append(bases, clonePlateauSolution(entry.solution))
		archive.baseOrigins[entry.origin] = struct{}{}
	}
	return bases
}

func (archive *plateauArchive) recordOperator(operator string, size int, result repairResult, best model.Score) {
	if archive == nil {
		return
	}
	key := operator + "|" + strconv.Itoa(size)
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := archive.operatorStats[key]
	stats.Operator = operator
	stats.NeighborhoodSize = size
	stats.Nodes += result.NodesExplored
	stats.PriorityPreservingCandidates += result.PriorityPreservingCandidates
	stats.PriorityBoundPruned += result.PriorityBoundPruned
	stats.CompletedBelowPriority += result.CompletedBelowPriority
	stats.PriorityPreservingComplete += result.PriorityPreservingCandidates
	stats.CompareScoreImprovement += result.CompareScoreImprovements
	if compareScores(best, stats.BestScore) > 0 {
		stats.BestScore = cloneScore(best)
	}
	archive.operatorStats[key] = stats
}

func (archive *plateauArchive) recordPartialUB(fixedStars int, partialUBStars int, incumbentStars int, improved bool) {
	if archive == nil {
		return
	}
	headroom := partialUBStars - fixedStars
	if headroom < 0 {
		headroom = 0
	}
	overIncumbent := partialUBStars - incumbentStars
	key := strconv.Itoa(headroom) + "|" + strconv.Itoa(overIncumbent)
	archive.mu.Lock()
	defer archive.mu.Unlock()
	bucket := archive.partialUBBuckets[key]
	bucket.FixedStars = fixedStars
	bucket.PartialUBStars = partialUBStars
	bucket.Headroom = headroom
	bucket.OverIncumbent = overIncumbent
	bucket.Candidates++
	if improved {
		bucket.ImprovedCandidates++
	}
	archive.partialUBBuckets[key] = bucket
}

func (archive *plateauArchive) recordClosure(size int, mandatory int, optional int, tooLarge bool) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := archive.closureStats[size]
	stats.NeighborhoodSize = size
	if stats.Attempts == 0 || mandatory < stats.MandatorySizeMin {
		stats.MandatorySizeMin = mandatory
	}
	if mandatory > stats.MandatorySizeMax {
		stats.MandatorySizeMax = mandatory
	}
	if stats.Attempts == 0 || optional < stats.OptionalSizeMin {
		stats.OptionalSizeMin = optional
	}
	if optional > stats.OptionalSizeMax {
		stats.OptionalSizeMax = optional
	}
	stats.Attempts++
	if archive.closureMandatorySizes[size] == nil {
		archive.closureMandatorySizes[size] = make(map[int]int64)
	}
	archive.closureMandatorySizes[size][mandatory]++
	if archive.closureOptionalSizes[size] == nil {
		archive.closureOptionalSizes[size] = make(map[int]int64)
	}
	archive.closureOptionalSizes[size][optional]++
	if tooLarge {
		stats.ClosureTooLarge++
	}
	archive.closureStats[size] = stats
}

func (archive *plateauArchive) recordUniqueClosure(size int) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := archive.closureStats[size]
	stats.NeighborhoodSize = size
	stats.UniqueClosures++
	archive.closureStats[size] = stats
}

func (archive *plateauArchive) recordPriorityFeasibleClosures(size int, count int) {
	if archive == nil || count <= 0 {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := archive.closureStats[size]
	stats.NeighborhoodSize = size
	stats.PriorityFeasible += int64(count)
	archive.closureStats[size] = stats
}

func (archive *plateauArchive) recordEnqueuedClosure(size int) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := archive.closureStats[size]
	stats.NeighborhoodSize = size
	stats.Enqueued++
	archive.closureStats[size] = stats
}

func plateauLevelStatsKey(level PlateauLevelPolicy) string {
	return strings.Join([]string{
		strconv.Itoa(level.MaxNeighborhoodSize),
		strconv.Itoa(level.MinMandatorySize),
		strconv.Itoa(level.MaxMandatorySize),
		strconv.FormatInt(level.QuotaBps, 10),
		strconv.Itoa(level.MaxSelected),
		strconv.Itoa(level.MaxSelectedPerBase),
		strconv.FormatInt(level.MinNodeBudget, 10),
	}, "|")
}

func plateauLevelStatsFor(statsByLevel map[string]model.PlateauLevelStats, level PlateauLevelPolicy) (string, model.PlateauLevelStats) {
	key := plateauLevelStatsKey(level)
	stats := statsByLevel[key]
	stats.MaxNeighborhoodSize = level.MaxNeighborhoodSize
	stats.MinMandatorySize = level.MinMandatorySize
	stats.MaxMandatorySize = level.MaxMandatorySize
	stats.QuotaBps = level.QuotaBps
	stats.MaxSelected = level.MaxSelected
	stats.MaxSelectedPerBase = level.MaxSelectedPerBase
	stats.MinNodeBudget = level.MinNodeBudget
	return key, stats
}

func (archive *plateauArchive) recordLevelClosure(level PlateauLevelPolicy, mandatory int) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	key, stats := plateauLevelStatsFor(archive.levelStats, level)
	if mandatory < level.MinMandatorySize {
		stats.RejectedBelowBand++
	} else if level.MaxMandatorySize > 0 && mandatory > level.MaxMandatorySize {
		stats.RejectedAboveBand++
	} else {
		stats.CandidatesMatchingBand++
	}
	archive.levelStats[key] = stats
}

func (archive *plateauArchive) recordLevelSelection(level PlateauLevelPolicy, selected int, perBaseDrops int, selectedCapDrops int) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	key, stats := plateauLevelStatsFor(archive.levelStats, level)
	stats.Selected += int64(selected)
	stats.PerBaseDrops += int64(perBaseDrops)
	stats.SelectedCapDrops += int64(selectedCapDrops)
	archive.levelStats[key] = stats
}

func (archive *plateauArchive) recordLevelQuota(level PlateauLevelPolicy, allocated int64, consumed int64, carried int64) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	key, stats := plateauLevelStatsFor(archive.levelStats, level)
	stats.QuotaAllocated += allocated
	stats.QuotaConsumed += consumed
	stats.QuotaCarried += carried
	archive.levelStats[key] = stats
}

func (archive *plateauArchive) recordLevelWork(level PlateauLevelPolicy, nodes int64, improvements int) {
	if archive == nil {
		return
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	key, stats := plateauLevelStatsFor(archive.levelStats, level)
	stats.Tasks++
	stats.Nodes += nodes
	stats.Improvements += int64(improvements)
	archive.levelStats[key] = stats
}

func (archive *plateauArchive) recordUBStars(realized int, bound int, promising bool, improved bool) {
	if archive == nil {
		return
	}
	gap := bound - realized
	archive.mu.Lock()
	defer archive.mu.Unlock()
	bucket := archive.ubBuckets[gap]
	bucket.Gap = gap
	bucket.Candidates++
	if promising {
		bucket.PromisingCandidates++
	}
	if improved {
		bucket.ImprovedCandidates++
	}
	archive.ubBuckets[gap] = bucket
}

func (archive *plateauArchive) stats() model.PlateauArchiveStats {
	if archive == nil {
		return model.PlateauArchiveStats{}
	}
	archive.mu.Lock()
	defer archive.mu.Unlock()
	stats := model.PlateauArchiveStats{
		Capacity:   archive.capacity,
		Size:       len(archive.entries),
		Admissions: archive.admissions,
		Rejections: archive.rejections,
	}
	signatures := map[string]struct{}{}
	for _, entry := range archive.entries {
		signatures[entry.signature] = struct{}{}
	}
	stats.SignatureDiversity = len(signatures)
	for origin := range archive.baseOrigins {
		stats.BaseOrigins = append(stats.BaseOrigins, origin)
	}
	sort.Strings(stats.BaseOrigins)
	for _, value := range archive.operatorStats {
		stats.OperatorStats = append(stats.OperatorStats, value)
	}
	sort.Slice(stats.OperatorStats, func(i, j int) bool {
		if stats.OperatorStats[i].Operator != stats.OperatorStats[j].Operator {
			return stats.OperatorStats[i].Operator < stats.OperatorStats[j].Operator
		}
		return stats.OperatorStats[i].NeighborhoodSize < stats.OperatorStats[j].NeighborhoodSize
	})
	for _, value := range archive.ubBuckets {
		stats.UBStarsBuckets = append(stats.UBStarsBuckets, value)
	}
	sort.Slice(stats.UBStarsBuckets, func(i, j int) bool { return stats.UBStarsBuckets[i].Gap < stats.UBStarsBuckets[j].Gap })
	for _, value := range archive.partialUBBuckets {
		stats.PartialUBStarsBuckets = append(stats.PartialUBStarsBuckets, value)
	}
	sort.Slice(stats.PartialUBStarsBuckets, func(i, j int) bool {
		if stats.PartialUBStarsBuckets[i].Headroom != stats.PartialUBStarsBuckets[j].Headroom {
			return stats.PartialUBStarsBuckets[i].Headroom < stats.PartialUBStarsBuckets[j].Headroom
		}
		return stats.PartialUBStarsBuckets[i].OverIncumbent < stats.PartialUBStarsBuckets[j].OverIncumbent
	})
	for size, value := range archive.closureStats {
		value.MandatorySizeHistogram = closureSizeHistogram(archive.closureMandatorySizes[size])
		value.OptionalSizeHistogram = closureSizeHistogram(archive.closureOptionalSizes[size])
		stats.ClosureStats = append(stats.ClosureStats, value)
	}
	sort.Slice(stats.ClosureStats, func(i, j int) bool {
		return stats.ClosureStats[i].NeighborhoodSize < stats.ClosureStats[j].NeighborhoodSize
	})
	for _, value := range archive.levelStats {
		stats.LevelStats = append(stats.LevelStats, value)
	}
	sort.Slice(stats.LevelStats, func(i, j int) bool {
		if stats.LevelStats[i].MaxNeighborhoodSize != stats.LevelStats[j].MaxNeighborhoodSize {
			return stats.LevelStats[i].MaxNeighborhoodSize < stats.LevelStats[j].MaxNeighborhoodSize
		}
		if stats.LevelStats[i].MinMandatorySize != stats.LevelStats[j].MinMandatorySize {
			return stats.LevelStats[i].MinMandatorySize < stats.LevelStats[j].MinMandatorySize
		}
		if stats.LevelStats[i].MaxMandatorySize != stats.LevelStats[j].MaxMandatorySize {
			return stats.LevelStats[i].MaxMandatorySize < stats.LevelStats[j].MaxMandatorySize
		}
		if stats.LevelStats[i].QuotaBps != stats.LevelStats[j].QuotaBps {
			return stats.LevelStats[i].QuotaBps < stats.LevelStats[j].QuotaBps
		}
		if stats.LevelStats[i].MaxSelected != stats.LevelStats[j].MaxSelected {
			return stats.LevelStats[i].MaxSelected < stats.LevelStats[j].MaxSelected
		}
		if stats.LevelStats[i].MaxSelectedPerBase != stats.LevelStats[j].MaxSelectedPerBase {
			return stats.LevelStats[i].MaxSelectedPerBase < stats.LevelStats[j].MaxSelectedPerBase
		}
		return stats.LevelStats[i].MinNodeBudget < stats.LevelStats[j].MinNodeBudget
	})
	return stats
}

func closureSizeHistogram(values map[int]int64) []model.ClosureSizeBucket {
	if len(values) == 0 {
		return nil
	}
	buckets := make([]model.ClosureSizeBucket, 0, len(values))
	for size, count := range values {
		buckets = append(buckets, model.ClosureSizeBucket{Size: size, Count: count})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Size < buckets[j].Size })
	return buckets
}

func selectPlateauEntries(entries []plateauArchiveEntry, capacity int) []plateauArchiveEntry {
	if len(entries) <= capacity {
		sortPlateauEntries(entries)
		return entries
	}
	bySignature := make(map[string][]plateauArchiveEntry)
	for _, entry := range entries {
		bySignature[entry.signature] = append(bySignature[entry.signature], entry)
	}
	var representatives []plateauArchiveEntry
	for _, group := range bySignature {
		sortPlateauEntries(group)
		representatives = append(representatives, group[0])
	}
	sortPlateauEntries(representatives)
	selected := make([]plateauArchiveEntry, 0, capacity)
	seenHash := make(map[string]struct{}, capacity)
	for _, entry := range representatives {
		if len(selected) >= capacity {
			break
		}
		selected = append(selected, entry)
		seenHash[entry.solution.CanonicalLayoutHash] = struct{}{}
	}
	sortPlateauEntries(entries)
	for _, entry := range entries {
		if len(selected) >= capacity {
			break
		}
		if _, exists := seenHash[entry.solution.CanonicalLayoutHash]; exists {
			continue
		}
		selected = append(selected, entry)
		seenHash[entry.solution.CanonicalLayoutHash] = struct{}{}
	}
	sortPlateauEntries(selected)
	return selected
}

func sortPlateauEntries(entries []plateauArchiveEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if compare := compareScores(entries[i].solution.Evaluation.Score, entries[j].solution.Evaluation.Score); compare != 0 {
			return compare > 0
		}
		if entries[i].signature != entries[j].signature {
			return entries[i].signature < entries[j].signature
		}
		return entries[i].solution.CanonicalLayoutHash < entries[j].solution.CanonicalLayoutHash
	})
}

func clonePlateauSolution(solution model.Solution) model.Solution {
	cloned := solution
	cloned.Placements = append([]model.Placement(nil), solution.Placements...)
	cloned.Evaluation = cloneEvaluation(solution.Evaluation)
	return cloned
}

func cloneEvaluation(evaluation model.Evaluation) model.Evaluation {
	cloned := evaluation
	cloned.Score = cloneScore(evaluation.Score)
	cloned.Crafts = append([]model.CraftActivation(nil), evaluation.Crafts...)
	cloned.Stars = append([]model.StarActivation(nil), evaluation.Stars...)
	return cloned
}

func canonicalLinkSignature(placements []model.Placement, stars []model.StarActivation) string {
	return strings.Join(canonicalLinkTokens(placements, stars), ";")
}

func canonicalLinkTokens(placements []model.Placement, stars []model.StarActivation) []string {
	physicalIDs := physicalInstanceIDs(placements)
	parts := make([]string, 0, len(stars))
	for _, star := range stars {
		parts = append(parts, canonicalLinkToken(star.SourceInstance, star.TargetInstance, star.StarPosition, physicalIDs))
	}
	sort.Strings(parts)
	return parts
}

func canonicalPlateauLinkTokens(placements []model.Placement, links []model.PlateauLink) []string {
	physicalIDs := physicalInstanceIDs(placements)
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, canonicalLinkToken(link.SourceInstance, link.TargetInstance, link.StarPosition, physicalIDs))
	}
	sort.Strings(parts)
	return parts
}

func canonicalLinkToken(sourceInstance string, targetInstance string, starPosition model.Coord, physicalIDs map[string]string) string {
	return physicalIDs[sourceInstance] + ">" + physicalIDs[targetInstance] + "@" + strconv.Itoa(starPosition.Row) + "," + strconv.Itoa(starPosition.Col)
}

func literalLinkToken(link model.PlateauLink) string {
	return link.SourceInstance + ">" + link.TargetInstance + "@" + strconv.Itoa(link.StarPosition.Row) + "," + strconv.Itoa(link.StarPosition.Col)
}

func literalLinks(stars []model.StarActivation) []model.PlateauLink {
	links := make([]model.PlateauLink, 0, len(stars))
	for _, star := range stars {
		links = append(links, model.PlateauLink{SourceInstance: star.SourceInstance, TargetInstance: star.TargetInstance, StarPosition: star.StarPosition})
	}
	return links
}

func literalLinkTokens(links []model.PlateauLink) []string {
	tokens := make([]string, 0, len(links))
	for _, link := range links {
		tokens = append(tokens, literalLinkToken(link))
	}
	sort.Strings(tokens)
	return tokens
}

func physicalInstanceIDs(placements []model.Placement) map[string]string {
	byItem := make(map[string][]model.Placement)
	for _, placement := range placements {
		byItem[placement.ItemID] = append(byItem[placement.ItemID], placement)
	}
	ids := make(map[string]string, len(placements))
	for itemID, group := range byItem {
		sort.Slice(group, func(i, j int) bool {
			left, right := canonicalPlacementKey(group[i]), canonicalPlacementKey(group[j])
			if left != right {
				return left < right
			}
			return group[i].OriginalIndex < group[j].OriginalIndex
		})
		for index, placement := range group {
			ids[placement.InstanceID] = itemID + "@" + strconv.Itoa(index)
		}
	}
	return ids
}

func plateauSample(solution model.Solution) model.PlateauSample {
	links := literalLinks(solution.Evaluation.Stars)
	bySource := map[string]int{}
	bySourceItem := map[string]int{}
	placements := placementByInstanceID(solution.Placements)
	for _, star := range solution.Evaluation.Stars {
		bySource[star.SourceInstance]++
		if source, exists := placements[star.SourceInstance]; exists {
			bySourceItem[source.ItemID]++
		}
	}
	sourceCounts := make([]model.StarInstanceTargetCount, 0, len(bySource))
	for source, count := range bySource {
		sourceCounts = append(sourceCounts, model.StarInstanceTargetCount{SourceInstance: source, TargetCount: count})
	}
	sort.Slice(sourceCounts, func(i, j int) bool { return sourceCounts[i].SourceInstance < sourceCounts[j].SourceInstance })
	return model.PlateauSample{
		Score:                  cloneScore(solution.Evaluation.Score),
		Placements:             append([]model.Placement(nil), solution.Placements...),
		LayoutKey:              solution.LayoutKey,
		CanonicalLayoutHash:    solution.CanonicalLayoutHash,
		LiteralLinks:           links,
		CanonicalLinkSignature: canonicalLinkSignature(solution.Placements, solution.Evaluation.Stars),
		StarsBySource:          sourceCounts,
		StarsBySourceItem:      bySourceItem,
	}
}

func scoreFrequencyKey(score model.Score) string {
	return strings.Join([]string{
		strings.Join(fmtInts(score.PriorityCounts), ","),
		strconv.Itoa(score.CraftCount), strconv.Itoa(score.StarCount), strconv.Itoa(score.ItemCount),
		strconv.Itoa(score.StarTargetBreadth), strconv.Itoa(score.StarReciprocalPairs), strconv.Itoa(score.StarSourceDefinitionDiversity),
	}, "|")
}

func fmtInts(values []int) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return parts
}

func configurationFingerprint(config Config) string {
	if config.executionFingerprint != "" {
		return config.executionFingerprint
	}
	return resolvedPolicyFingerprint(policyForConfig(config))
}

func observeSearchCandidate(
	catalog model.Catalog,
	placements []model.Placement,
	instances []model.InventoryInstance,
	score model.Score,
	terminal bool,
	config Config,
) {
	var evaluation *model.Evaluation
	if config.priorityBounds != nil && config.priorityBounds.reached(score) && (config.plateauArchive != nil || config.trace != nil) {
		computed := evaluateLayoutForConfig(catalog, placements, config)
		evaluation = &computed
		solution := model.Solution{
			Placements:          append([]model.Placement(nil), placements...),
			Evaluation:          computed,
			LayoutKey:           layoutKey(placements, instances),
			CanonicalLayoutHash: canonicalLayoutHash(placements),
		}
		origin := config.tracePhase
		if rootID := config.constellationRootOrigins[solution.CanonicalLayoutHash]; rootID != "" {
			origin += "|" + rootID
		}
		config.plateauArchive.observe(solution, origin)
	}
	config.trace.observeCandidate(config.tracePhase, placements, instances, score, terminal, evaluation)
}

func applyPlateauSearchStats(stats *model.SearchStats, config Config) {
	if stats == nil {
		return
	}
	stats.ConfigFingerprint = configurationFingerprint(config)
	stats.ExecutionFingerprint = config.executionFingerprint
	algorithmic := config.plateauArchive.stats()
	if config.Diagnostics {
		algorithmic.Samples = stats.PlateauArchive.Samples
		algorithmic.LinkFrequency = stats.PlateauArchive.LinkFrequency
		algorithmic.SourceFrequency = stats.PlateauArchive.SourceFrequency
		algorithmic.TargetFrequency = stats.PlateauArchive.TargetFrequency
		algorithmic.StarsBySource = stats.PlateauArchive.StarsBySource
		algorithmic.StarsBySourceItem = stats.PlateauArchive.StarsBySourceItem
		algorithmic.ScoreDistribution = stats.PlateauArchive.ScoreDistribution
		algorithmic.ReferenceEvaluations = stats.PlateauArchive.ReferenceEvaluations
		algorithmic.MinimumReferenceDistance = cloneReferenceDistance(stats.PlateauArchive.MinimumReferenceDistance)
	}
	stats.PlateauArchive = algorithmic
}

func plateauTieBreakLNS(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	incumbent model.Solution,
	config Config,
	coverage *coverageContext,
	gridMask uint64,
	nodeBudget int64,
) repairResult {
	if nodeBudget <= 0 || config.plateauArchive == nil || config.priorityBounds == nil || !config.priorityBounds.reached(incumbent.Evaluation.Score) {
		return repairResult{}
	}
	bases := config.plateauArchive.bases()
	if len(bases) == 0 {
		return repairResult{}
	}
	instanceByID := make(map[string]model.InventoryInstance, len(instances))
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}
	result := repairResult{Solutions: []model.Solution{incumbent}}
	remaining := nodeBudget
	carriedQuota := int64(0)
	policy := policyForConfig(config)
	type preparedPlateauLevel struct {
		neighborhoods []repairNeighborhood
		potentials    map[string]plateauNeighborhoodPotential
	}
	prepared := make(map[int]preparedPlateauLevel)
	prepareLevel := func(index int) preparedPlateauLevel {
		if level, exists := prepared[index]; exists {
			return level
		}
		level := policy.PlateauLevels[index]
		maxNeighborhoods := policy.RepairMaxNeighborhoods
		if level.MaxSelected > 0 || level.MaxSelectedPerBase > 0 {
			// Deep selection ranks the complete candidate set before applying caps.
			maxNeighborhoods = 0
		}
		neighborhoods := buildPlateauStarOpportunityNeighborhoods(catalog, instances, optionsByInstance, bases, level, maxNeighborhoods, config.plateauArchive)
		potentials := prioritizePlateauNeighborhoods(catalog, instanceByID, optionsByInstance, bases, neighborhoods, config, gridMask, incumbent.Evaluation.Score)
		neighborhoods = filterPriorityFeasibleNeighborhoods(neighborhoods, potentials)
		config.plateauArchive.recordPriorityFeasibleClosures(level.MaxNeighborhoodSize, len(neighborhoods))
		var perBaseDrops int
		var selectedCapDrops int
		neighborhoods, perBaseDrops, selectedCapDrops = selectPlateauLevelNeighborhoods(neighborhoods, level)
		config.plateauArchive.recordLevelSelection(level, len(neighborhoods), perBaseDrops, selectedCapDrops)
		preparedLevel := preparedPlateauLevel{neighborhoods: neighborhoods, potentials: potentials}
		prepared[index] = preparedLevel
		return preparedLevel
	}
	for index, level := range policy.PlateauLevels {
		if !plateauLevelIsDeep(level) || !plateauLevelCanRun(nodeBudget*level.QuotaBps/10_000, level) {
			continue
		}
		prepareLevel(index)
	}
	hasLaterEligibleDeepLevel := func(index int) bool {
		for next := index + 1; next < len(policy.PlateauLevels); next++ {
			level := policy.PlateauLevels[next]
			if plateauLevelIsDeep(level) && plateauLevelCanRun(nodeBudget*level.QuotaBps/10_000, level) && len(prepareLevel(next).neighborhoods) > 0 {
				return true
			}
		}
		return false
	}
	deepOnly := false
	for index, level := range policy.PlateauLevels {
		if remaining <= 0 {
			break
		}
		if deepOnly && !plateauLevelIsDeep(level) {
			// Preserve the legacy early-stop behavior for normal levels while
			// still giving the rare deep bands their independently budgeted turn.
			config.plateauArchive.recordLevelQuota(level, 0, 0, 0)
			continue
		}
		quota := plateauLevelQuota(nodeBudget, remaining, carriedQuota, index, policy.PlateauLevels)
		if !plateauLevelCanRun(quota, level) {
			config.plateauArchive.recordLevelQuota(level, quota, 0, quota)
			carriedQuota = quota
			continue
		}
		preparedLevel := prepareLevel(index)
		neighborhoods := preparedLevel.neighborhoods
		potentialByKey := preparedLevel.potentials
		if len(neighborhoods) == 0 {
			config.plateauArchive.recordLevelQuota(level, quota, 0, quota)
			carriedQuota = quota
			continue
		}
		budgets := allocateRepairNeighborhoodBudgets(neighborhoods, quota)
		levelImproved := false
		levelNodesBefore := result.NodesExplored
		for _, neighborhood := range neighborhoods {
			budget := budgets[neighborhood.Key]
			if budget <= 0 || result.NodesExplored >= nodeBudget {
				continue
			}
			config.plateauArchive.recordEnqueuedClosure(level.MaxNeighborhoodSize)
			lnsConfig := config
			lnsConfig.DisableExactBounds = true
			lnsConfig.DisableOutgoingBounds = true
			lnsConfig.tracePhase = tracePhasePlateauLNS
			lnsConfig.repairPriorityTarget = append([]int(nil), incumbent.Evaluation.Score.PriorityCounts...)
			partial := runRepairNeighborhood(catalog, instances, instanceByID, optionsByInstance, bases, neighborhood, lnsConfig, coverage, nil, gridMask, budget, nil)
			result.NodesExplored += partial.NodesExplored
			result.CandidateCount += partial.CandidateCount
			result.Iterations++
			result.SymmetryPrunedBranches += partial.SymmetryPrunedBranches
			best := incumbent.Evaluation.Score
			improved := false
			improvementCount := 0
			for _, candidate := range partial.Solutions {
				if !config.priorityBounds.reached(candidate.Evaluation.Score) {
					continue
				}
				if compareScores(candidate.Evaluation.Score, incumbent.Evaluation.Score) > 0 {
					incumbent = candidate
					result.Solutions = []model.Solution{candidate}
					result.Improvements++
					levelImproved = true
					improved = true
					improvementCount++
				}
				if compareScores(candidate.Evaluation.Score, best) > 0 {
					best = candidate.Evaluation.Score
				}
			}
			potential := potentialByKey[neighborhood.Key]
			config.plateauArchive.recordPartialUB(potential.fixedStars, potential.upperStars, incumbent.Evaluation.Score.StarCount, improved)
			config.plateauArchive.recordOperator(neighborhood.Operator, level.MaxNeighborhoodSize, partial, best)
			config.plateauArchive.recordLevelWork(level, partial.NodesExplored, improvementCount)
		}
		levelNodes := result.NodesExplored - levelNodesBefore
		remaining -= levelNodes
		carriedQuota = quota - levelNodes
		config.plateauArchive.recordLevelQuota(level, quota, levelNodes, carriedQuota)
		if levelImproved {
			if hasLaterEligibleDeepLevel(index) {
				deepOnly = true
				continue
			}
			break
		}
	}
	result.BestSummary = repairBestSummary(result.Solutions)
	return result
}

func plateauLevelQuota(nodeBudget int64, remaining int64, carried int64, index int, levels []PlateauLevelPolicy) int64 {
	if index < 0 || index >= len(levels) || remaining <= 0 {
		return 0
	}
	quota := nodeBudget*levels[index].QuotaBps/10_000 + carried
	if quota > remaining {
		return remaining
	}
	if index == len(levels)-1 && !plateauLevelIsDeep(levels[index]) {
		return remaining
	}
	return quota
}

func plateauLevelCanRun(quota int64, level PlateauLevelPolicy) bool {
	return quota > 0 && quota >= level.MinNodeBudget
}

func plateauLevelIsDeep(level PlateauLevelPolicy) bool {
	return level.MinMandatorySize > 0
}

func (level PlateauLevelPolicy) matchesMandatorySize(size int) bool {
	return size >= level.MinMandatorySize && (level.MaxMandatorySize == 0 || size <= level.MaxMandatorySize)
}

func selectPlateauLevelNeighborhoods(neighborhoods []repairNeighborhood, level PlateauLevelPolicy) ([]repairNeighborhood, int, int) {
	if level.MaxSelected <= 0 && level.MaxSelectedPerBase <= 0 {
		return neighborhoods, 0, 0
	}
	if plateauLevelIsDeep(level) {
		sort.SliceStable(neighborhoods, func(i, j int) bool {
			if neighborhoods[i].Priority != neighborhoods[j].Priority {
				return neighborhoods[i].Priority > neighborhoods[j].Priority
			}
			if neighborhoods[i].MandatorySize != neighborhoods[j].MandatorySize {
				return neighborhoods[i].MandatorySize < neighborhoods[j].MandatorySize
			}
			return neighborhoods[i].Key < neighborhoods[j].Key
		})
	}
	selected := make([]repairNeighborhood, 0, len(neighborhoods))
	selectedByBase := make(map[string]int)
	perBaseDrops := 0
	selectedCapDrops := 0
	for _, neighborhood := range neighborhoods {
		if level.MaxSelectedPerBase > 0 && selectedByBase[neighborhood.BaseLayoutKey] >= level.MaxSelectedPerBase {
			perBaseDrops++
			continue
		}
		if level.MaxSelected > 0 && len(selected) >= level.MaxSelected {
			selectedCapDrops++
			continue
		}
		selected = append(selected, neighborhood)
		selectedByBase[neighborhood.BaseLayoutKey]++
	}
	return selected, perBaseDrops, selectedCapDrops
}

func buildPlateauStarOpportunityNeighborhoods(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	bases []model.Solution,
	level PlateauLevelPolicy,
	maxNeighborhoods int,
	archive *plateauArchive,
) []repairNeighborhood {
	maxSize := level.MaxNeighborhoodSize
	var neighborhoods []repairNeighborhood
	seen := map[string]struct{}{}
	for _, base := range bases {
		placed := placementByInstanceID(base.Placements)
		links := make(map[string]int)
		for _, star := range base.Evaluation.Stars {
			links[star.SourceInstance]++
		}
		for _, source := range base.Placements {
			item := catalog.Items[source.ItemID]
			if len(item.Stars) == 0 || links[source.InstanceID] >= len(item.Stars) {
				continue
			}
			type targetRank struct {
				id    string
				value int
			}
			var targets []targetRank
			for _, target := range instances {
				if target.InstanceID == source.InstanceID || !looseSourceCanTarget(catalog, source.ItemID, target.ItemID) {
					continue
				}
				value := directedOptionCompatibility(catalog, optionsByInstance[source.InstanceID], optionsByInstance[target.InstanceID])
				if value == 0 {
					continue
				}
				if targetPlacement, exists := placed[target.InstanceID]; exists && sourceHitsTargetWithCatalog(catalog, source, targetPlacement) {
					value /= 2
				}
				targets = append(targets, targetRank{id: target.InstanceID, value: value})
			}
			sort.Slice(targets, func(i, j int) bool {
				if targets[i].value != targets[j].value {
					return targets[i].value > targets[j].value
				}
				return targets[i].id < targets[j].id
			})
			for _, target := range targets {
				mandatory := []string{source.InstanceID, target.id}
				direct := perInstanceBlockers(base.Placements, optionsByInstance, mandatory)
				indirect := plateauIndirectBlockers(base.Placements, optionsByInstance, direct, mandatory)
				mandatory = uniqueInstanceIDs(append(append(mandatory, direct...), indirect...))
				optional := nearbyInstances(base.Placements, placed, mandatory)
				archive.recordClosure(maxSize, len(mandatory), len(optional), len(mandatory) > maxSize)
				archive.recordLevelClosure(level, len(mandatory))
				if !level.matchesMandatorySize(len(mandatory)) {
					continue
				}
				if len(mandatory) > maxSize {
					// Larger configured levels will retry this exact closure.
					continue
				}
				ids := append([]string(nil), mandatory...)
				for _, optionalID := range optional {
					if len(ids) >= maxSize {
						break
					}
					ids = uniqueInstanceIDs(append(ids, optionalID))
				}
				if len(ids) < minInt(5, len(instances)) {
					continue
				}
				sortedIDs := append([]string(nil), ids...)
				sort.Strings(sortedIDs)
				key := base.LayoutKey + "|star-opportunity|" + strings.Join(sortedIDs, ",")
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				archive.recordUniqueClosure(maxSize)
				neighborhoods = append(neighborhoods, repairNeighborhood{
					Operator:      "star-opportunity",
					InstanceIDs:   ids,
					MandatorySize: len(mandatory),
					OptionalSize:  len(optional),
					Priority:      100000 + target.value + len(indirect)*10,
					Key:           key,
					BaseLayoutKey: base.LayoutKey,
				})
			}
		}
	}
	sort.Slice(neighborhoods, func(i, j int) bool {
		if neighborhoods[i].Priority != neighborhoods[j].Priority {
			return neighborhoods[i].Priority > neighborhoods[j].Priority
		}
		return neighborhoods[i].Key < neighborhoods[j].Key
	})
	if maxNeighborhoods > 0 && len(neighborhoods) > maxNeighborhoods {
		neighborhoods = neighborhoods[:maxNeighborhoods]
	}
	return neighborhoods
}

type plateauNeighborhoodPotential struct {
	fixedStars    int
	headroom      int
	overIncumbent int
	upperStars    int
}

func prioritizePlateauNeighborhoods(
	catalog model.Catalog,
	instanceByID map[string]model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	bases []model.Solution,
	neighborhoods []repairNeighborhood,
	config Config,
	gridMask uint64,
	incumbent model.Score,
) map[string]plateauNeighborhoodPotential {
	baseByKey := make(map[string]model.Solution, len(bases))
	for _, base := range bases {
		baseByKey[base.LayoutKey] = base
	}
	potentials := make(map[string]plateauNeighborhoodPotential, len(neighborhoods))
	for index := range neighborhoods {
		base, exists := baseByKey[neighborhoods[index].BaseLayoutKey]
		if !exists {
			continue
		}
		removed := make([]model.InventoryInstance, 0, len(neighborhoods[index].InstanceIDs))
		removeSet := stringSet(neighborhoods[index].InstanceIDs)
		for _, id := range neighborhoods[index].InstanceIDs {
			if instance, exists := instanceByID[id]; exists {
				removed = append(removed, instance)
			}
		}
		fixed := make([]model.Placement, 0, len(base.Placements))
		var fixedOccupied uint64
		for _, placement := range base.Placements {
			if _, removed := removeSet[placement.InstanceID]; removed {
				continue
			}
			fixed = append(fixed, placement)
			fixedOccupied |= placement.Mask
		}
		state := partialRepairState{FixedPlacements: fixed, RemovedInstances: removed, FreeCells: gridMask &^ fixedOccupied}
		upper := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, config.Priorities)
		if !partialRepairTargetVectorFeasible(upper, incumbent.PriorityCounts) {
			continue
		}
		upperStars := partialRelaxedStarUpperBound(catalog, state, optionsByInstance)
		fixedStars := len(partialRepairFixedStars(catalog, state))
		potential := plateauNeighborhoodPotential{
			fixedStars:    fixedStars,
			headroom:      upperStars - fixedStars,
			overIncumbent: upperStars - incumbent.StarCount,
			upperStars:    upperStars,
		}
		potentials[neighborhoods[index].Key] = potential
		neighborhoods[index].Priority += upperStars
	}
	return potentials
}

func filterPriorityFeasibleNeighborhoods(neighborhoods []repairNeighborhood, potentials map[string]plateauNeighborhoodPotential) []repairNeighborhood {
	filtered := make([]repairNeighborhood, 0, len(neighborhoods))
	for _, neighborhood := range neighborhoods {
		if _, feasible := potentials[neighborhood.Key]; feasible {
			filtered = append(filtered, neighborhood)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Priority != filtered[j].Priority {
			return filtered[i].Priority > filtered[j].Priority
		}
		return filtered[i].Key < filtered[j].Key
	})
	return filtered
}

func plateauIndirectBlockers(
	placements []model.Placement,
	optionsByInstance map[string][]model.Placement,
	direct []string,
	mandatory []string,
) []string {
	mandatorySet := stringSet(mandatory)
	directSet := stringSet(direct)
	var indirect []string
	for _, blocker := range direct {
		indirect = append(indirect, blockingInstancesForOptions(placements, optionsByInstance[blocker])...)
	}
	indirect = uniqueInstanceIDs(indirect)
	filtered := indirect[:0]
	for _, blocker := range indirect {
		if _, mandatory := mandatorySet[blocker]; mandatory {
			continue
		}
		if _, direct := directSet[blocker]; direct {
			continue
		}
		filtered = append(filtered, blocker)
	}
	return filtered
}

type plateauRefineStats struct {
	NodesExplored int64
	MaxDepth      int
	MaxValley     int
	Improved      bool
}

type plateauWalkState struct {
	solution  model.Solution
	depth     int
	upperStar int
	signature string
}

// refinePlateau is a bounded best-first walk. Unlike the normal hill climber,
// it retains priority-ceiling states across a tie-break valley, but only emits
// a replacement when the complete compareScores vector strictly improves.
func refinePlateau(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	incumbent model.Solution,
	config Config,
	starBounds *starUpperBoundContext,
	nodeBudget int64,
) (model.Solution, plateauRefineStats, error) {
	if nodeBudget <= 0 || config.priorityBounds == nil || !config.priorityBounds.reached(incumbent.Evaluation.Score) {
		return incumbent, plateauRefineStats{}, nil
	}
	best := incumbent
	stats := plateauRefineStats{}
	visited := map[string]struct{}{incumbent.CanonicalLayoutHash: {}}
	policy := policyForConfig(config)
	queue := make([]plateauWalkState, 0, policy.PlateauArchiveCapacity)
	for _, base := range config.plateauArchive.bases() {
		if !config.priorityBounds.reached(base.Evaluation.Score) {
			continue
		}
		hash := base.CanonicalLayoutHash
		if _, seen := visited[hash]; seen {
			continue
		}
		visited[hash] = struct{}{}
		queue = append(queue, plateauState(base, 0, starBounds, instances))
	}
	if len(queue) == 0 {
		queue = append(queue, plateauState(incumbent, 0, starBounds, instances))
	}
	for len(queue) > 0 && stats.NodesExplored < nodeBudget {
		sortPlateauWalk(queue)
		current := queue[0]
		queue = queue[1:]
		if current.depth >= policy.PlateauWalkDepth {
			continue
		}
		for placementIndex, placement := range current.solution.Placements {
			fixedMask := occupiedExcept(current.solution.Placements, placementIndex)
			for _, option := range optionsByInstance[placement.InstanceID] {
				if stats.NodesExplored >= nodeBudget {
					break
				}
				if option.Mask == placement.Mask && option.Origin == placement.Origin && option.Rotation == placement.Rotation {
					continue
				}
				if option.Mask&fixedMask != 0 {
					continue
				}
				candidatePlacements := append([]model.Placement(nil), current.solution.Placements...)
				candidatePlacements[placementIndex] = option
				if !placementRespectsCanonicalCopyOrder(option, candidatePlacements) {
					continue
				}
				if !chargeNode(config, tracePhasePlateauRefine) {
					break
				}
				stats.NodesExplored++
				score := evaluateScoreForConfig(catalog, candidatePlacements, config)
				if !config.priorityBounds.reached(score) {
					continue
				}
				evaluation := evaluateLayoutForConfig(catalog, candidatePlacements, config)
				candidate := model.Solution{
					Placements:          candidatePlacements,
					Evaluation:          evaluation,
					LayoutKey:           layoutKey(candidatePlacements, instances),
					CanonicalLayoutHash: canonicalLayoutHash(candidatePlacements),
				}
				config.plateauArchive.observe(candidate, tracePhasePlateauRefine)
				config.trace.observeCandidate(tracePhasePlateauRefine, candidatePlacements, instances, score, true, &evaluation)
				if compareScores(candidate.Evaluation.Score, best.Evaluation.Score) > 0 {
					best = candidate
					stats.Improved = true
				}
				if candidate.Evaluation.Score.StarCount < incumbent.Evaluation.Score.StarCount {
					valley := incumbent.Evaluation.Score.StarCount - candidate.Evaluation.Score.StarCount
					if valley > stats.MaxValley {
						stats.MaxValley = valley
					}
				}
				if current.depth+1 > stats.MaxDepth {
					stats.MaxDepth = current.depth + 1
				}
				if _, seen := visited[candidate.CanonicalLayoutHash]; seen {
					continue
				}
				visited[candidate.CanonicalLayoutHash] = struct{}{}
				upper := plateauState(candidate, current.depth+1, starBounds, instances)
				config.plateauArchive.recordUBStars(candidate.Evaluation.Score.StarCount, upper.upperStar, upper.upperStar > candidate.Evaluation.Score.StarCount, compareScores(candidate.Evaluation.Score, incumbent.Evaluation.Score) > 0)
				queue = append(queue, upper)
				if len(queue) > policy.PlateauArchiveCapacity {
					sortPlateauWalk(queue)
					queue = queue[:policy.PlateauArchiveCapacity]
				}
			}
		}
	}
	return best, stats, nil
}

func plateauState(solution model.Solution, depth int, starBounds *starUpperBoundContext, instances []model.InventoryInstance) plateauWalkState {
	upper := solution.Evaluation.Score.StarCount
	if starBounds != nil {
		upper = starBounds.forPlacements(solution.Placements, instances).GeometricRelaxed
	}
	return plateauWalkState{
		solution:  solution,
		depth:     depth,
		upperStar: upper,
		signature: canonicalLinkSignature(solution.Placements, solution.Evaluation.Stars),
	}
}

func sortPlateauWalk(queue []plateauWalkState) {
	sort.Slice(queue, func(i, j int) bool {
		if queue[i].upperStar != queue[j].upperStar {
			return queue[i].upperStar > queue[j].upperStar
		}
		if compare := compareScores(queue[i].solution.Evaluation.Score, queue[j].solution.Evaluation.Score); compare != 0 {
			return compare > 0
		}
		if queue[i].signature != queue[j].signature {
			return queue[i].signature < queue[j].signature
		}
		return queue[i].solution.CanonicalLayoutHash < queue[j].solution.CanonicalLayoutHash
	})
}

package solver

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/model"
)

const coverageSeedBeamWidth = 256
const coverageSeedMaxNodes = int64(250000)
const coverageSeedStateBuffer = coverageSeedBeamWidth * 4

type coverageSeedState struct {
	occupied        uint64
	placed          []model.Placement
	coverage        coverageSearchState
	coverageCounts  []int
	potentialCounts []int
	potential       int
	key             string
}

func seedNodeBudget(maxNodes int64) int64 {
	if maxNodes == 0 {
		return coverageSeedMaxNodes
	}
	budget := maxNodes / 4
	if budget <= 0 {
		return 0
	}
	if budget > coverageSeedMaxNodes {
		return coverageSeedMaxNodes
	}
	return budget
}

func coverageSeedSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	coverage *coverageContext,
	gridMask uint64,
	nodeBudget int64,
	progress *progressTracker,
) coverageSeedResult {
	if coverage == nil || !coverage.enabled || !coverage.pruningEnabled || coverage.targetCount() == 0 || nodeBudget <= 0 {
		return coverageSeedResult{}
	}

	ordered := coverageSeedOrder(catalog, instances, optionsByInstance, coverage)
	remainingCells := remainingCellCounts(catalog, ordered)
	initialCoverage := coverageSearchState{}
	states := []coverageSeedState{{
		coverage:        initialCoverage,
		coverageCounts:  coverageSeedCounts(coverage, initialCoverage),
		potentialCounts: coverageSeedPotentialCounts(coverage, initialCoverage),
	}}
	var nodes int64
	var progressBatch int64
	exhausted := false
	canceled := false
	reportNode := func() {
		nodes++
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil {
			canceled = true
			exhausted = true
		}
		if progress == nil {
			return
		}
		progressBatch++
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
			progressBatch = 0
		}
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
			progressBatch = 0
		}
	}

	for index, instance := range ordered {
		if canceled || exhausted || len(states) == 0 {
			break
		}
		nextStates := make([]coverageSeedState, 0, coverageSeedBeamWidth*2)
		for _, state := range states {
			if canceled || (nodeBudget > 0 && nodes >= nodeBudget) {
				exhausted = true
				break
			}
			if !config.AllowSkips && remainingCells[index] > bits.OnesCount64(gridMask&^state.occupied) {
				continue
			}
			options := optionsByInstance[instance.InstanceID]
			for _, option := range options {
				if option.Mask&state.occupied != 0 {
					continue
				}
				reportNode()
				if canceled {
					break
				}
				nextCoverage := coverage.withPlacement(catalog, state.coverage, option, state.placed)
				nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				nextStates = appendCoverageSeedState(nextStates, coverageSeedState{
					occupied:        state.occupied | option.Mask,
					placed:          nextPlaced,
					coverage:        nextCoverage,
					coverageCounts:  coverageSeedCounts(coverage, nextCoverage),
					potentialCounts: coverageSeedPotentialCounts(coverage, nextCoverage),
					potential:       state.potential + coverage.priorityForPlacement(option),
					key:             coverageSeedAppendKey(state.key, option),
				})
				if nodeBudget > 0 && nodes >= nodeBudget {
					exhausted = true
					break
				}
			}
			if exhausted {
				break
			}
			if config.AllowSkips {
				reportNode()
				if canceled {
					break
				}
				nextCoverage := coverage.withSkip(state.coverage, instance)
				nextPlaced := append([]model.Placement(nil), state.placed...)
				nextStates = appendCoverageSeedState(nextStates, coverageSeedState{
					occupied:        state.occupied,
					placed:          nextPlaced,
					coverage:        nextCoverage,
					coverageCounts:  coverageSeedCounts(coverage, nextCoverage),
					potentialCounts: coverageSeedPotentialCounts(coverage, nextCoverage),
					potential:       state.potential,
					key:             coverageSeedSkipKey(state.key, instance),
				})
			}
		}
		sortCoverageSeedStates(nextStates)
		if len(nextStates) > coverageSeedBeamWidth {
			clear(nextStates[coverageSeedBeamWidth:])
			nextStates = nextStates[:coverageSeedBeamWidth]
		}
		states = nextStates
	}

	results := make([]model.Solution, 0, config.TopN)
	var candidateCount int
	for _, state := range states {
		if !config.AllowSkips && len(state.placed) != len(instances) {
			continue
		}
		candidateCount++
		results = insertCandidateWithScoreOnlyFilter(catalog, results, state.placed, instances, config)
		if config.StopOnCoverageCeiling && len(results) > 0 && coverage.ceilingReached(results[0].Evaluation.Score) {
			flushProgress()
			return coverageSeedResult{
				Solutions:                   results,
				NodesExplored:               nodes,
				CandidateCount:              candidateCount,
				BestSummary:                 seedBestSummary(results),
				StoppedAfterCoverageCeiling: true,
			}
		}
	}

	flushProgress()
	return coverageSeedResult{
		Solutions:      results,
		NodesExplored:  nodes,
		CandidateCount: candidateCount,
		BestSummary:    seedBestSummary(results),
	}
}

func coverageSeedOrder(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	coverage *coverageContext,
) []model.InventoryInstance {
	ordered := append([]model.InventoryInstance(nil), instances...)
	sort.Slice(ordered, func(i, j int) bool {
		leftGroup := coverageSeedInstanceGroup(coverage, ordered[i])
		rightGroup := coverageSeedInstanceGroup(coverage, ordered[j])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		leftTargetIndex := coverage.targetIndexByOriginal[ordered[i].OriginalIndex]
		rightTargetIndex := coverage.targetIndexByOriginal[ordered[j].OriginalIndex]
		if leftTargetIndex >= 0 && rightTargetIndex >= 0 {
			leftSources := bits.OnesCount64(coverage.targetPossibleSourceMask[leftTargetIndex])
			rightSources := bits.OnesCount64(coverage.targetPossibleSourceMask[rightTargetIndex])
			if leftSources != rightSources {
				return leftSources > rightSources
			}
		}
		leftItem := catalog.Items[ordered[i].ItemID]
		rightItem := catalog.Items[ordered[j].ItemID]
		if len(leftItem.Shape) != len(rightItem.Shape) {
			return len(leftItem.Shape) > len(rightItem.Shape)
		}
		leftOptions := len(optionsByInstance[ordered[i].InstanceID])
		rightOptions := len(optionsByInstance[ordered[j].InstanceID])
		if leftOptions != rightOptions {
			return leftOptions < rightOptions
		}
		if ordered[i].ItemID != ordered[j].ItemID {
			return ordered[i].ItemID < ordered[j].ItemID
		}
		return ordered[i].OriginalIndex < ordered[j].OriginalIndex
	})
	return ordered
}

func coverageSeedInstanceGroup(coverage *coverageContext, instance model.InventoryInstance) int {
	if coverage.targetIndexByOriginal[instance.OriginalIndex] >= 0 {
		return 0
	}
	if coverage.sourceMaskByOriginal[instance.OriginalIndex] != 0 {
		return 1
	}
	return 2
}

func sortCoverageSeedStates(states []coverageSeedState) {
	sort.Slice(states, func(i, j int) bool {
		if compare := comparePriorityCounts(states[i].coverageCounts, states[j].coverageCounts); compare != 0 {
			return compare > 0
		}
		if states[i].potential != states[j].potential {
			return states[i].potential > states[j].potential
		}
		if compare := comparePriorityCounts(states[i].potentialCounts, states[j].potentialCounts); compare != 0 {
			return compare > 0
		}
		if len(states[i].placed) != len(states[j].placed) {
			return len(states[i].placed) > len(states[j].placed)
		}
		return states[i].key < states[j].key
	})
}

func appendCoverageSeedState(states []coverageSeedState, state coverageSeedState) []coverageSeedState {
	states = append(states, state)
	if len(states) <= coverageSeedStateBuffer {
		return states
	}
	sortCoverageSeedStates(states)
	clear(states[coverageSeedBeamWidth:])
	return states[:coverageSeedBeamWidth]
}

func coverageSeedCounts(coverage *coverageContext, state coverageSearchState) []int {
	targetCount := coverage.targetCount()
	targetMasks := make([]uint64, 0, targetCount)
	for targetIndex := 0; targetIndex < targetCount; targetIndex++ {
		if state.targetPlacedMask&(uint64(1)<<uint(targetIndex)) == 0 {
			continue
		}
		targetMasks = append(targetMasks, state.targetCoverage[targetIndex])
	}
	return coverageCountsFromTargetMasks(targetMasks, len(coverage.sourceItemIDs))
}

func coverageSeedPotentialCounts(coverage *coverageContext, state coverageSearchState) []int {
	targetCount := coverage.targetCount()
	targetMasks := make([]uint64, 0, targetCount)
	for targetIndex := 0; targetIndex < targetCount; targetIndex++ {
		targetBit := uint64(1) << uint(targetIndex)
		if state.targetDecidedMask&targetBit != 0 && state.targetPlacedMask&targetBit == 0 {
			continue
		}
		if state.targetPlacedMask&targetBit != 0 {
			targetMasks = append(targetMasks, state.targetCoverage[targetIndex]|coverage.targetPossibleSourceMask[targetIndex])
		} else {
			targetMasks = append(targetMasks, coverage.targetPossibleSourceMask[targetIndex])
		}
	}
	return coverageCountsFromTargetMasks(targetMasks, len(coverage.sourceItemIDs))
}

func seedBestSummary(results []model.Solution) string {
	if len(results) == 0 || results[0].Evaluation.StarCoverage == nil {
		return ""
	}
	coverage := results[0].Evaluation.StarCoverage
	parts := make([]string, 0, len(coverage.Buckets))
	totalSources := len(coverage.Sources)
	for _, bucket := range coverage.Buckets {
		parts = append(parts, fmt.Sprintf("%d/%d=%d", bucket.CoveredSources, totalSources, bucket.TargetCount))
	}
	return strings.Join(parts, ", ")
}

func coverageSeedAppendKey(current string, placement model.Placement) string {
	var builder strings.Builder
	builder.Grow(len(current) + len(placement.ItemID) + len(placement.InstanceID) + 48)
	builder.WriteString(current)
	builder.WriteString(placement.ItemID)
	builder.WriteByte('|')
	builder.WriteString(placement.InstanceID)
	builder.WriteByte('|')
	builder.WriteString(placementKey(placement))
	builder.WriteByte(';')
	return builder.String()
}

func coverageSeedSkipKey(current string, instance model.InventoryInstance) string {
	var builder strings.Builder
	builder.Grow(len(current) + len(instance.ItemID) + len(instance.InstanceID) + 8)
	builder.WriteString(current)
	builder.WriteString("skip|")
	builder.WriteString(instance.ItemID)
	builder.WriteByte('|')
	builder.WriteString(instance.InstanceID)
	builder.WriteByte(';')
	return builder.String()
}

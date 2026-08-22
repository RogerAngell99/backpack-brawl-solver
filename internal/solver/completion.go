package solver

import (
	"sort"

	"backpack-brawl-solver/internal/model"
)

type completionStats struct {
	MovesChecked           int64
	Improvements           int
	SymmetryPrunedBranches int64
}

// completeSkippedSolutions tries to pack skipped items into unused space after a
// limited solve. It never replaces an incumbent unless full scoring improves it,
// so optional packing cannot reduce the configured objective.
func completeSkippedSolutions(
	catalog model.Catalog,
	original []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solutions []model.Solution,
	searchStats model.SearchStats,
	config Config,
) ([]model.Solution, model.SearchStats, error) {
	if !config.AllowSkips || len(solutions) == 0 {
		return solutions, searchStats, nil
	}
	remainingMoves := completionMoveLimit(config.MaxRefineMoves)
	completed := make([]model.Solution, 0, len(solutions))
	for solutionIndex, solution := range solutions {
		if remainingMoves <= 0 {
			completed = append(completed, solutions[solutionIndex:]...)
			break
		}
		next, stats, err := completeSkippedSolution(catalog, original, optionsByInstance, solution, config, remainingMoves)
		if err != nil {
			return nil, searchStats, err
		}
		searchStats.CompletionMovesChecked += stats.MovesChecked
		searchStats.CompletionImprovements += stats.Improvements
		searchStats.SymmetryPrunedBranches += stats.SymmetryPrunedBranches
		next.Search = searchStats
		completed = append(completed, next)
		remainingMoves -= stats.MovesChecked
	}
	return completed, searchStats, nil
}

func completionMoveLimit(refineMoveLimit int64) int64 {
	if refineMoveLimit <= 0 {
		return 0
	}
	limit := refineMoveLimit / 2
	if limit < 1000 {
		return 1000
	}
	if limit > 25000 {
		return 25000
	}
	return limit
}

func completeSkippedSolution(
	catalog model.Catalog,
	original []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solution model.Solution,
	config Config,
	maxMoves int64,
) (model.Solution, completionStats, error) {
	placed := placementByInstanceID(solution.Placements)
	skipped := skippedInstances(original, placed)
	if len(skipped) == 0 || maxMoves <= 0 {
		return solution, completionStats{}, nil
	}
	sort.Slice(skipped, func(i, j int) bool {
		leftOptions := len(optionsByInstance[skipped[i].InstanceID])
		rightOptions := len(optionsByInstance[skipped[j].InstanceID])
		if leftOptions != rightOptions {
			return leftOptions < rightOptions
		}
		leftArea := len(catalog.Items[skipped[i].ItemID].Shape)
		rightArea := len(catalog.Items[skipped[j].ItemID].Shape)
		if leftArea != rightArea {
			return leftArea > rightArea
		}
		return skipped[i].OriginalIndex < skipped[j].OriginalIndex
	})

	best := solution
	var stats completionStats
	var visit func(index int, occupied uint64, placements []model.Placement) error
	visit = func(index int, occupied uint64, placements []model.Placement) error {
		if stats.MovesChecked >= maxMoves {
			return nil
		}
		if config.Context != nil {
			if err := config.Context.Err(); err != nil {
				return err
			}
		}
		if index == len(skipped) {
			return nil
		}
		instance := skipped[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if stats.MovesChecked >= maxMoves {
				return nil
			}
			if option.Mask&occupied != 0 {
				continue
			}
			if !placementRespectsCanonicalCopyOrder(option, placements) {
				stats.SymmetryPrunedBranches++
				continue
			}
			stats.MovesChecked++
			config.trace.addUncharged(config.tracePhase, 1)
			nextPlacements, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
			score := evaluateScoreForConfig(catalog, nextPlacements, config)
			observeSearchCandidate(catalog, nextPlacements, original, score, true, config)
			if scoreOnlyImprovesSolution(nextPlacements, original, score, best) {
				evaluation := evaluateLayoutForConfig(catalog, nextPlacements, config)
				if candidate, improved := improvedCandidate(nextPlacements, original, evaluation, best); improved {
					candidate.Search = solution.Search
					best = candidate
					stats.Improvements++
				}
			}
			if err := visit(index+1, occupied|option.Mask, nextPlacements); err != nil {
				return err
			}
		}
		return visit(index+1, occupied, placements)
	}
	occupied := uint64(0)
	for _, placement := range solution.Placements {
		occupied |= placement.Mask
	}
	if err := visit(0, occupied, append([]model.Placement(nil), solution.Placements...)); err != nil {
		return solution, stats, err
	}
	return best, stats, nil
}

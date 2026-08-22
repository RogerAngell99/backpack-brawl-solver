package solver

import (
	"math/bits"
	"sort"

	"backpack-brawl-solver/internal/model"
)

type starSeedState struct {
	occupied  uint64
	placed    []model.Placement
	score     model.Score
	potential int
	key       string
}

const perInstanceSeedMaxNodes = int64(2_000_000)

func perInstanceSeedNodeBudget(maxNodes int64) int64 {
	if maxNodes <= 0 {
		return coverageSeedMaxNodes
	}
	baseline := seedNodeBudget(maxNodes)
	if maxNodes <= 2_500_000 {
		return baseline
	}
	budget := maxNodes / 10
	if budget < baseline {
		budget = baseline
	}
	maximumShare := maxNodes / 4
	if budget > maximumShare {
		budget = maximumShare
	}
	if budget > perInstanceSeedMaxNodes {
		budget = perInstanceSeedMaxNodes
	}
	return budget
}

func starSeedBeamWidth(nodeBudget int64) int {
	if nodeBudget < 500_000 {
		return coverageSeedBeamWidth
	}
	width := int(nodeBudget / 2_000)
	if width < coverageSeedBeamWidth {
		return coverageSeedBeamWidth
	}
	if width > 1_024 {
		return 1_024
	}
	return width
}

// starSeedSearch builds complete generic-star candidates when no explicit
// incoming coverage group is active. It ranks only real partial scores and a
// read-only ordering heuristic; potential never substitutes for evaluation.
func starSeedSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	potential *starPotentialContext,
	progress *progressTracker,
) coverageSeedResult {
	if nodeBudget <= 0 || len(ordered) == 0 {
		return coverageSeedResult{}
	}
	beamWidth := policyForConfig(config).StarSeedBeamWidth
	remainingCells := remainingCellCounts(catalog, ordered)
	states := []starSeedState{{}}
	var nodes int64
	var symmetryPruned int64
	var progressBatch int64
	exhausted := false
	reportNode := func() bool {
		if !chargeNode(config, config.tracePhase) {
			exhausted = true
			return false
		}
		nodes++
		if progress == nil {
			return true
		}
		progressBatch++
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
			progressBatch = 0
		}
		return true
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
		}
	}

	for index, instance := range ordered {
		if exhausted || len(states) == 0 || (config.Context != nil && config.Context.Err() != nil) {
			break
		}
		next := make([]starSeedState, 0, beamWidth*2)
		for _, state := range states {
			if exhausted || (nodeBudget > 0 && nodes >= nodeBudget) {
				exhausted = true
				break
			}
			if !config.AllowSkips && remainingCells[index] > bits.OnesCount64(gridMask&^state.occupied) {
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
				if !reportNode() {
					break
				}
				nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				next = appendStarSeedState(next, starSeedState{
					occupied:  state.occupied | option.Mask,
					placed:    nextPlaced,
					score:     evaluateScoreForConfig(catalog, nextPlaced, config),
					potential: state.potential + potential.priorityForPlacement(option),
					key:       coverageSeedAppendKey(state.key, option),
				}, beamWidth)
				if nodeBudget > 0 && nodes >= nodeBudget {
					exhausted = true
					break
				}
			}
			if exhausted {
				break
			}
			if config.AllowSkips {
				if !reportNode() {
					break
				}
				next = appendStarSeedState(next, starSeedState{
					occupied:  state.occupied,
					placed:    append([]model.Placement(nil), state.placed...),
					score:     state.score,
					potential: state.potential,
					key:       coverageSeedSkipKey(state.key, instance),
				}, beamWidth)
			}
		}
		sortStarSeedStates(next)
		if len(next) > beamWidth {
			clear(next[beamWidth:])
			next = next[:beamWidth]
		}
		states = next
	}

	results := make([]model.Solution, 0, config.TopN)
	candidateCount := 0
	for _, state := range states {
		if !config.AllowSkips && len(state.placed) != len(instances) {
			continue
		}
		candidateCount++
		results = insertCandidateWithScoreOnlyFilter(catalog, results, state.placed, instances, config)
	}
	flushProgress()
	return coverageSeedResult{
		Solutions:              results,
		NodesExplored:          nodes,
		CandidateCount:         candidateCount,
		SymmetryPrunedBranches: symmetryPruned,
	}
}

func sortStarSeedStates(states []starSeedState) {
	sort.Slice(states, func(i, j int) bool {
		if compare := compareScores(states[i].score, states[j].score); compare != 0 {
			return compare > 0
		}
		if states[i].potential != states[j].potential {
			return states[i].potential > states[j].potential
		}
		if len(states[i].placed) != len(states[j].placed) {
			return len(states[i].placed) > len(states[j].placed)
		}
		return states[i].key < states[j].key
	})
}

func appendStarSeedState(states []starSeedState, state starSeedState, beamWidth int) []starSeedState {
	states = append(states, state)
	if len(states) <= beamWidth*4 {
		return states
	}
	sortStarSeedStates(states)
	clear(states[beamWidth:])
	return states[:beamWidth]
}

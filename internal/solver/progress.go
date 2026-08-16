package solver

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"backpack-brawl-solver/internal/model"
)

const (
	ProgressPhaseSeed   = "seed"
	ProgressPhaseRepair = "repair"
	ProgressPhaseSearch = "search"
	ProgressPhaseRefine = "refine"
	ProgressPhaseDone   = "done"

	progressNodeInterval  = int64(8192)
	progressEmitInterval  = 150 * time.Millisecond
	incumbentEmitInterval = time.Second
)

type ProgressSnapshot struct {
	Phase            string
	NodesExplored    int64
	NodesTotal       int64
	Percent          float64
	ElapsedMS        int64
	NodesPerSecond   float64
	EtaMS            int64
	PartialSolutions []model.Solution
}

type ProgressReporter func(ProgressSnapshot)

type progressTracker struct {
	reporter ProgressReporter
	total    int64
	started  time.Time
	nodes    atomic.Int64

	mu                sync.Mutex
	lastEmit          time.Time
	lastIncumbentEmit time.Time
	bestSolutions     []model.Solution
}

func newProgressTracker(reporter ProgressReporter, total int64) *progressTracker {
	if reporter == nil {
		return nil
	}
	now := time.Now()
	return &progressTracker{
		reporter:          reporter,
		total:             total,
		started:           now,
		lastEmit:          now.Add(-progressEmitInterval),
		lastIncumbentEmit: now.Add(-incumbentEmitInterval),
	}
}

func (tracker *progressTracker) addNodes(phase string, delta int64, force bool) {
	if tracker == nil {
		return
	}
	if delta > 0 {
		tracker.nodes.Add(delta)
	}
	tracker.emit(phase, force)
}

func (tracker *progressTracker) emitPhase(phase string) {
	if tracker == nil {
		return
	}
	tracker.emit(phase, true)
}

func (tracker *progressTracker) reportIncumbent(phase string, solutions []model.Solution, force bool) {
	if tracker == nil || len(solutions) == 0 {
		return
	}
	now := time.Now()
	tracker.mu.Lock()
	if !tracker.shouldReplaceBestLocked(solutions) {
		tracker.mu.Unlock()
		return
	}
	// Solutions are immutable after insertion; defer costly deep copies until a
	// snapshot is actually emitted to a progress consumer.
	tracker.bestSolutions = append([]model.Solution(nil), solutions...)
	if !force && now.Sub(tracker.lastIncumbentEmit) < incumbentEmitInterval {
		tracker.mu.Unlock()
		return
	}
	tracker.lastIncumbentEmit = now
	tracker.lastEmit = now
	snapshot := tracker.snapshot(now, phase)
	snapshot.PartialSolutions = tracker.partialSolutionsForSnapshot(snapshot)
	tracker.mu.Unlock()

	tracker.reporter(snapshot)
}

func (tracker *progressTracker) finish() {
	if tracker == nil {
		return
	}
	tracker.emit(ProgressPhaseDone, true)
}

func (tracker *progressTracker) emit(phase string, force bool) {
	now := time.Now()
	tracker.mu.Lock()
	if !force && now.Sub(tracker.lastEmit) < progressEmitInterval {
		tracker.mu.Unlock()
		return
	}
	tracker.lastEmit = now
	snapshot := tracker.snapshot(now, phase)
	tracker.mu.Unlock()

	tracker.reporter(snapshot)
}

func (tracker *progressTracker) snapshot(now time.Time, phase string) ProgressSnapshot {
	nodes := tracker.nodes.Load()
	elapsed := now.Sub(tracker.started)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedSeconds := elapsed.Seconds()
	var nodesPerSecond float64
	if elapsedSeconds > 0 && nodes > 0 {
		nodesPerSecond = float64(nodes) / elapsedSeconds
	}

	var percent float64
	var etaMS int64
	if tracker.total > 0 {
		if phase == ProgressPhaseDone {
			percent = 100
		} else {
			percent = math.Min(99.9, math.Max(0, (float64(nodes)/float64(tracker.total))*100))
		}
		if phase == ProgressPhaseSeed || phase == ProgressPhaseRepair || phase == ProgressPhaseSearch {
			remaining := tracker.total - nodes
			if remaining > 0 && nodesPerSecond > 0 {
				etaMS = int64((float64(remaining) / nodesPerSecond) * 1000)
			}
		}
	}

	return ProgressSnapshot{
		Phase:          phase,
		NodesExplored:  nodes,
		NodesTotal:     tracker.total,
		Percent:        percent,
		ElapsedMS:      elapsed.Milliseconds(),
		NodesPerSecond: nodesPerSecond,
		EtaMS:          etaMS,
	}
}

func (tracker *progressTracker) shouldReplaceBestLocked(solutions []model.Solution) bool {
	if len(solutions) == 0 {
		return false
	}
	if len(tracker.bestSolutions) == 0 {
		return true
	}
	return SolutionLess(solutions[0], tracker.bestSolutions[0])
}

func (tracker *progressTracker) partialSolutionsForSnapshot(snapshot ProgressSnapshot) []model.Solution {
	out := cloneSolutions(tracker.bestSolutions)
	for idx := range out {
		out[idx].Search.NodesExplored = snapshot.NodesExplored
		out[idx].Search.NodesPerSecond = snapshot.NodesPerSecond
		out[idx].Search.Limited = tracker.total > 0
	}
	return out
}

func cloneSolutions(solutions []model.Solution) []model.Solution {
	if len(solutions) == 0 {
		return nil
	}
	out := make([]model.Solution, len(solutions))
	copy(out, solutions)
	for idx := range out {
		out[idx].Placements = append([]model.Placement(nil), out[idx].Placements...)
		out[idx].Evaluation.Crafts = append([]model.CraftActivation(nil), out[idx].Evaluation.Crafts...)
		out[idx].Evaluation.Stars = append([]model.StarActivation(nil), out[idx].Evaluation.Stars...)
		out[idx].Evaluation.LooseStarPriorities = append([]model.LooseStarPriority(nil), out[idx].Evaluation.LooseStarPriorities...)
		out[idx].Evaluation.StarCoverageGroups = append([]model.StarCoverageBreakdown(nil), out[idx].Evaluation.StarCoverageGroups...)
		if out[idx].Evaluation.StarCoverage != nil {
			coverage := *out[idx].Evaluation.StarCoverage
			out[idx].Evaluation.StarCoverage = &coverage
		}
	}
	return out
}

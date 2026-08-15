package websolve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/scenario"
	"backpack-brawl-solver/internal/solver"
)

type Request struct {
	Catalog  json.RawMessage   `json:"catalog"`
	Scenario scenario.Scenario `json:"scenario"`
}

type Options struct {
	ProgressReporter solver.ProgressReporter
	WorkerOverride   int
	DefaultMaxNodes  int64
	MaxNodesCap      int64
	Backend          string
	Context          context.Context
}

type Metadata struct {
	Backend         string
	ServerElapsedMS int64
	Workers         int
	MaxNodesApplied int64
	MaxNodesCapped  bool
}

type Result struct {
	JSON     []byte
	Metadata Metadata
}

func SolveScenarioJSON(input []byte) ([]byte, error) {
	return SolveScenarioJSONWithProgress(input, nil)
}

func SolveScenarioJSONWithProgress(input []byte, progressReporter solver.ProgressReporter) ([]byte, error) {
	result, err := SolveScenarioJSONWithOptions(input, Options{ProgressReporter: progressReporter})
	if err != nil {
		return nil, err
	}
	return result.JSON, nil
}

func SolveScenarioJSONWithOptions(input []byte, options Options) (Result, error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	var request Request
	if err := json.Unmarshal(input, &request); err != nil {
		return Result{}, err
	}
	if len(request.Catalog) == 0 {
		return Result{}, fmt.Errorf("catalog is required")
	}
	if err := request.Scenario.Validate(); err != nil {
		return Result{}, err
	}

	loadedCatalog, err := catalog.Parse(request.Catalog)
	if err != nil {
		return Result{}, err
	}

	gridMask := geometry.FullGridMask()
	if len(request.Scenario.Grid) > 0 {
		gridMask, err = geometry.ParseGridText(request.Scenario.GridText())
		if err != nil {
			return Result{}, err
		}
	}

	top := 3
	if request.Scenario.Top != nil {
		top = *request.Scenario.Top
	}
	maxNodes := int64(200000)
	if request.Scenario.MaxNodes != nil {
		maxNodes = *request.Scenario.MaxNodes
	}
	maxNodesCapped := false
	if maxNodes == 0 && options.DefaultMaxNodes > 0 {
		maxNodes = options.DefaultMaxNodes
		maxNodesCapped = true
	}
	if options.MaxNodesCap > 0 && (maxNodes == 0 || maxNodes > options.MaxNodesCap) {
		maxNodes = options.MaxNodesCap
		maxNodesCapped = true
	}
	workers := 1
	if request.Scenario.Workers != nil {
		workers = *request.Scenario.Workers
	}
	if options.WorkerOverride > 0 {
		workers = options.WorkerOverride
	}
	noSkips := false
	if request.Scenario.NoSkips != nil {
		noSkips = *request.Scenario.NoSkips
	}
	stopOnCoverageCeiling := false
	if request.Scenario.StopOnCoverageCeiling != nil {
		stopOnCoverageCeiling = *request.Scenario.StopOnCoverageCeiling
	}
	repairSearch := maxNodes > 0
	if request.Scenario.RepairSearch != nil {
		repairSearch = *request.Scenario.RepairSearch
	}
	if maxNodes == 0 {
		repairSearch = false
	}

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(loadedCatalog, request.Scenario.ItemIDs(), gridMask, solver.Config{
		TopN:                  top,
		AllowSkips:            !noSkips,
		MaxNodes:              maxNodes,
		Workers:               workers,
		Priorities:            request.Scenario.Priorities,
		CoverageGroups:        request.Scenario.ModelCoverageGroups(),
		StopOnCoverageCeiling: stopOnCoverageCeiling,
		RepairSearch:          repairSearch,
		ProgressReporter:      options.ProgressReporter,
		Context:               options.Context,
	})
	if err != nil {
		return Result{}, err
	}
	elapsed := time.Since(startedAt)
	elapsedMS := elapsed.Milliseconds()
	elapsedSeconds := elapsed.Seconds()
	for index := range solutions {
		if elapsedSeconds > 0 {
			solutions[index].Search.NodesPerSecond = float64(solutions[index].Search.NodesExplored) / elapsedSeconds
		}
		if options.Backend != "" {
			solutions[index].Search.Backend = options.Backend
			solutions[index].Search.ServerElapsedMS = elapsedMS
			solutions[index].Search.RemoteWorkers = workers
			solutions[index].Search.MaxNodesApplied = maxNodes
			solutions[index].Search.MaxNodesCapped = maxNodesCapped
		}
	}
	output, err := render.SolutionsJSON(solutions)
	if err != nil {
		return Result{}, err
	}
	return Result{
		JSON: output,
		Metadata: Metadata{
			Backend:         options.Backend,
			ServerElapsedMS: elapsedMS,
			Workers:         workers,
			MaxNodesApplied: maxNodes,
			MaxNodesCapped:  maxNodesCapped,
		},
	}, nil
}

package websolve

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
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
	return solvePreparedCatalog(loadedCatalog, request.Scenario, options)
}

// SolvePreparedCatalog solves a validated scenario against an already parsed catalog.
// It retains scenario-specific filtering so one prepared catalog can serve many scenarios.
func SolvePreparedCatalog(loadedCatalog model.Catalog, loadedScenario scenario.Scenario, options Options) (Result, error) {
	if err := loadedScenario.Validate(); err != nil {
		return Result{}, err
	}
	return solvePreparedCatalog(loadedCatalog, loadedScenario, options)
}

func solvePreparedCatalog(loadedCatalog model.Catalog, loadedScenario scenario.Scenario, options Options) (Result, error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	filteredCatalog, err := catalog.FilterForHeroes(loadedCatalog, loadedScenario.HeroFilter)
	if err != nil {
		return Result{}, err
	}
	for _, itemID := range loadedScenario.ItemIDs() {
		if _, ok := filteredCatalog.Items[itemID]; !ok {
			return Result{}, fmt.Errorf("item %q is unavailable for the selected hero filter", itemID)
		}
	}

	gridMask := geometry.FullGridMask()
	if len(loadedScenario.Grid) > 0 {
		gridMask, err = geometry.ParseGridText(loadedScenario.GridText())
		if err != nil {
			return Result{}, err
		}
	}

	top := 3
	if loadedScenario.Top != nil {
		top = *loadedScenario.Top
	}
	maxNodes := int64(200000)
	if loadedScenario.MaxNodes != nil {
		maxNodes = *loadedScenario.MaxNodes
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
	if loadedScenario.Workers != nil {
		workers = *loadedScenario.Workers
	}
	if options.WorkerOverride > 0 {
		workers = options.WorkerOverride
	}
	noSkips := false
	if loadedScenario.NoSkips != nil {
		noSkips = *loadedScenario.NoSkips
	}
	stopOnCoverageCeiling := false
	if loadedScenario.StopOnCoverageCeiling != nil {
		stopOnCoverageCeiling = *loadedScenario.StopOnCoverageCeiling
	}
	repairSearch := maxNodes > 0
	if loadedScenario.RepairSearch != nil {
		repairSearch = *loadedScenario.RepairSearch
	}
	if maxNodes == 0 {
		repairSearch = false
	}

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(filteredCatalog, loadedScenario.ItemIDs(), gridMask, solver.Config{
		TopN:                  top,
		AllowSkips:            !noSkips,
		MaxNodes:              maxNodes,
		Workers:               workers,
		Priorities:            loadedScenario.Priorities,
		CoverageGroups:        loadedScenario.ModelCoverageGroups(),
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

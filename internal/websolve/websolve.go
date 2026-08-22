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

// LayoutEvaluationRequest is the wire format used by the manual layout
// comparer. Instance IDs are optional; omitted IDs are assigned in inventory
// order for the supplied item definition.
type LayoutEvaluationRequest struct {
	Scenario   scenario.Scenario        `json:"scenario"`
	Placements []LayoutPlacementRequest `json:"placements"`
}

type LayoutPlacementRequest struct {
	InstanceID string `json:"instance_id"`
	ItemID     string `json:"item_id"`
	Rotation   int    `json:"rotation"`
	Origin     []int  `json:"origin"`
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

func EvaluatePreparedLayoutJSON(loadedCatalog model.Catalog, input []byte) ([]byte, error) {
	var request LayoutEvaluationRequest
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, err
	}
	if err := request.Scenario.Validate(); err != nil {
		return nil, err
	}
	filteredCatalog, err := catalog.FilterForHeroes(loadedCatalog, request.Scenario.HeroFilter)
	if err != nil {
		return nil, err
	}
	itemIDs := request.Scenario.ItemIDs()
	for _, itemID := range itemIDs {
		if _, ok := filteredCatalog.Items[itemID]; !ok {
			return nil, fmt.Errorf("item %q is unavailable for the selected hero filter", itemID)
		}
	}
	gridMask := geometry.FullGridMask()
	if len(request.Scenario.Grid) > 0 {
		gridMask, err = geometry.ParseGridText(request.Scenario.GridText())
		if err != nil {
			return nil, err
		}
	}
	placements, err := requestedPlacements(itemIDs, request.Placements)
	if err != nil {
		return nil, err
	}
	solution, err := solver.EvaluateKnownLayout(filteredCatalog, itemIDs, gridMask, solver.Config{
		InitialPlacements: placements,
		PrioritySemantics: request.Scenario.PrioritySemantics,
		Priorities:        request.Scenario.Priorities,
		CoverageGroups:    request.Scenario.ModelCoverageGroups(),
	})
	if err != nil {
		return nil, err
	}
	return render.SolutionsJSON([]model.Solution{solution})
}

func requestedPlacements(itemIDs []string, requested []LayoutPlacementRequest) ([]model.Placement, error) {
	instances := solver.ExpandInventory(itemIDs)
	if len(requested) != len(instances) {
		return nil, fmt.Errorf("manual layout has %d placements, want %d", len(requested), len(instances))
	}
	instanceByID := make(map[string]model.InventoryInstance, len(instances))
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}
	used := make(map[string]struct{}, len(requested))
	placements := make([]model.Placement, 0, len(requested))
	for index, input := range requested {
		if len(input.Origin) != 2 {
			return nil, fmt.Errorf("manual placements[%d].origin must have row and column", index)
		}
		var instance model.InventoryInstance
		if input.InstanceID != "" {
			found, ok := instanceByID[input.InstanceID]
			if !ok {
				return nil, fmt.Errorf("manual placements[%d] references unknown instance %q", index, input.InstanceID)
			}
			instance = found
		} else {
			found := false
			for _, candidate := range instances {
				if candidate.ItemID == input.ItemID {
					if _, alreadyUsed := used[candidate.InstanceID]; !alreadyUsed {
						instance = candidate
						found = true
						break
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("manual placements[%d] has no unused instance for %q", index, input.ItemID)
			}
		}
		if input.ItemID != "" && input.ItemID != instance.ItemID {
			return nil, fmt.Errorf("manual placements[%d] item %q does not match %q", index, input.ItemID, instance.ItemID)
		}
		if _, exists := used[instance.InstanceID]; exists {
			return nil, fmt.Errorf("manual layout repeats instance %q", instance.InstanceID)
		}
		used[instance.InstanceID] = struct{}{}
		placements = append(placements, model.Placement{
			InstanceID: instance.InstanceID,
			ItemID:     instance.ItemID,
			Rotation:   input.Rotation,
			Origin:     model.Coord{Row: input.Origin[0], Col: input.Origin[1]},
		})
	}
	return placements, nil
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
	stopOnPriorityCeiling := false
	if loadedScenario.StopOnPriorityCeiling != nil {
		stopOnPriorityCeiling = *loadedScenario.StopOnPriorityCeiling
	}
	repairSearch := maxNodes > 0
	if loadedScenario.RepairSearch != nil {
		repairSearch = *loadedScenario.RepairSearch
	}
	if maxNodes == 0 {
		repairSearch = false
	}

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(filteredCatalog, loadedScenario.ItemIDs(), gridMask, webSolverConfig(
		loadedScenario,
		top,
		maxNodes,
		workers,
		noSkips,
		stopOnCoverageCeiling,
		stopOnPriorityCeiling,
		repairSearch,
		options,
	))
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

func webSolverConfig(
	loadedScenario scenario.Scenario,
	top int,
	maxNodes int64,
	workers int,
	noSkips bool,
	stopOnCoverageCeiling bool,
	stopOnPriorityCeiling bool,
	repairSearch bool,
	options Options,
) solver.Config {
	return solver.Config{
		TopN:                  top,
		AllowSkips:            !noSkips,
		MaxNodes:              maxNodes,
		Workers:               workers,
		PrioritySemantics:     loadedScenario.PrioritySemantics,
		Priorities:            loadedScenario.Priorities,
		CoverageGroups:        loadedScenario.ModelCoverageGroups(),
		StopOnCoverageCeiling: stopOnCoverageCeiling,
		StopOnPriorityCeiling: stopOnPriorityCeiling,
		RepairSearch:          repairSearch,
		ProgressReporter:      options.ProgressReporter,
		Context:               options.Context,
	}
}

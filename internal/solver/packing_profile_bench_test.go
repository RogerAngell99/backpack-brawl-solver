package solver

import (
	"fmt"
	"testing"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

func BenchmarkPlacementKey(b *testing.B) {
	for _, cells := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("cells=%d", cells), func(b *testing.B) {
			placement := packingProfilePlacementKeyFixture(cells)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if key := placementKey(placement); key == "" {
					b.Fatal("empty placement key")
				}
			}
		})
	}
}

func BenchmarkCanonicalCopyOrder(b *testing.B) {
	for _, existingCopies := range []int{0, 1, 2, 4} {
		outcomes := []struct {
			name     string
			accepted bool
		}{
			{name: "accepted", accepted: true},
		}
		if existingCopies > 0 {
			outcomes = append(outcomes, struct {
				name     string
				accepted bool
			}{name: "rejected", accepted: false})
		}
		for _, outcome := range outcomes {
			outcome := outcome
			b.Run(fmt.Sprintf("same_item_copies=%d/%s", existingCopies, outcome.name), func(b *testing.B) {
				candidate, existing := packingProfileCanonicalOrderFixture(existingCopies, outcome.accepted)
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if placementRespectsCanonicalCopyOrder(candidate, existing) != outcome.accepted {
						b.Fatalf("canonical order did not %s", outcome.name)
					}
				}
			})
		}
	}
}

func BenchmarkPackingFeasibility(b *testing.B) {
	for _, remainingCount := range []int{8, 16, 24} {
		for _, duplicated := range []bool{false, true} {
			name := "all_unique"
			if duplicated {
				name = "duplicated"
			}
			b.Run(fmt.Sprintf("remaining=%d/%s", remainingCount, name), func(b *testing.B) {
				remaining, options, occupied, placements := packingProfileFeasibilityFixture(remainingCount, duplicated)
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					_, _, feasible := packingFeasibility(remaining, options, occupied, placements)
					if !feasible {
						b.Fatal("expected feasible packing state")
					}
				}
			})
		}
	}
}

func BenchmarkPlacementOptions(b *testing.B) {
	catalog, instance := packingProfilePlacementOptionsFixture(b)
	gridMask := geometry.FullGridMask()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		options, err := PlacementOptions(catalog, instance, gridMask)
		if err != nil || len(options) == 0 {
			b.Fatalf("PlacementOptions options=%d err=%v", len(options), err)
		}
	}
}

func packingProfilePlacementKeyFixture(cellCount int) model.Placement {
	cells := make([]model.Coord, cellCount)
	for index := range cells {
		cells[index] = model.Coord{Row: index / geometry.GridCols, Col: index % geometry.GridCols}
	}
	return model.Placement{
		InstanceID:    "shape#0",
		ItemID:        "shape",
		OriginalIndex: 0,
		Rotation:      90,
		Origin:        model.Coord{Row: 2, Col: 3},
		Cells:         cells,
	}
}

func packingProfileCanonicalOrderFixture(existingCopies int, accepted bool) (model.Placement, []model.Placement) {
	if existingCopies == 0 {
		return packingProfileBenchmarkPlacement("copy#0", "copy", 0, 0), nil
	}
	candidateColumn := existingCopies
	if !accepted {
		candidateColumn = existingCopies - 1
	}
	candidate := packingProfileBenchmarkPlacement(fmt.Sprintf("copy#%d", existingCopies), "copy", existingCopies, candidateColumn)
	existing := make([]model.Placement, 0, existingCopies)
	for index := 0; index < existingCopies; index++ {
		column := index
		if !accepted && index == existingCopies-1 {
			column = existingCopies
		}
		existing = append(existing, packingProfileBenchmarkPlacement(fmt.Sprintf("copy#%d", index), "copy", index, column))
	}
	return candidate, existing
}

func packingProfileFeasibilityFixture(remainingCount int, duplicated bool) ([]model.InventoryInstance, map[string][]model.Placement, uint64, []model.Placement) {
	remaining := make([]model.InventoryInstance, 0, remainingCount)
	options := make(map[string][]model.Placement, remainingCount)
	placements := []model.Placement{packingProfileBenchmarkPlacement("anchor#0", "anchor", 0, 0)}
	if duplicated {
		placements[0] = packingProfileBenchmarkPlacement("copy#0", "copy", 0, 0)
	}
	for index := 0; index < remainingCount; index++ {
		itemID := fmt.Sprintf("unique-%02d", index)
		instanceID := itemID + "#0"
		originalIndex := index
		if duplicated {
			itemID = "copy"
			instanceID = fmt.Sprintf("copy#%d", index+1)
			originalIndex = index + 1
		}
		instance := model.InventoryInstance{InstanceID: instanceID, ItemID: itemID, OriginalIndex: originalIndex}
		remaining = append(remaining, instance)
		for column := 1; column <= 8; column++ {
			options[instanceID] = append(options[instanceID], packingProfileBenchmarkPlacement(instanceID, itemID, originalIndex, column))
		}
	}
	return remaining, options, uint64(1), placements
}

func packingProfileBenchmarkPlacement(instanceID string, itemID string, originalIndex int, column int) model.Placement {
	cell := model.Coord{Col: column}
	return model.Placement{
		InstanceID:    instanceID,
		ItemID:        itemID,
		OriginalIndex: originalIndex,
		Origin:        cell,
		Cells:         []model.Coord{cell},
		Mask:          uint64(1) << uint(column),
	}
}

func BenchmarkPlacementOptionsRepeatedCopies(b *testing.B) {
	catalog, instance := packingProfilePlacementOptionsFixture(b)
	gridMask := geometry.FullGridMask()
	instances := make([]model.InventoryInstance, 8)
	for index := range instances {
		instances[index] = instance
		instances[index].InstanceID = fmt.Sprintf("shape#%d", index)
		instances[index].OriginalIndex = index
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, copyInstance := range instances {
			options, err := PlacementOptions(catalog, copyInstance, gridMask)
			if err != nil || len(options) == 0 {
				b.Fatalf("PlacementOptions options=%d err=%v", len(options), err)
			}
		}
	}
}

func BenchmarkPlacementOptionsForInstanceClone(b *testing.B) {
	catalog, instance := packingProfilePlacementOptionsFixture(b)
	template, err := PlacementOptions(catalog, instance, geometry.FullGridMask())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		options := placementOptionsForInstance(template, instance)
		if len(options) != len(template) {
			b.Fatalf("clone options=%d want %d", len(options), len(template))
		}
	}
}

func BenchmarkConstellationRootMRVSelection(b *testing.B) {
	for _, remainingCount := range []int{8, 16, 24} {
		b.Run(fmt.Sprintf("remaining=%d", remainingCount), func(b *testing.B) {
			_, instances, options, state, _ := rootPackingProfileBenchmarkState(b, remainingCount, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, selected, ok := constellationRootMRVSelection(instances, state.remainingMask, options, state.occupied, state.placed)
				if !ok || len(selected) == 0 {
					b.Fatal("expected a legal MRV selection")
				}
			}
		})
	}
}

func BenchmarkConstellationRootMRVFeasibility(b *testing.B) {
	for _, remainingCount := range []int{8, 16, 24} {
		b.Run(fmt.Sprintf("remaining=%d", remainingCount), func(b *testing.B) {
			_, instances, options, state, _ := rootPackingProfileBenchmarkState(b, remainingCount, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, feasible := constellationRootMRVFeasibility(instances, state.remainingMask, options, state.occupied, state.placed)
				if !feasible {
					b.Fatal("expected a feasible state")
				}
			}
		})
	}
}

func BenchmarkFreeSpaceFragmentation(b *testing.B) {
	gridMask := geometry.FullGridMask()
	masks := map[string]uint64{
		"full_open":       0,
		"holes":           (uint64(1) << 10) | (uint64(1) << 20) | (uint64(1) << 30) | (uint64(1) << 40),
		"bottleneck":      0b111111111111111111111111111111111111111111000000000000,
		"narrow_corridor": 0b111111111111111111111111111111111111111111111011111111,
		"dense_occupied":  ^uint64(0) >> 8,
	}
	for name, occupied := range masks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = freeSpaceFragmentation(gridMask, occupied)
			}
		})
	}
}

func BenchmarkConstellationRootMRVStateKey(b *testing.B) {
	for _, placedCount := range []int{4, 12, 24} {
		b.Run(fmt.Sprintf("placed=%d", placedCount), func(b *testing.B) {
			_, _, _, state, _ := rootPackingProfileBenchmarkState(b, 24, placedCount)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if key := constellationRootMRVStateKey(state); key == "" {
					b.Fatal("empty state key")
				}
			}
		})
	}
}

func BenchmarkEvaluatePartialScore(b *testing.B) {
	for _, objective := range []struct {
		name       string
		outgoingV3 bool
	}{
		{name: "no_stars"},
		{name: "outgoing_v3", outgoingV3: true},
	} {
		objective := objective
		b.Run(objective.name, func(b *testing.B) {
			for _, placedCount := range []int{4, 12, 24} {
				b.Run(fmt.Sprintf("placements=%d", placedCount), func(b *testing.B) {
					catalog, placements, config := rootPackingProfileScoreBenchmarkFixture(b, placedCount, objective.outgoingV3)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_ = evaluateScoreForConfig(catalog, placements, config)
					}
				})
			}
		})
	}
}

func BenchmarkConstellationRootPackingFinishMRVDepth(b *testing.B) {
	for _, benchmark := range []struct {
		frontierSize int
		beamWidth    int
	}{
		{frontierSize: 128, beamWidth: 128},
		{frontierSize: 512, beamWidth: 128},
		{frontierSize: 2048, beamWidth: 128},
		{frontierSize: 8192, beamWidth: 128},
	} {
		benchmark := benchmark
		b.Run(fmt.Sprintf("frontier=%d/beam=%d", benchmark.frontierSize, benchmark.beamWidth), func(b *testing.B) {
			_, _, _, _, config := rootPackingProfileBenchmarkState(b, 24, 4)
			frontier := rootPackingProfileBenchmarkFrontier(benchmark.frontierSize)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				depth := model.ConstellationRootPackingDepthDiagnostic{Depth: 1}
				states := constellationRootPackingFinishMRVDepth(frontier, &depth, nil, nil, nil, benchmark.beamWidth, config)
				want := min(benchmark.frontierSize, benchmark.beamWidth)
				if len(states) != want {
					b.Fatalf("states=%d want %d", len(states), want)
				}
			}
		})
	}
}

func BenchmarkPackingSession1K(b *testing.B)  { benchmarkPackingSession(b, 1_000) }
func BenchmarkPackingSession10K(b *testing.B) { benchmarkPackingSession(b, 10_000) }

func BenchmarkNodeLedgerChargeSerial(b *testing.B) {
	ledger := newNodeLedger(0, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ledger.charge("benchmark") {
			b.Fatal("unexpected denied ledger charge")
		}
	}
}

func BenchmarkNodeLedgerChargeParallel(b *testing.B) {
	ledger := newNodeLedger(0, nil)
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if !ledger.charge("benchmark") {
				b.Fatal("unexpected denied ledger charge")
			}
		}
	})
}

func packingProfilePlacementOptionsFixture(b testing.TB) (model.Catalog, model.InventoryInstance) {
	b.Helper()
	catalog := model.Catalog{Items: map[string]model.Item{
		"shape": {ID: "shape", Shape: []model.Coord{{}, {Col: 1}, {Row: 1}}, Rotations: []int{0, 90, 180, 270}},
	}}
	return catalog, model.InventoryInstance{InstanceID: "shape#0", ItemID: "shape", OriginalIndex: 0}
}

func rootPackingProfileBenchmarkState(b testing.TB, totalInstances int, placedCount int) (model.Catalog, []model.InventoryInstance, map[string][]model.Placement, constellationRootMRVState, Config) {
	b.Helper()
	if totalInstances > 32 || placedCount > totalInstances {
		b.Fatalf("invalid benchmark fixture total=%d placed=%d", totalInstances, placedCount)
	}
	catalog := model.Catalog{Items: map[string]model.Item{
		"piece": {ID: "piece", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	itemIDs := make([]string, totalInstances)
	for index := range itemIDs {
		itemIDs[index] = "piece"
	}
	instances := ExpandInventory(itemIDs)
	options := make(map[string][]model.Placement, len(instances))
	for _, instance := range instances {
		for position := 0; position < 32; position++ {
			options[instance.InstanceID] = append(options[instance.InstanceID], model.Placement{
				InstanceID:    instance.InstanceID,
				ItemID:        instance.ItemID,
				OriginalIndex: instance.OriginalIndex,
				Origin:        model.Coord{Row: position / geometry.GridCols, Col: position % geometry.GridCols},
				Cells:         []model.Coord{{Row: position / geometry.GridCols, Col: position % geometry.GridCols}},
				Mask:          uint64(1) << uint(position),
			})
		}
	}
	placed := make([]model.Placement, 0, placedCount)
	occupied := uint64(0)
	for index := 0; index < placedCount; index++ {
		placement := options[instances[index].InstanceID][index]
		placed = append(placed, placement)
		occupied |= placement.Mask
	}
	remainingMask := uint64(0)
	for index := placedCount; index < len(instances); index++ {
		remainingMask |= uint64(1) << uint(instances[index].OriginalIndex)
	}
	config := Config{TopN: 1, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3}
	policy := resolveSearchPolicy(config, 10_000)
	policy.ConstellationSeedPackingStrategy = constellationPackingStrategyStateMRV
	policy.ConstellationSeedPackingBeamWidth = 128
	config.policy = &policy
	return catalog, instances, options, constellationRootMRVState{
		packingSeedState: packingSeedState{occupied: occupied, placed: placed, score: model.Score{ItemCount: len(placed)}, key: "benchmark-root"},
		remainingMask:    remainingMask,
		remainingArea:    totalInstances - placedCount,
	}, config
}

func rootPackingProfileScoreBenchmarkFixture(b testing.TB, placedCount int, outgoingV3 bool) (model.Catalog, []model.Placement, Config) {
	b.Helper()
	if !outgoingV3 {
		catalog, _, _, state, config := rootPackingProfileBenchmarkState(b, 24, placedCount)
		return catalog, state.placed, config
	}
	if placedCount != 4 && placedCount != 12 && placedCount != 24 {
		b.Fatalf("unsupported score benchmark placement count %d", placedCount)
	}
	catalog := model.Catalog{Items: map[string]model.Item{
		"source-a": {
			ID:        "source-a",
			Types:     []string{"Source", "SourceA"},
			Shape:     []model.Coord{{}},
			Rotations: []int{0},
			Stars: []model.Star{
				{Offset: model.Coord{Col: 1}, TargetTypes: []string{"AOnly"}},
				{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Shared"}},
				{Offset: model.Coord{Col: 2}, TargetTypes: []string{"Shared"}},
				{Offset: model.Coord{Col: 2}, TargetTypes: []string{"SourceB"}},
			},
		},
		"source-b": {
			ID:        "source-b",
			Types:     []string{"Source", "SourceB"},
			Shape:     []model.Coord{{}},
			Rotations: []int{0},
			Stars: []model.Star{
				{Offset: model.Coord{Col: -1}, TargetTypes: []string{"BOnly"}},
				{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Shared"}},
				{Offset: model.Coord{Col: -2}, TargetTypes: []string{"Shared"}},
				{Offset: model.Coord{Col: -2}, TargetTypes: []string{"SourceA"}},
			},
		},
		"a-target":      {ID: "a-target", Types: []string{"AOnly"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b-target":      {ID: "b-target", Types: []string{"BOnly"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"shared-target": {ID: "shared-target", Types: []string{"Shared"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"neutral":       {ID: "neutral", Types: []string{"Neutral"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	counts := make(map[string]int)
	placements := make([]model.Placement, 0, placedCount)
	appendPlacement := func(itemID string) {
		index := len(placements)
		instanceIndex := counts[itemID]
		counts[itemID]++
		position := model.Coord{Row: index / geometry.GridCols, Col: index % geometry.GridCols}
		item := catalog.Items[itemID]
		starPositions := make([]model.StarPosition, 0, len(item.Stars))
		for _, star := range item.Stars {
			starPositions = append(starPositions, model.StarPosition{
				Star:     star,
				Position: model.Coord{Row: position.Row + star.Offset.Row, Col: position.Col + star.Offset.Col},
			})
		}
		placements = append(placements, model.Placement{
			InstanceID:    fmt.Sprintf("%s#%d", itemID, instanceIndex),
			ItemID:        itemID,
			OriginalIndex: index,
			Origin:        position,
			Cells:         []model.Coord{position},
			Mask:          uint64(1) << uint(index),
			StarPositions: starPositions,
		})
	}
	for _, itemID := range []string{"source-a", "shared-target", "source-b", "neutral"} {
		if len(placements) < placedCount {
			appendPlacement(itemID)
		}
	}
	for len(placements) < placedCount {
		for _, itemID := range []string{"source-a", "a-target", "shared-target", "b-target", "source-b"} {
			if len(placements) == placedCount {
				break
			}
			appendPlacement(itemID)
		}
	}
	return catalog, placements, Config{
		TopN:              1,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source-a", "star_source:source-b"},
	}
}

func TestRootPackingProfileScoreBenchmarkFixtureExercisesOutgoingV3(t *testing.T) {
	catalog, placements, config := rootPackingProfileScoreBenchmarkFixture(t, 24, true)
	score := evaluateScoreForConfig(catalog, placements, config)
	if len(score.PriorityCounts) != 2 || score.PriorityCounts[0] == 0 || score.PriorityCounts[1] == 0 || score.StarCount == 0 || score.StarTargetBreadth == 0 || score.StarReciprocalPairs == 0 || score.StarSourceDefinitionDiversity < 2 {
		t.Fatalf("outgoing-v3 score fixture did not exercise structural scoring: %+v", score)
	}
}

func rootPackingProfileBenchmarkFrontier(size int) map[string]constellationRootMRVState {
	frontier := make(map[string]constellationRootMRVState, size)
	for index := 0; index < size; index++ {
		key := fmt.Sprintf("frontier-%05d", index)
		frontier[key] = constellationRootMRVState{packingSeedState: packingSeedState{key: key, score: model.Score{StarCount: index % 17, ItemCount: index % 9}}, remainingMask: 1}
	}
	return frontier
}

func rootPackingProfileLongSessionFixture() (model.Catalog, []model.InventoryInstance, map[string][]model.Placement, constellationSkeleton, Config, uint64) {
	const itemCount = 8
	const optionCount = 32
	catalog := model.Catalog{Items: map[string]model.Item{
		"piece": {ID: "piece", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	itemIDs := make([]string, itemCount)
	for index := range itemIDs {
		itemIDs[index] = "piece"
	}
	instances := ExpandInventory(itemIDs)
	options := make(map[string][]model.Placement, len(instances))
	for _, instance := range instances {
		for position := 0; position < optionCount; position++ {
			coord := model.Coord{Row: position / geometry.GridCols, Col: position % geometry.GridCols}
			options[instance.InstanceID] = append(options[instance.InstanceID], model.Placement{
				InstanceID:    instance.InstanceID,
				ItemID:        instance.ItemID,
				OriginalIndex: instance.OriginalIndex,
				Origin:        coord,
				Cells:         []model.Coord{coord},
				Mask:          uint64(1) << uint(position),
			})
		}
	}
	config := Config{TopN: 1, MaxNodes: 10_000}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	policy.ConstellationSeedPackingStrategy = constellationPackingStrategyStateMRV
	policy.ConstellationSeedPackingBeamWidth = 128
	config.policy = &policy
	return catalog, instances, options, constellationSkeleton{rootID: "benchmark-root", signature: "benchmark-root"}, config, (uint64(1) << optionCount) - 1
}

func TestRootPackingProfileLongSessionFixtureSupportsAdvertisedAllocations(t *testing.T) {
	for _, allocation := range []int64{1_000, 10_000} {
		catalog, instances, options, root, config, gridMask := rootPackingProfileLongSessionFixture()
		session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
		result := session.Run(allocation)
		if result.nodes != allocation || session.Done() {
			t.Fatalf("allocation=%d nodes=%d done=%t termination=%q", allocation, result.nodes, session.Done(), result.terminationReason)
		}
	}
}

func benchmarkPackingSession(b *testing.B, allocation int64) {
	catalog, instances, options, root, config, gridMask := rootPackingProfileLongSessionFixture()
	for _, withLedger := range []bool{false, true} {
		name := "without_ledger"
		if withLedger {
			name = "with_ledger"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				reportNode := func(bool) bool { return true }
				if withLedger {
					ledger := newNodeLedger(0, nil)
					reportNode = func(bool) bool { return ledger.charge("benchmark") }
				}
				session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, reportNode)
				result := session.Run(allocation)
				if result.nodes != allocation || session.Done() {
					b.Fatalf("allocation=%d nodes=%d done=%t termination=%q", allocation, result.nodes, session.Done(), result.terminationReason)
				}
			}
		})
	}
}

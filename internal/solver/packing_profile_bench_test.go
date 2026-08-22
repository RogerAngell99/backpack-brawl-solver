package solver

import (
	"fmt"
	"testing"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

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
	for _, placedCount := range []int{4, 12, 24} {
		b.Run(fmt.Sprintf("placements=%d", placedCount), func(b *testing.B) {
			catalog, _, _, state, config := rootPackingProfileBenchmarkState(b, 24, placedCount)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = evaluateScoreForConfig(catalog, state.placed, config)
			}
		})
	}
}

func BenchmarkConstellationRootPackingFinishMRVDepth(b *testing.B) {
	for _, frontierSize := range []int{128, 512, 2048, 8192} {
		b.Run(fmt.Sprintf("frontier=%d", frontierSize), func(b *testing.B) {
			_, _, _, _, config := rootPackingProfileBenchmarkState(b, 24, 4)
			frontier := rootPackingProfileBenchmarkFrontier(frontierSize)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				depth := model.ConstellationRootPackingDepthDiagnostic{Depth: 1}
				states := constellationRootPackingFinishMRVDepth(frontier, &depth, nil, nil, nil, frontierSize, config)
				if len(states) != frontierSize {
					b.Fatalf("states=%d want %d", len(states), frontierSize)
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

func rootPackingProfileBenchmarkFrontier(size int) map[string]constellationRootMRVState {
	frontier := make(map[string]constellationRootMRVState, size)
	for index := 0; index < size; index++ {
		key := fmt.Sprintf("frontier-%05d", index)
		frontier[key] = constellationRootMRVState{packingSeedState: packingSeedState{key: key, score: model.Score{StarCount: index % 17, ItemCount: index % 9}}, remainingMask: 1}
	}
	return frontier
}

func benchmarkPackingSession(b *testing.B, allocation int64) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
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
				if !session.Done() {
					result = session.FinalizeBudgetExhausted()
				}
				if len(result.solutions) == 0 {
					b.Fatal("packing session did not produce a solution")
				}
			}
		})
	}
}

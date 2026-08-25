package solver

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

const outgoingIndexGeneratedSeed int64 = 0x52314947

var (
	outgoingPlacementIndexBenchmarkSink outgoingPlacementIndex
	outgoingPlacementMapBenchmarkSink   map[string]model.Placement
	outgoingUpperBenchmarkSink          []int
)

func TestOutgoingPlacementIndexDifferentialMatrix(t *testing.T) {
	baseInstances := []model.InventoryInstance{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0},
		{InstanceID: "food#1", ItemID: "food", OriginalIndex: 1},
	}
	basePlacements := placementsForOutgoingIndexInstances(baseInstances)
	boundaryInstances := []model.InventoryInstance{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0},
		{InstanceID: "food#63", ItemID: "food", OriginalIndex: 63},
	}

	all64Instances := make([]model.InventoryInstance, 64)
	for index := range all64Instances {
		itemID := "food"
		if index%8 == 0 {
			itemID = "source"
		}
		all64Instances[index] = model.InventoryInstance{InstanceID: fmt.Sprintf("%s#%d", itemID, index), ItemID: itemID, OriginalIndex: index}
	}
	tooManyInstances := append([]model.InventoryInstance(nil), all64Instances...)
	tooManyInstances = append(tooManyInstances, model.InventoryInstance{InstanceID: "food#64", ItemID: "food", OriginalIndex: 64})

	sparseInstances := []model.InventoryInstance{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0},
		{InstanceID: "food#7", ItemID: "food", OriginalIndex: 7},
		{InstanceID: "food#31", ItemID: "food", OriginalIndex: 31},
		{InstanceID: "food#63", ItemID: "food", OriginalIndex: 63},
	}

	cloneInstances := func(instances []model.InventoryInstance) []model.InventoryInstance {
		return append([]model.InventoryInstance(nil), instances...)
	}
	clonePlacements := func(placements []model.Placement) []model.Placement {
		return append([]model.Placement(nil), placements...)
	}

	negativePlacement := clonePlacements(basePlacements[:1])
	negativePlacement[0].OriginalIndex = -1
	index64Placement := clonePlacements(basePlacements[:1])
	index64Placement[0].OriginalIndex = 64
	largeIndexPlacement := clonePlacements(basePlacements[:1])
	largeIndexPlacement[0].OriginalIndex = 1 << 20
	negativeInventoryIndex := cloneInstances(baseInstances)
	negativeInventoryIndex[1].OriginalIndex = -1
	inventoryIndex64 := cloneInstances(baseInstances)
	inventoryIndex64[1].OriginalIndex = 64
	largeInventoryIndex := cloneInstances(baseInstances)
	largeInventoryIndex[1].OriginalIndex = 1 << 20
	duplicateInventoryIndex := cloneInstances(baseInstances)
	duplicateInventoryIndex[1].OriginalIndex = 0
	duplicateInventoryID := cloneInstances(baseInstances)
	duplicateInventoryID[1].InstanceID = duplicateInventoryID[0].InstanceID
	emptyInventoryID := cloneInstances(baseInstances)
	emptyInventoryID[1].InstanceID = ""
	duplicatePlacementIndex := append(clonePlacements(basePlacements), basePlacements[0])
	duplicatePlacementID := clonePlacements(basePlacements)
	duplicatePlacementID[1].InstanceID = duplicatePlacementID[0].InstanceID
	lastWritePlacements := append(clonePlacements(basePlacements), basePlacements[0])
	lastWritePlacements[2].StarPositions = append([]model.StarPosition(nil), basePlacements[0].StarPositions...)
	lastWritePlacements[2].Rotation = 3
	lastWritePlacements[2].Origin = model.Coord{Row: 5, Col: 8}
	lastWritePlacements[2].Mask = uint64(1) << 53
	lastWritePlacements[2].StarPositions[0].Position = model.Coord{Row: 5, Col: 8}
	unknownPlacement := clonePlacements(basePlacements)
	unknownPlacement = append(unknownPlacement, outgoingIndexPlacement(model.InventoryInstance{InstanceID: "unknown#2", ItemID: "food", OriginalIndex: 2}, 0))
	wrongIDPlacement := clonePlacements(basePlacements)
	wrongIDPlacement[1].InstanceID = "wrong#1"
	reorderedPlacements := []model.Placement{basePlacements[1], basePlacements[0]}
	physicalCopies := []model.InventoryInstance{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0},
		{InstanceID: "food#1", ItemID: "food", OriginalIndex: 1},
		{InstanceID: "food#2", ItemID: "food", OriginalIndex: 2},
	}
	rotatedPlacements := clonePlacements(basePlacements)
	rotatedPlacements[0].Rotation = 1
	rotatedPlacements[1].Rotation = 3
	divergentItemPlacement := clonePlacements(basePlacements)
	divergentItemPlacement[1].ItemID = "unexpected-item"

	cases := []struct {
		name       string
		instances  []model.InventoryInstance
		placements []model.Placement
		wantFast   bool
	}{
		{name: "empty inventory", wantFast: true},
		{name: "empty placements", instances: baseInstances, wantFast: true},
		{name: "one instance", instances: baseInstances[:1], placements: basePlacements[:1], wantFast: true},
		{name: "indices zero and sixty-three", instances: boundaryInstances, placements: placementsForOutgoingIndexInstances(boundaryInstances), wantFast: true},
		{name: "negative placement index", instances: baseInstances, placements: negativePlacement, wantFast: false},
		{name: "placement index sixty-four", instances: baseInstances, placements: index64Placement, wantFast: false},
		{name: "placement index much larger", instances: baseInstances, placements: largeIndexPlacement, wantFast: false},
		{name: "negative inventory index", instances: negativeInventoryIndex, wantFast: false},
		{name: "inventory index sixty-four", instances: inventoryIndex64, wantFast: false},
		{name: "inventory index much larger", instances: largeInventoryIndex, wantFast: false},
		{name: "inventory larger than sixty-four", instances: tooManyInstances, wantFast: false},
		{name: "duplicate inventory index", instances: duplicateInventoryIndex, placements: basePlacements, wantFast: false},
		{name: "duplicate inventory ID", instances: duplicateInventoryID, placements: basePlacements, wantFast: false},
		{name: "empty inventory ID", instances: emptyInventoryID, placements: basePlacements, wantFast: false},
		{name: "duplicate placement index", instances: baseInstances, placements: duplicatePlacementIndex, wantFast: false},
		{name: "duplicate placement ID", instances: baseInstances, placements: duplicatePlacementID, wantFast: false},
		{name: "last write wins", instances: baseInstances, placements: lastWritePlacements, wantFast: false},
		{name: "unknown placement", instances: baseInstances, placements: unknownPlacement, wantFast: false},
		{name: "valid index wrong ID", instances: baseInstances, placements: wrongIDPlacement, wantFast: false},
		{name: "missing source placement", instances: baseInstances, placements: basePlacements[1:], wantFast: true},
		{name: "missing target placement", instances: baseInstances, placements: basePlacements[:1], wantFast: true},
		{name: "placement order differs", instances: baseInstances, placements: reorderedPlacements, wantFast: true},
		{name: "duplicate physical copies", instances: physicalCopies, placements: placementsForOutgoingIndexInstances(physicalCopies), wantFast: true},
		{name: "multiple rotations", instances: baseInstances, placements: rotatedPlacements, wantFast: true},
		{name: "placement ItemID differs", instances: baseInstances, placements: divergentItemPlacement, wantFast: true},
		{name: "all sixty-four indices occupied", instances: all64Instances, placements: placementsForOutgoingIndexInstances(all64Instances), wantFast: true},
		{name: "sparse domain with gaps", instances: sparseInstances, placements: placementsForOutgoingIndexInstances(sparseInstances), wantFast: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := newOutgoingIndexTestContext(testCase.instances)
			index, fast := ctx.buildOutgoingPlacementIndex(testCase.placements)
			if fast != testCase.wantFast {
				t.Fatalf("fast path=%v, want %v", fast, testCase.wantFast)
			}
			legacy := ctx.upperPriorityCountsLegacy(testCase.placements)
			if got := ctx.upperPriorityCounts(testCase.placements); !reflect.DeepEqual(got, legacy) {
				t.Fatalf("wrapper upper=%v, legacy=%v", got, legacy)
			}
			if fast {
				indexed := ctx.upperPriorityCountsIndexed(testCase.placements, index)
				if !reflect.DeepEqual(indexed, legacy) {
					t.Fatalf("indexed upper=%v, legacy=%v", indexed, legacy)
				}
			}
		})
	}

	t.Run("legacy duplicate InstanceID uses the last placement", func(t *testing.T) {
		ctx := newOutgoingIndexTestContext(baseInstances)
		got := ctx.upperPriorityCountsLegacy(lastWritePlacements)
		lastOnly := ctx.upperPriorityCountsLegacy([]model.Placement{basePlacements[1], lastWritePlacements[2]})
		firstOnly := ctx.upperPriorityCountsLegacy(basePlacements)
		if !reflect.DeepEqual(got, lastOnly) {
			t.Fatalf("duplicate upper=%v, last-only upper=%v", got, lastOnly)
		}
		if reflect.DeepEqual(got, firstOnly) {
			t.Fatalf("fixture did not distinguish last-write result: duplicate=%v first-only=%v", got, firstOnly)
		}
	})
}

func TestOutgoingPlacementIndexGeneratedValidCorpus(t *testing.T) {
	for state := 0; state < 1000; state++ {
		ctx, placements := generatedOutgoingIndexState(outgoingIndexGeneratedSeed, state)
		index, ok := ctx.buildOutgoingPlacementIndex(placements)
		if !ok {
			t.Fatalf("state %d refused valid generated domain", state)
		}
		legacy := ctx.upperPriorityCountsLegacy(placements)
		indexed := ctx.upperPriorityCountsIndexed(placements, index)
		if !reflect.DeepEqual(indexed, legacy) {
			t.Fatalf("state %d indexed upper=%v, legacy=%v", state, indexed, legacy)
		}
	}
}

func TestOutgoingPlacementIndexGeneratedMalformedCorpus(t *testing.T) {
	for state := 0; state < 1000; state++ {
		ctx, placements := generatedOutgoingIndexState(outgoingIndexGeneratedSeed+1, state)
		instances := append([]model.InventoryInstance(nil), ctx.instances...)
		placements = append([]model.Placement(nil), placements...)

		switch state % 6 {
		case 0:
			placements[0].OriginalIndex = -1
		case 1:
			placements = append(placements, placements[0])
		case 2:
			if len(instances) == 1 {
				instances = append(instances, model.InventoryInstance{InstanceID: instances[0].InstanceID, ItemID: "food", OriginalIndex: 63})
			} else {
				instances[1].InstanceID = instances[0].InstanceID
			}
			ctx = newOutgoingIndexTestContext(instances)
		case 3:
			instances[0].InstanceID = ""
			ctx = newOutgoingIndexTestContext(instances)
		case 4:
			placements[0].InstanceID = "mismatched-instance"
		case 5:
			unknownIndex := firstUnusedOutgoingIndex(instances)
			placements = append(placements, outgoingIndexPlacement(model.InventoryInstance{InstanceID: "unknown-instance", ItemID: "food", OriginalIndex: unknownIndex}, state))
		}

		if _, ok := ctx.buildOutgoingPlacementIndex(placements); ok {
			t.Fatalf("state %d accepted malformed state", state)
		}
		legacy := ctx.upperPriorityCountsLegacy(placements)
		if got := ctx.upperPriorityCounts(placements); !reflect.DeepEqual(got, legacy) {
			t.Fatalf("state %d fallback upper=%v, legacy=%v", state, got, legacy)
		}
	}
}

func TestOutgoingPlacementIndexBuilderDoesNotAllocate(t *testing.T) {
	ctx, placements := outgoingIndexBenchmarkFixture(64)
	allocations := testing.AllocsPerRun(1000, func() {
		index, ok := ctx.buildOutgoingPlacementIndex(placements)
		if !ok {
			panic("valid benchmark domain refused")
		}
		outgoingPlacementIndexBenchmarkSink = index
	})
	if allocations != 0 {
		t.Fatalf("indexed builder allocations/run=%v, want 0", allocations)
	}
}

func BenchmarkOutgoingPlacementLookup(b *testing.B) {
	for _, size := range []int{1, 4, 8, 16, 32, 64} {
		ctx, placements := outgoingIndexBenchmarkFixture(size)
		b.Run(fmt.Sprintf("legacy/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				outgoingPlacementMapBenchmarkSink = placementByInstanceID(placements)
			}
		})
		b.Run(fmt.Sprintf("indexed/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				index, ok := ctx.buildOutgoingPlacementIndex(placements)
				if !ok {
					b.Fatal("valid benchmark domain refused")
				}
				outgoingPlacementIndexBenchmarkSink = index
			}
		})
	}
}

func BenchmarkOutgoingUpperPriorityCounts(b *testing.B) {
	for _, size := range []int{1, 4, 8, 16, 32, 64} {
		ctx, placements := outgoingIndexBenchmarkFixture(size)
		index, ok := ctx.buildOutgoingPlacementIndex(placements)
		if !ok {
			b.Fatalf("size %d valid benchmark domain refused", size)
		}
		b.Run(fmt.Sprintf("legacy/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				outgoingUpperBenchmarkSink = ctx.upperPriorityCountsLegacy(placements)
			}
		})
		b.Run(fmt.Sprintf("indexed/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				outgoingUpperBenchmarkSink = ctx.upperPriorityCountsIndexed(placements, index)
			}
		})
	}
}

func generatedOutgoingIndexState(seed int64, state int) (*outgoingBoundContext, []model.Placement) {
	random := rand.New(rand.NewSource(seed + int64(state)*7919))
	instanceCount := 1 + random.Intn(24)
	originals := random.Perm(64)[:instanceCount]
	instances := make([]model.InventoryInstance, instanceCount)
	for index, originalIndex := range originals {
		itemID := "food"
		if index%4 == 0 {
			itemID = "source"
		}
		instances[index] = model.InventoryInstance{
			InstanceID:    fmt.Sprintf("generated-%04d-%02d", state, index),
			ItemID:        itemID,
			OriginalIndex: originalIndex,
		}
	}
	placements := make([]model.Placement, 0, instanceCount)
	for index, instance := range instances {
		if index != 0 && random.Intn(3) == 0 {
			continue
		}
		placements = append(placements, outgoingIndexPlacement(instance, random.Intn(4)))
	}
	random.Shuffle(len(placements), func(left, right int) {
		placements[left], placements[right] = placements[right], placements[left]
	})
	ctx := newOutgoingIndexTestContext(instances)
	for _, placement := range placements {
		var targets uint64
		for _, instance := range instances {
			if instance.InstanceID != placement.InstanceID && random.Intn(2) == 0 {
				targets |= uint64(1) << uint(instance.OriginalIndex)
			}
		}
		ctx.potential.outgoingTargets[coveragePlacementKey(placement)] = targets
	}
	return ctx, placements
}

func outgoingIndexBenchmarkFixture(size int) (*outgoingBoundContext, []model.Placement) {
	instances := make([]model.InventoryInstance, size)
	for index := range instances {
		itemID := "food"
		if index%4 == 0 {
			itemID = "source"
		}
		instances[index] = model.InventoryInstance{InstanceID: fmt.Sprintf("bench-%02d", index), ItemID: itemID, OriginalIndex: index}
	}
	return newOutgoingIndexTestContext(instances), placementsForOutgoingIndexInstances(instances)
}

func newOutgoingIndexTestContext(instances []model.InventoryInstance) *outgoingBoundContext {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID:        "source",
			Shape:     []model.Coord{{}},
			Stars:     []model.Star{{TargetTypes: []string{"Food"}}, {TargetTypes: []string{"Food"}}, {TargetTypes: []string{"Food"}}},
			Rotations: []int{0, 1, 2, 3},
		},
		"food": {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0, 1, 2, 3}},
	}}
	potential := &starPotentialContext{
		outgoingTargets:         map[string]uint64{},
		instanceOutgoingTargets: map[string]uint64{},
	}
	var inventoryMask uint64
	for _, instance := range instances {
		if instance.OriginalIndex >= 0 && instance.OriginalIndex < 64 {
			inventoryMask |= uint64(1) << uint(instance.OriginalIndex)
		}
	}
	for _, instance := range instances {
		potential.instanceOutgoingTargets[instance.InstanceID] = inventoryMask
	}
	return &outgoingBoundContext{
		catalog:       catalog,
		instances:     instances,
		priorityItems: []string{"source"},
		potential:     potential,
		indexDomain:   newOutgoingPlacementIndexDomain(instances),
	}
}

func placementsForOutgoingIndexInstances(instances []model.InventoryInstance) []model.Placement {
	placements := make([]model.Placement, len(instances))
	for index, instance := range instances {
		placements[index] = outgoingIndexPlacement(instance, index%4)
	}
	return placements
}

func outgoingIndexPlacement(instance model.InventoryInstance, rotation int) model.Placement {
	cellIndex := instance.OriginalIndex
	if cellIndex < 0 {
		cellIndex = -cellIndex
	}
	cellIndex %= 54
	origin := model.Coord{Row: cellIndex / 9, Col: cellIndex % 9}
	starCell := (cellIndex + 1 + rotation) % 54
	return model.Placement{
		InstanceID:    instance.InstanceID,
		ItemID:        instance.ItemID,
		OriginalIndex: instance.OriginalIndex,
		Rotation:      rotation,
		Origin:        origin,
		Cells:         []model.Coord{origin},
		StarPositions: []model.StarPosition{{
			Star:     model.Star{TargetTypes: []string{"Food"}},
			Position: model.Coord{Row: starCell / 9, Col: starCell % 9},
		}},
		Mask: uint64(1) << uint(cellIndex),
	}
}

func firstUnusedOutgoingIndex(instances []model.InventoryInstance) int {
	var used uint64
	for _, instance := range instances {
		if instance.OriginalIndex >= 0 && instance.OriginalIndex < 64 {
			used |= uint64(1) << uint(instance.OriginalIndex)
		}
	}
	for index := 0; index < 64; index++ {
		if used&(uint64(1)<<uint(index)) == 0 {
			return index
		}
	}
	return 64
}

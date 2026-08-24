package solver

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestWritePlacementKeyIntMatchesLegacyFormatting(t *testing.T) {
	for value := 0; value <= 999; value++ {
		var builder strings.Builder
		writePlacementKeyInt(&builder, value)
		if got, want := builder.String(), fmt.Sprintf("%03d", value); got != want {
			t.Fatalf("value=%d: got %q want %q", value, got, want)
		}
	}
}

func TestWritePlacementKeyIntFallbackMatchesLegacyFormatting(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, value := range []int{minInt, -1001, -1000, -999, -100, -99, -10, -9, -1, 1000, 1001, 10_000, maxInt} {
		var builder strings.Builder
		writePlacementKeyInt(&builder, value)
		if got, want := builder.String(), fmt.Sprintf("%03d", value); got != want {
			t.Fatalf("value=%d: got %q want %q", value, got, want)
		}
	}
}

func TestPlacementKeyMatchesLegacyFixtures(t *testing.T) {
	fixtures := []struct {
		name      string
		placement model.Placement
	}{
		{name: "empty"},
		{
			name: "ordinary",
			placement: model.Placement{
				Rotation: 90,
				Origin:   model.Coord{Row: 2, Col: 3},
				Cells:    []model.Coord{{Row: 2, Col: 3}, {Row: 2, Col: 4}, {Row: 3, Col: 3}},
			},
		},
		{
			name: "duplicate_cells",
			placement: model.Placement{
				Rotation: 270,
				Origin:   model.Coord{Row: 999, Col: 0},
				Cells:    []model.Coord{{Row: 7, Col: 8}, {Row: 7, Col: 8}},
			},
		},
		{
			name: "reordered_cells",
			placement: model.Placement{
				Rotation: 180,
				Origin:   model.Coord{Row: 4, Col: 5},
				Cells:    []model.Coord{{Row: 5, Col: 6}, {Row: 4, Col: 5}},
			},
		},
		{
			name: "fallback_values",
			placement: model.Placement{
				Rotation: -90,
				Origin:   model.Coord{Row: 1000, Col: -1},
				Cells:    []model.Coord{{Row: -100, Col: 10_000}},
			},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if got, want := placementKey(fixture.placement), legacyPlacementKeyForTest(fixture.placement); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestPlacementKeyMatchesLegacyGeneratedCorpus(t *testing.T) {
	random := rand.New(rand.NewSource(0x51A7E))
	for index := 0; index < 10_000; index++ {
		placement := generatedPlacementKeyFixture(random, index)
		if got, want := placementKey(placement), legacyPlacementKeyForTest(placement); got != want {
			t.Fatalf("case=%d placement=%+v: got %q want %q", index, placement, got, want)
		}
	}
}

func TestPlacementKeyPreservesLegacyOrdering(t *testing.T) {
	type identifiedPlacement struct {
		id        int
		placement model.Placement
	}

	random := rand.New(rand.NewSource(0x0AD3))
	legacyOrder := make([]identifiedPlacement, 2_048)
	for index := range legacyOrder {
		legacyOrder[index] = identifiedPlacement{id: index, placement: generatedPlacementKeyFixture(random, index)}
	}
	candidateOrder := append([]identifiedPlacement(nil), legacyOrder...)
	sort.SliceStable(legacyOrder, func(left, right int) bool {
		return legacyPlacementKeyForTest(legacyOrder[left].placement) < legacyPlacementKeyForTest(legacyOrder[right].placement)
	})
	sort.SliceStable(candidateOrder, func(left, right int) bool {
		return placementKey(candidateOrder[left].placement) < placementKey(candidateOrder[right].placement)
	})
	for index := range legacyOrder {
		if candidateOrder[index].id != legacyOrder[index].id {
			t.Fatalf("position=%d: candidate id=%d legacy id=%d", index, candidateOrder[index].id, legacyOrder[index].id)
		}
	}
}

func TestCoveragePlacementKeyMatchesLegacyComposition(t *testing.T) {
	random := rand.New(rand.NewSource(0xC0A3))
	for index := 0; index < 1_000; index++ {
		placement := generatedPlacementKeyFixture(random, index)
		got := coveragePlacementKey(placement)
		want := placement.InstanceID + "\x00" + legacyPlacementKeyForTest(placement)
		if got != want {
			t.Fatalf("case=%d: got %q want %q", index, got, want)
		}
	}
}

func legacyPlacementKeyForTest(placement model.Placement) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%03d|%03d|%03d|", placement.Rotation, placement.Origin.Row, placement.Origin.Col)
	for _, cell := range placement.Cells {
		fmt.Fprintf(&builder, "%03d,%03d;", cell.Row, cell.Col)
	}
	return builder.String()
}

func generatedPlacementKeyFixture(random *rand.Rand, index int) model.Placement {
	cells := make([]model.Coord, random.Intn(9))
	for cellIndex := range cells {
		cells[cellIndex] = model.Coord{Row: generatedPlacementKeyInt(random), Col: generatedPlacementKeyInt(random)}
	}
	return model.Placement{
		InstanceID: fmt.Sprintf("fixture-%05d", index),
		Rotation:   generatedPlacementKeyInt(random),
		Origin:     model.Coord{Row: generatedPlacementKeyInt(random), Col: generatedPlacementKeyInt(random)},
		Cells:      cells,
	}
}

func generatedPlacementKeyInt(random *rand.Rand) int {
	if random.Intn(10) == 0 {
		return random.Intn(4_001) - 2_000
	}
	return random.Intn(1_000)
}

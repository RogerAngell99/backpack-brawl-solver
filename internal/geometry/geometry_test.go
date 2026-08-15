package geometry

import (
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestParseGridRequiresSixByNine(t *testing.T) {
	grid := strings.Repeat("111111111/", 5) + "111111111"
	mask, err := ParseGridText(grid)
	if err != nil {
		t.Fatalf("ParseGridText returned error: %v", err)
	}
	if mask != FullGridMask() {
		t.Fatalf("mask mismatch: got %d want %d", mask, FullGridMask())
	}

	if _, err := ParseGridText("111111111/111111111"); err == nil {
		t.Fatal("expected invalid grid to fail")
	}
}

func TestRotationNormalizesShapeAndStars(t *testing.T) {
	item := model.Item{
		ID:    "vertical",
		Name:  "Vertical",
		Shape: []model.Coord{{Row: 0, Col: 0}, {Row: 1, Col: 0}},
		Stars: []model.Star{{Offset: model.Coord{Row: 0, Col: 1}}},
	}

	variants, err := VariantsForItem(item)
	if err != nil {
		t.Fatalf("VariantsForItem returned error: %v", err)
	}

	byRotation := map[int]model.Variant{}
	for _, variant := range variants {
		byRotation[variant.Rotation] = variant
	}

	assertCells(t, byRotation[0].Cells, []model.Coord{{Row: 0, Col: 0}, {Row: 1, Col: 0}})
	assertCells(t, byRotation[90].Cells, []model.Coord{{Row: 0, Col: 0}, {Row: 0, Col: 1}})
	if byRotation[90].Stars[0].Offset != (model.Coord{Row: 1, Col: 1}) {
		t.Fatalf("rotated star = %v, want (1, 1)", byRotation[90].Stars[0].Offset)
	}
}

func assertCells(t *testing.T, got []model.Coord, want []model.Coord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d", len(got), len(want))
	}
	for idx := range got {
		if got[idx] != want[idx] {
			t.Fatalf("got[%d]=%v want %v", idx, got[idx], want[idx])
		}
	}
}

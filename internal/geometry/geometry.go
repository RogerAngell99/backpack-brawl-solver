package geometry

import (
	"fmt"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/model"
)

const (
	GridRows  = 6
	GridCols  = 9
	GridCells = GridRows * GridCols
)

func CellIndex(coord model.Coord) int {
	return coord.Row*GridCols + coord.Col
}

func InBounds(coord model.Coord) bool {
	return coord.Row >= 0 && coord.Row < GridRows && coord.Col >= 0 && coord.Col < GridCols
}

func MaskFromCells(cells []model.Coord) uint64 {
	var mask uint64
	for _, coord := range cells {
		mask |= 1 << CellIndex(coord)
	}
	return mask
}

func AdjacentMaskFromCells(cells []model.Coord) uint64 {
	ownMask := MaskFromCells(cells)
	var adjacent uint64
	for _, coord := range cells {
		neighbors := [...]model.Coord{
			{Row: coord.Row - 1, Col: coord.Col},
			{Row: coord.Row + 1, Col: coord.Col},
			{Row: coord.Row, Col: coord.Col - 1},
			{Row: coord.Row, Col: coord.Col + 1},
		}
		for _, neighbor := range neighbors {
			if InBounds(neighbor) {
				adjacent |= 1 << CellIndex(neighbor)
			}
		}
	}
	return adjacent &^ ownMask
}

func FullGridMask() uint64 {
	return (uint64(1) << GridCells) - 1
}

func ParseGridText(text string) (uint64, error) {
	normalized := strings.ReplaceAll(text, "/", "\n")
	normalized = strings.ReplaceAll(normalized, ";", "\n")

	var rows []string
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			rows = append(rows, line)
		}
	}
	if len(rows) != GridRows {
		return 0, fmt.Errorf("grid must have %d rows, got %d", GridRows, len(rows))
	}

	var cells []model.Coord
	for row, line := range rows {
		var values []string
		if strings.Contains(line, " ") {
			for _, part := range strings.Split(line, " ") {
				part = strings.TrimSpace(part)
				if part != "" {
					values = append(values, part)
				}
			}
		} else {
			for _, r := range line {
				values = append(values, string(r))
			}
		}

		if len(values) != GridCols {
			return 0, fmt.Errorf("grid row %d must have %d columns, got %d", row, GridCols, len(values))
		}
		for col, value := range values {
			switch value {
			case "0":
			case "1":
				cells = append(cells, model.Coord{Row: row, Col: col})
			default:
				return 0, fmt.Errorf("grid values must be 0 or 1, got %q at row %d, col %d", value, row, col)
			}
		}
	}

	return MaskFromCells(cells), nil
}

func RotateCoord(coord model.Coord, rotation int) (model.Coord, error) {
	switch ((rotation % 360) + 360) % 360 {
	case 0:
		return coord, nil
	case 90:
		return model.Coord{Row: coord.Col, Col: -coord.Row}, nil
	case 180:
		return model.Coord{Row: -coord.Row, Col: -coord.Col}, nil
	case 270:
		return model.Coord{Row: -coord.Col, Col: coord.Row}, nil
	default:
		return model.Coord{}, fmt.Errorf("rotation must be one of 0, 90, 180, 270; got %d", rotation)
	}
}

func NormalizeVariant(shape []model.Coord, stars []model.Star, rotation int) (model.Variant, error) {
	rotatedShape := make([]model.Coord, 0, len(shape))
	for _, coord := range shape {
		rotated, err := RotateCoord(coord, rotation)
		if err != nil {
			return model.Variant{}, err
		}
		rotatedShape = append(rotatedShape, rotated)
	}

	minRow := rotatedShape[0].Row
	minCol := rotatedShape[0].Col
	for _, coord := range rotatedShape {
		if coord.Row < minRow {
			minRow = coord.Row
		}
		if coord.Col < minCol {
			minCol = coord.Col
		}
	}

	cells := make([]model.Coord, 0, len(rotatedShape))
	for _, coord := range rotatedShape {
		cells = append(cells, model.Coord{Row: coord.Row - minRow, Col: coord.Col - minCol})
	}
	sortCoords(cells)

	rotatedStars := make([]model.Star, 0, len(stars))
	for _, star := range stars {
		rotated, err := RotateCoord(star.Offset, rotation)
		if err != nil {
			return model.Variant{}, err
		}
		copied := star
		copied.Offset = model.Coord{Row: rotated.Row - minRow, Col: rotated.Col - minCol}
		rotatedStars = append(rotatedStars, copied)
	}

	return model.Variant{Rotation: rotation, Cells: cells, Stars: rotatedStars}, nil
}

func VariantsForItem(item model.Item) ([]model.Variant, error) {
	rotations := item.Rotations
	if len(rotations) == 0 {
		rotations = []int{0, 90, 180, 270}
	}

	seen := map[string]bool{}
	var variants []model.Variant
	for _, rotation := range rotations {
		variant, err := NormalizeVariant(item.Shape, item.Stars, rotation)
		if err != nil {
			return nil, err
		}
		key := variantKey(variant)
		if seen[key] {
			continue
		}
		seen[key] = true
		variants = append(variants, variant)
	}
	return variants, nil
}

func TranslateCells(cells []model.Coord, origin model.Coord) []model.Coord {
	result := make([]model.Coord, 0, len(cells))
	for _, cell := range cells {
		result = append(result, model.Coord{Row: origin.Row + cell.Row, Col: origin.Col + cell.Col})
	}
	return result
}

func Touches(a []model.Coord, b []model.Coord) bool {
	set := map[model.Coord]bool{}
	for _, coord := range b {
		set[coord] = true
	}
	for _, coord := range a {
		if set[model.Coord{Row: coord.Row - 1, Col: coord.Col}] ||
			set[model.Coord{Row: coord.Row + 1, Col: coord.Col}] ||
			set[model.Coord{Row: coord.Row, Col: coord.Col - 1}] ||
			set[model.Coord{Row: coord.Row, Col: coord.Col + 1}] {
			return true
		}
	}
	return false
}

func sortCoords(coords []model.Coord) {
	sort.Slice(coords, func(i, j int) bool {
		if coords[i].Row != coords[j].Row {
			return coords[i].Row < coords[j].Row
		}
		return coords[i].Col < coords[j].Col
	})
}

func variantKey(variant model.Variant) string {
	var builder strings.Builder
	for _, cell := range variant.Cells {
		fmt.Fprintf(&builder, "c%d,%d;", cell.Row, cell.Col)
	}
	for _, star := range variant.Stars {
		fmt.Fprintf(&builder, "s%d,%d;", star.Offset.Row, star.Offset.Col)
	}
	return builder.String()
}

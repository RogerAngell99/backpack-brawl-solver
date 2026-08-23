package solver

import (
	"fmt"
	"math/rand"
	"testing"

	"backpack-brawl-solver/internal/model"
)

// placementRespectsCanonicalCopyOrderBaselineForTest preserves the eager
// implementation as a differential oracle. Keep its lexical placementKey
// comparison intact: H2a changes only when the candidate key is built.
func placementRespectsCanonicalCopyOrderBaselineForTest(placement model.Placement, existing []model.Placement) bool {
	key := placementKey(placement)
	for _, other := range existing {
		if other.ItemID != placement.ItemID || other.InstanceID == placement.InstanceID {
			continue
		}
		otherKey := placementKey(other)
		if other.OriginalIndex < placement.OriginalIndex && otherKey > key {
			return false
		}
		if other.OriginalIndex > placement.OriginalIndex && otherKey < key {
			return false
		}
	}
	return true
}

func TestPlacementRespectsCanonicalCopyOrderMatchesEagerBaseline(t *testing.T) {
	copyAt := func(index, rotation, column int) model.Placement {
		return canonicalOrderTestPlacement(fmt.Sprintf("copy#%d", index), "copy", index, rotation, 0, column)
	}
	other := canonicalOrderTestPlacement("other#0", "other", 0, 0, 0, 0)
	adversarial := []model.Placement{
		copyAt(0, -90, 0),
		copyAt(1, 0, 1),
		copyAt(2, 90, 2),
		copyAt(3, 1080, 3),
		copyAt(4, 180, 4),
		copyAt(5, 270, 5),
		copyAt(6, 360, 6),
		copyAt(7, 450, 7),
	}
	tests := []struct {
		name      string
		placement model.Placement
		existing  []model.Placement
		want      bool
	}{
		{
			name:      "zero matching copies",
			placement: canonicalOrderTestPlacement("unique#0", "unique", 0, 0, 0, 0),
			want:      true,
		},
		{
			name:      "same instance is ignored",
			placement: copyAt(0, 0, 0),
			existing:  []model.Placement{copyAt(0, 90, 1)},
			want:      true,
		},
		{
			name:      "one matching copy accepted",
			placement: copyAt(1, 90, 1),
			existing:  []model.Placement{other, copyAt(0, 0, 0)},
			want:      true,
		},
		{
			name:      "one matching copy rejected",
			placement: copyAt(1, 0, 0),
			existing:  []model.Placement{other, copyAt(0, 90, 1)},
			want:      false,
		},
		{
			name:      "two matching copies accepted",
			placement: copyAt(1, 90, 1),
			existing:  []model.Placement{copyAt(0, 0, 0), other, copyAt(2, 180, 2)},
			want:      true,
		},
		{
			name:      "lower original index rejected by a higher key",
			placement: copyAt(0, 180, 2),
			existing:  []model.Placement{copyAt(1, 90, 1)},
			want:      false,
		},
		{
			name:      "four matching copies accepted with interleaved items",
			placement: copyAt(2, 180, 2),
			existing: []model.Placement{
				copyAt(0, 0, 0), other, copyAt(1, 90, 1),
				canonicalOrderTestPlacement("other#1", "other", 1, 270, 0, 7),
				copyAt(3, 270, 3), copyAt(4, 360, 4),
			},
			want: true,
		},
		{
			name:      "equal keys are accepted",
			placement: copyAt(1, 0, 0),
			existing:  []model.Placement{copyAt(0, 0, 0)},
			want:      true,
		},
		{
			name:      "lexical order retains adversarial rotations",
			placement: adversarial[4],
			existing:  append(append([]model.Placement{}, adversarial[:4]...), append([]model.Placement{other}, adversarial[5:]...)...),
			want:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := placementRespectsCanonicalCopyOrderBaselineForTest(test.placement, test.existing)
			got := placementRespectsCanonicalCopyOrder(test.placement, test.existing)
			if baseline != test.want {
				t.Fatalf("baseline=%t, want %t", baseline, test.want)
			}
			if got != baseline {
				t.Fatalf("lazy canonical order=%t, eager baseline=%t", got, baseline)
			}
		})
	}
}

func TestPlacementRespectsCanonicalCopyOrderMatchesEagerBaselineRandomized(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	rotations := []int{-90, 0, 90, 180, 270, 360, 450, 1080}
	for iteration := 0; iteration < 10_000; iteration++ {
		copyCount := 2 + rng.Intn(3)
		candidateIndex := rng.Intn(copyCount)
		copies := make([]model.Placement, copyCount)
		for index := range copies {
			copies[index] = canonicalOrderTestPlacement(
				fmt.Sprintf("copy#%d", index),
				"copy",
				index,
				rotations[rng.Intn(len(rotations))],
				rng.Intn(9),
				rng.Intn(9),
			)
		}
		candidate := copies[candidateIndex]
		existing := make([]model.Placement, 0, copyCount+5)
		for index, placement := range copies {
			if index != candidateIndex {
				existing = append(existing, placement)
			}
		}
		for index := 0; index < rng.Intn(5); index++ {
			existing = append(existing, canonicalOrderTestPlacement(
				fmt.Sprintf("other-%d#%d", iteration, index),
				fmt.Sprintf("other-%d", index),
				index,
				rotations[rng.Intn(len(rotations))],
				rng.Intn(9),
				rng.Intn(9),
			))
		}
		if rng.Intn(4) == 0 {
			existing = append(existing, candidate)
		}
		rng.Shuffle(len(existing), func(i, j int) { existing[i], existing[j] = existing[j], existing[i] })

		baseline := placementRespectsCanonicalCopyOrderBaselineForTest(candidate, existing)
		got := placementRespectsCanonicalCopyOrder(candidate, existing)
		if got != baseline {
			t.Fatalf("iteration %d: lazy=%t eager=%t candidate=%+v existing=%+v", iteration, got, baseline, candidate, existing)
		}
	}
}

func canonicalOrderTestPlacement(instanceID, itemID string, originalIndex, rotation, row, column int) model.Placement {
	origin := model.Coord{Row: row, Col: column}
	return model.Placement{
		InstanceID:    instanceID,
		ItemID:        itemID,
		OriginalIndex: originalIndex,
		Rotation:      rotation,
		Origin:        origin,
		Cells:         []model.Coord{origin},
	}
}

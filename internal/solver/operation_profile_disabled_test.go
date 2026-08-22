//go:build !searchprofile

package solver

import (
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestOperationProfilingRequiresSearchprofileBuildTag(t *testing.T) {
	if OperationProfilingAvailable() {
		t.Fatal("normal build unexpectedly exposes operation profiling")
	}
	catalog := model.Catalog{Items: map[string]model.Item{"a": {ID: "a", Shape: []model.Coord{{}}}}}
	_, err := SolveLayout(catalog, []string{"a"}, 1, Config{OperationProfiling: true})
	if err == nil || !strings.Contains(err.Error(), "requires a binary built with -tags searchprofile") {
		t.Fatalf("operation profiling error=%v", err)
	}
}

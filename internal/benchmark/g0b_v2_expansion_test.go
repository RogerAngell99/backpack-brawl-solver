package benchmark

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestG0BV2CoreIsMechanicallyExtracted(t *testing.T) {
	manifest, err := LoadSearchSuiteManifest(filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ExtractG0BV2Core(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(core.Cases) != 14 {
		t.Fatalf("core size = %d, want 14", len(core.Cases))
	}
	for index, entry := range core.Cases {
		want := "gsv2-" + strconv.Itoa(13+index)
		if len(want) == len("gsv2-13") {
			want = "gsv2-0" + strconv.Itoa(13+index)
		}
		if entry.CaseID != want {
			t.Fatalf("core[%d].case_id = %q, want %q", index, entry.CaseID, want)
		}
	}
}

func TestG0BV2OrchestratorImportBoundary(t *testing.T) {
	path := filepath.Join("..", "..", "cmd", "g0b-v2-expansion", "main.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"backpack-brawl-solver/internal/benchmark": true,
		"bytes": true, "crypto/sha256": true, "encoding/csv": true, "encoding/hex": true,
		"encoding/json": true, "flag": true, "fmt": true, "io": true, "os": true,
		"os/exec": true, "path/filepath": true, "sort": true, "strings": true,
	}
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed[path] {
			t.Fatalf("G0-B orchestrator imports non-allowlisted package %q", path)
		}
	}
	forbidden := []string{"internal/solver", "SolveLayout", "solver.Config", "CompareScores", "SearchStats", "benchmark-scenarios"}
	for _, sourcePath := range []string{path, "g0b_v2_expansion.go"} {
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(content), needle) {
				t.Fatalf("G0-B orchestration source %q contains forbidden identifier %q", sourcePath, needle)
			}
		}
	}
	if ast.FileExports(parsed) {
		t.Fatal("command package unexpectedly exports declarations")
	}
}

func TestG0BV2IndependentStructuralAuditSynthetic(t *testing.T) {
	schema := DevelopmentCohortSchema{Version: 1, Dimensions: []DevelopmentCohortDimension{
		{Name: "x", Values: []string{"a", "b"}}, {Name: "y", Values: []string{"c", "d"}},
	}}
	summary := independentlySummarizeG0BV2Population(schema, []DevelopmentCohortDescriptor{
		{Values: []string{"a", "c"}}, {Values: []string{"b", "d"}},
	})
	if !summary.CategoriesComplete || !summary.MarginalsBalanced || summary.PairwiseCoverage != 2 {
		t.Fatalf("unexpected synthetic audit: %+v", summary)
	}
}

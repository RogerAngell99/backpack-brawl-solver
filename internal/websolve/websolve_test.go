package websolve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/scenario"
)

func TestSolveScenarioJSON(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := readProjectFile(t, "scenarios", "spinegrowth-basic.json")

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			Crafts int `json:"crafts"`
			Stars  int `json:"stars"`
			Items  int `json:"items"`
		} `json:"score"`
		Placements []struct {
			ItemID        string  `json:"item_id"`
			StarPositions [][]int `json:"star_positions"`
		} `json:"placements"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if solutions[0].Score.Crafts != 1 || solutions[0].Score.Stars != 2 || solutions[0].Score.Items != 3 {
		t.Fatalf("unexpected score: %+v", solutions[0].Score)
	}
	var foundWithStars bool
	var foundWithoutStars bool
	for _, placement := range solutions[0].Placements {
		if placement.StarPositions == nil {
			t.Fatalf("star_positions is nil for placement %+v", placement)
		}
		if len(placement.StarPositions) > 0 {
			foundWithStars = true
		}
		if len(placement.StarPositions) == 0 {
			foundWithoutStars = true
		}
	}
	if !foundWithStars || !foundWithoutStars {
		t.Fatalf("expected placements with and without star positions: %+v", solutions[0].Placements)
	}
}

func TestSolvePreparedCatalog(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	loadedCatalog, err := catalog.Parse(catalogContent)
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	var loadedScenario scenario.Scenario
	if err := json.Unmarshal([]byte(`{
		"items": {"scalemail": 1},
		"top": 1,
		"workers": 3,
		"max_nodes": 0
	}`), &loadedScenario); err != nil {
		t.Fatalf("parse scenario: %v", err)
	}

	result, err := SolvePreparedCatalog(loadedCatalog, loadedScenario, Options{
		WorkerOverride: 1,
		Backend:        "wasm",
	})
	if err != nil {
		t.Fatalf("SolvePreparedCatalog returned error: %v", err)
	}
	if result.Metadata.Workers != 1 || result.Metadata.Backend != "wasm" {
		t.Fatalf("unexpected metadata: %+v", result.Metadata)
	}
	var solutions []any
	if err := json.Unmarshal(result.JSON, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(result.JSON))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
}

func TestSolveScenarioJSONSteelForgeHammerCoversRockAndMiningPick(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"grid": [
			"011110000",
			"011110000",
			"111111100",
			"111111100",
			"001100000",
			"001100000"
		],
		"items": {
			"mining_pick": 1,
			"rock": 1,
			"steel_forge_hammer": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0,
		"no_skips": true,
		"priorities": ["star_source:steel_forge_hammer"]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			Stars          int   `json:"stars"`
			Items          int   `json:"items"`
			PriorityCounts []int `json:"priority_counts"`
		} `json:"score"`
		Stars []struct {
			SourceInstance string `json:"source_instance"`
			TargetInstance string `json:"target_instance"`
			StarPosition   []int  `json:"star_position"`
		} `json:"stars"`
		Placements []struct {
			InstanceID    string  `json:"instance_id"`
			StarPositions [][]int `json:"star_positions"`
		} `json:"placements"`
		Coverage struct {
			Buckets []struct {
				CoveredSources int `json:"covered_sources"`
				TargetCount    int `json:"target_count"`
			} `json:"buckets"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	best := solutions[0]
	if best.Score.Stars != 2 || best.Score.Items != 3 {
		t.Fatalf("unexpected score: %+v", best.Score)
	}
	if len(best.Score.PriorityCounts) != 1 || best.Score.PriorityCounts[0] != 2 {
		t.Fatalf("priority_counts=%v want [2]", best.Score.PriorityCounts)
	}
	if len(best.Stars) != 2 {
		t.Fatalf("stars=%+v want two activations", best.Stars)
	}
	targets := map[string]bool{}
	positions := map[string]bool{}
	for _, star := range best.Stars {
		if star.SourceInstance != "steel_forge_hammer#2" {
			t.Fatalf("unexpected star source: %+v", star)
		}
		targets[star.TargetInstance] = true
		positions[fmt.Sprintf("%d,%d", star.StarPosition[0], star.StarPosition[1])] = true
	}
	if !targets["mining_pick#0"] || !targets["rock#1"] || len(targets) != 2 {
		t.Fatalf("star targets=%v want mining_pick#0 and rock#1", targets)
	}
	if len(positions) != 2 {
		t.Fatalf("star positions=%v want two distinct positions", positions)
	}
	var sourceStarPositions [][]int
	for _, placement := range best.Placements {
		if placement.InstanceID == "steel_forge_hammer#2" {
			sourceStarPositions = placement.StarPositions
			break
		}
	}
	if len(sourceStarPositions) != 2 {
		t.Fatalf("hammer star positions=%v want two positions", sourceStarPositions)
	}
	for _, star := range best.Stars {
		if !containsCoord(sourceStarPositions, star.StarPosition) {
			t.Fatalf("star position %v is not a hammer star: %v", star.StarPosition, sourceStarPositions)
		}
	}
	if len(best.Coverage.Buckets) != 1 || best.Coverage.Buckets[0].CoveredSources != 1 || best.Coverage.Buckets[0].TargetCount != 2 {
		t.Fatalf("coverage buckets=%+v want one source covering two targets", best.Coverage.Buckets)
	}
}

func containsCoord(coords [][]int, wanted []int) bool {
	for _, coord := range coords {
		if len(coord) == len(wanted) && len(coord) == 2 && coord[0] == wanted[0] && coord[1] == wanted[1] {
			return true
		}
	}
	return false
}

func TestSolveScenarioJSONWithRemoteOptionsAppliesCapsAndMetadata(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"scalemail": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	result, err := SolveScenarioJSONWithOptions(input, Options{
		WorkerOverride:  2,
		DefaultMaxNodes: 1234,
		MaxNodesCap:     5000,
		Backend:         "vercel-go",
	})
	if err != nil {
		t.Fatalf("SolveScenarioJSONWithOptions returned error: %v", err)
	}
	if result.Metadata.Workers != 2 {
		t.Fatalf("expected worker override, got %+v", result.Metadata)
	}
	if result.Metadata.MaxNodesApplied != 1234 || !result.Metadata.MaxNodesCapped {
		t.Fatalf("expected remote default max nodes metadata, got %+v", result.Metadata)
	}

	var solutions []struct {
		Search struct {
			Backend         string  `json:"backend"`
			ServerElapsedMS int64   `json:"server_elapsed_ms"`
			RemoteWorkers   int     `json:"remote_workers"`
			MaxNodesApplied int64   `json:"max_nodes_applied"`
			MaxNodesCapped  bool    `json:"max_nodes_capped"`
			NodesPerSecond  float64 `json:"nodes_per_second"`
		} `json:"search"`
	}
	if err := json.Unmarshal(result.JSON, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(result.JSON))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	search := solutions[0].Search
	if search.Backend != "vercel-go" || search.RemoteWorkers != 2 || search.MaxNodesApplied != 1234 || !search.MaxNodesCapped {
		t.Fatalf("unexpected remote search metadata: %+v", search)
	}
	if search.ServerElapsedMS < 0 {
		t.Fatalf("unexpected server elapsed metadata: %+v", search)
	}
}

func TestSolveScenarioJSONIncludesPriorityCounts(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"scalemail": 1,
			"thornwall": 1,
			"armor_kit": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0,
		"priorities": ["craft:spinegrowth_breastplate"]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			PriorityCounts []int `json:"priority_counts"`
		} `json:"score"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if len(solutions[0].Score.PriorityCounts) != 1 || solutions[0].Score.PriorityCounts[0] != 1 {
		t.Fatalf("priority_counts=%v want [1]", solutions[0].Score.PriorityCounts)
	}
}

func TestSolveScenarioJSONIncludesCoverageBreakdown(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"shaman_s_talisman": 1,
			"royal_seax": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0,
		"no_skips": true,
		"priorities": ["star_source:shaman_s_talisman"]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			PriorityCounts []int `json:"priority_counts"`
		} `json:"score"`
		Search struct {
			CoverageSeedNodes      int64  `json:"coverage_seed_nodes"`
			CoverageSeedCandidates int    `json:"coverage_seed_candidates"`
			CoverageSeedBest       string `json:"coverage_seed_best"`
		} `json:"search"`
		Coverage struct {
			Sources []string `json:"sources"`
			Buckets []struct {
				CoveredSources int `json:"covered_sources"`
				TargetCount    int `json:"target_count"`
			} `json:"buckets"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if len(solutions[0].Coverage.Sources) != 1 || solutions[0].Coverage.Sources[0] != "shaman_s_talisman" {
		t.Fatalf("unexpected coverage sources: %+v", solutions[0].Coverage.Sources)
	}
	if len(solutions[0].Coverage.Buckets) != 1 || solutions[0].Coverage.Buckets[0].TargetCount != 1 {
		t.Fatalf("unexpected coverage buckets: %+v", solutions[0].Coverage.Buckets)
	}
	if len(solutions[0].Score.PriorityCounts) != 1 || solutions[0].Score.PriorityCounts[0] != 1 {
		t.Fatalf("priority_counts=%v want [1]", solutions[0].Score.PriorityCounts)
	}
	if solutions[0].Search.CoverageSeedNodes == 0 || solutions[0].Search.CoverageSeedCandidates == 0 || solutions[0].Search.CoverageSeedBest == "" {
		t.Fatalf("missing coverage seed metadata: %+v", solutions[0].Search)
	}
}

func TestSolveScenarioJSONCoverageEmptySourcesAreArrays(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"doombringer": 1,
			"ragnarok": 1,
			"royal_seax": 1,
			"rune_of_r_lyeh": 1,
			"shaman_s_talisman": 1,
			"spinegrowth_breastplate": 1,
			"venomous_pincer": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 1000000,
		"priorities": [
			"star_source:rune_of_r_lyeh",
			"star_source:shaman_s_talisman",
			"star_source:spinegrowth_breastplate",
			"star_source:venomous_pincer"
		]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}
	if strings.Contains(string(output), `"covered_sources": null`) {
		t.Fatalf("covered_sources serialized as null:\n%s", string(output))
	}

	var solutions []struct {
		Coverage struct {
			Targets []struct {
				CoveredSources []string `json:"covered_sources"`
				CoveredCount   int      `json:"covered_count"`
			} `json:"targets"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	foundZeroCoverageTarget := false
	for _, target := range solutions[0].Coverage.Targets {
		if target.CoveredCount == 0 {
			foundZeroCoverageTarget = true
			if target.CoveredSources == nil {
				t.Fatal("zero coverage target has nil covered_sources")
			}
		}
	}
	if !foundZeroCoverageTarget {
		t.Fatal("expected at least one zero coverage target")
	}
}

func TestSolveScenarioJSONIncludesCoverageGroups(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"fine_sword": 1,
			"piercing_lance": 1,
			"excalibur": 1,
			"gloves_of_power": 1,
			"power_stone": 1,
			"spice": 1,
			"pitahaya": 1,
			"apple": 1,
			"banana": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 100000,
		"coverage_groups": [
			{"name": "Weapons", "sources": ["power_stone", "piercing_lance"], "targets": ["excalibur", "fine_sword"]},
			{"name": "Food", "sources": ["spice"]}
		]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			PriorityCounts []int `json:"priority_counts"`
		} `json:"score"`
		CoverageGroups []struct {
			Name          string   `json:"name"`
			Sources       []string `json:"sources"`
			TargetItemIDs []string `json:"target_item_ids"`
			Targets       []struct {
				TargetItemID string `json:"target_item_id"`
			} `json:"targets"`
		} `json:"coverage_groups"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if len(solutions[0].CoverageGroups) != 2 {
		t.Fatalf("coverage group count=%d want 2: %s", len(solutions[0].CoverageGroups), string(output))
	}
	if solutions[0].CoverageGroups[0].Name != "Weapons" || solutions[0].CoverageGroups[1].Name != "Food" {
		t.Fatalf("unexpected coverage groups: %+v", solutions[0].CoverageGroups)
	}
	if len(solutions[0].Score.PriorityCounts) != 3 {
		t.Fatalf("priority_counts=%v want 3 buckets", solutions[0].Score.PriorityCounts)
	}
	if len(solutions[0].CoverageGroups[0].TargetItemIDs) != 2 ||
		solutions[0].CoverageGroups[0].TargetItemIDs[0] != "excalibur" ||
		solutions[0].CoverageGroups[0].TargetItemIDs[1] != "fine_sword" {
		t.Fatalf("unexpected explicit target item ids: %+v", solutions[0].CoverageGroups[0].TargetItemIDs)
	}
	if groupContainsTarget(solutions[0].CoverageGroups[0].Targets, "apple") {
		t.Fatalf("weapon group should not include apple target: %+v", solutions[0].CoverageGroups[0].Targets)
	}
	if groupContainsTarget(solutions[0].CoverageGroups[0].Targets, "piercing_lance") {
		t.Fatalf("weapon group should not include unlisted piercing_lance target: %+v", solutions[0].CoverageGroups[0].Targets)
	}
	if groupContainsTarget(solutions[0].CoverageGroups[1].Targets, "fine_sword") {
		t.Fatalf("food group should not include fine_sword target: %+v", solutions[0].CoverageGroups[1].Targets)
	}
}

func TestSolveScenarioJSONIncludesLooseStarPriorities(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")
	scenarioContent := json.RawMessage(`{
		"items": {
			"fine_sword": 1,
			"power_stone": 1,
			"spinegrowth_breastplate": 1
		},
		"top": 1,
		"workers": 1,
		"max_nodes": 0,
		"no_skips": true,
		"coverage_groups": [
			{"name": "Weapons", "sources": ["power_stone"]}
		],
		"priorities": ["star_source:spinegrowth_breastplate"]
	}`)

	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": scenarioContent,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	output, err := SolveScenarioJSON(input)
	if err != nil {
		t.Fatalf("SolveScenarioJSON returned error: %v", err)
	}

	var solutions []struct {
		Score struct {
			PriorityCounts []int `json:"priority_counts"`
		} `json:"score"`
		LooseStarPriorities []struct {
			SourceItemID string `json:"source_item_id"`
			TargetCount  int    `json:"target_count"`
		} `json:"loose_star_priorities"`
	}
	if err := json.Unmarshal(output, &solutions); err != nil {
		t.Fatalf("invalid output JSON: %v\n%s", err, string(output))
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if len(solutions[0].Score.PriorityCounts) != 2 {
		t.Fatalf("priority_counts=%v want coverage bucket plus loose star count", solutions[0].Score.PriorityCounts)
	}
	if len(solutions[0].LooseStarPriorities) != 1 {
		t.Fatalf("loose_star_priorities=%+v want one entry", solutions[0].LooseStarPriorities)
	}
	if solutions[0].LooseStarPriorities[0].SourceItemID != "spinegrowth_breastplate" {
		t.Fatalf("loose_star_priorities=%+v want spinegrowth_breastplate", solutions[0].LooseStarPriorities)
	}
}

func TestSolveScenarioJSONRejectsEmptyInventory(t *testing.T) {
	catalogContent := readProjectFile(t, "data", "catalog.json")

	input, err := json.Marshal(map[string]any{
		"catalog": json.RawMessage(catalogContent),
		"scenario": map[string]any{
			"items": map[string]int{},
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	_, err = SolveScenarioJSON(input)

	if err == nil {
		t.Fatal("SolveScenarioJSON returned nil error")
	}
}

func BenchmarkCatalogParseProduction(b *testing.B) {
	catalogContent := readProjectFile(b, "data", "catalog.json")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := catalog.Parse(catalogContent); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWebSolveEnvelope(b *testing.B) {
	input := benchmarkSolveInput(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := SolveScenarioJSONWithOptions(input, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWebSolvePreparedCatalog(b *testing.B) {
	catalogContent := readProjectFile(b, "data", "catalog.json")
	loadedCatalog, err := catalog.Parse(catalogContent)
	if err != nil {
		b.Fatal(err)
	}
	var loadedScenario scenario.Scenario
	if err := json.Unmarshal(benchmarkScenarioJSON, &loadedScenario); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := SolvePreparedCatalog(loadedCatalog, loadedScenario, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

var benchmarkScenarioJSON = []byte(`{
	"items": {"scalemail": 1},
	"top": 1,
	"max_nodes": 1,
	"repair_search": false
}`)

func benchmarkSolveInput(b testing.TB) []byte {
	b.Helper()
	catalogContent := readProjectFile(b, "data", "catalog.json")
	input, err := json.Marshal(map[string]json.RawMessage{
		"catalog":  catalogContent,
		"scenario": benchmarkScenarioJSON,
	})
	if err != nil {
		b.Fatal(err)
	}
	return input
}

func groupContainsTarget(targets []struct {
	TargetItemID string `json:"target_item_id"`
}, itemID string) bool {
	for _, target := range targets {
		if target.TargetItemID == itemID {
			return true
		}
	}
	return false
}

func readProjectFile(t testing.TB, parts ...string) []byte {
	t.Helper()
	pathParts := append([]string{"..", ".."}, parts...)
	content, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	return content
}

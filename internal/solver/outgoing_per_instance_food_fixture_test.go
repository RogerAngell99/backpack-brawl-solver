package solver

import (
	"math/bits"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const outgoingPerInstanceFoodPrintLayoutKey = "000|banana|000|002|002;001|cactrio|000|000|000;002|champion_s_ripper|000|001|006;003|cleansing_crown|090|001|000;004|death_essence|000|003|007;005|discordant_harp|270|000|004;006|donut|000|004|003;007|ginseng_root|090|004|000;008|ginseng_root|090|003|005;009|green_snapper|000|005|005;010|hooded_cowl|000|000|007;011|longing_begonia|090|000|002;012|pitahaya|090|003|000;013|pitahaya|000|004|004;014|spice|270|003|004;015|spice|000|005|001;016|spice|000|005|002;017|spicy_sausage|000|002|001;018|spiked_sickle|180|001|004;019|spirit_biscuit|000|005|003;020|steadfast_boots|180|002|007;021|tender_sausage|000|004|001;022|thornwall|000|004|007;023|twinmaw|090|001|001;"

var outgoingPerInstanceFoodPrintStars = []string{
	"banana#0|pitahaya#12|3,1", "banana#0|spicy_sausage#17|2,2", "banana#0|tender_sausage#21|4,2", "banana#0|donut#6|4,3", "banana#0|spice#14|3,4",
	"death_essence#4|steadfast_boots#20|2,8", "death_essence#4|champion_s_ripper#2|4,6", "death_essence#4|thornwall#22|4,8",
	"discordant_harp#5|twinmaw#23|1,4", "discordant_harp#5|spiked_sickle#18|1,5", "discordant_harp#5|champion_s_ripper#2|1,6",
	"donut#6|spice#16|5,2", "donut#6|pitahaya#13|5,4", "donut#6|spice#14|3,4", "donut#6|banana#0|3,2",
	"ginseng_root#7|tender_sausage#21|4,1", "ginseng_root#7|pitahaya#12|3,0", "ginseng_root#7|spice#15|5,1",
	"ginseng_root#8|spice#14|3,4", "ginseng_root#8|pitahaya#13|4,4",
	"hooded_cowl#10|champion_s_ripper#2|1,6",
	"pitahaya#12|ginseng_root#7|4,0", "pitahaya#12|banana#0|3,2", "pitahaya#12|tender_sausage#21|4,1", "pitahaya#12|spicy_sausage#17|2,1",
	"pitahaya#13|spirit_biscuit#19|5,3", "pitahaya#13|spice#14|3,4", "pitahaya#13|ginseng_root#8|4,5", "pitahaya#13|donut#6|4,3",
	"spice#14|banana#0|3,3", "spice#14|spicy_sausage#17|2,2", "spice#14|tender_sausage#21|4,2", "spice#14|pitahaya#12|3,1",
	"spice#15|tender_sausage#21|4,1", "spice#15|banana#0|3,2", "spice#15|pitahaya#12|3,0", "spice#15|spicy_sausage#17|2,1",
	"spice#16|tender_sausage#21|4,2", "spice#16|banana#0|3,3", "spice#16|pitahaya#12|3,1", "spice#16|spicy_sausage#17|2,2",
	"spicy_sausage#17|pitahaya#12|3,1", "spicy_sausage#17|banana#0|2,3",
	"spirit_biscuit#19|spicy_sausage#17|2,2", "spirit_biscuit#19|spiked_sickle#18|2,4", "spirit_biscuit#19|tender_sausage#21|4,1", "spirit_biscuit#19|ginseng_root#8|4,5", "spirit_biscuit#19|spice#16|5,2", "spirit_biscuit#19|pitahaya#13|5,4",
	"steadfast_boots#20|champion_s_ripper#2|2,6",
	"tender_sausage#21|pitahaya#12|3,1", "tender_sausage#21|spice#15|5,1", "tender_sausage#21|ginseng_root#7|4,0", "tender_sausage#21|banana#0|3,2", "tender_sausage#21|donut#6|4,3", "tender_sausage#21|spice#16|5,2",
}

func TestOutgoingPerInstanceFoodPrintFixture(t *testing.T) {
	placements := OutgoingPerInstanceFoodDiagnosticReference()
	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime catalog: %v", err)
	}
	loadedScenario, err := scenario.Load(filepath.Join("..", "..", "benchmarks", "scenarios", "outgoing-per-instance-food.json"))
	if err != nil {
		t.Fatalf("load food scenario: %v", err)
	}
	loadedCatalog, err = catalog.FilterForHeroes(loadedCatalog, loadedScenario.HeroFilter)
	if err != nil {
		t.Fatalf("filter runtime catalog: %v", err)
	}
	gridMask, err := geometry.ParseGridText(loadedScenario.GridText())
	if err != nil {
		t.Fatalf("parse scenario grid: %v", err)
	}

	solution, err := EvaluateKnownLayout(loadedCatalog, loadedScenario.ItemIDs(), gridMask, Config{
		InitialPlacements: placements,
		PrioritySemantics: loadedScenario.PrioritySemantics,
		Priorities:        loadedScenario.Priorities,
		CoverageGroups:    loadedScenario.ModelCoverageGroups(),
	})
	if err != nil {
		t.Fatalf("evaluate UI print transcription: %v", err)
	}
	if len(placements) != 24 || len(solution.Placements) != 24 {
		t.Fatalf("fixture placements=%d evaluated=%d want 24", len(placements), len(solution.Placements))
	}
	var occupied uint64
	cellCount := 0
	for _, placement := range solution.Placements {
		occupied |= placement.Mask
		cellCount += len(placement.Cells)
	}
	if cellCount != 54 || bits.OnesCount64(occupied) != 54 {
		t.Fatalf("occupied cells total=%d distinct=%d want 54", cellCount, bits.OnesCount64(occupied))
	}
	if solution.LayoutKey == "" || solution.CanonicalLayoutHash == "" {
		t.Fatalf("fixture identity layout=%q hash=%q", solution.LayoutKey, solution.CanonicalLayoutHash)
	}
	if solution.LayoutKey != outgoingPerInstanceFoodPrintLayoutKey {
		t.Fatalf("layout key=%q want %q", solution.LayoutKey, outgoingPerInstanceFoodPrintLayoutKey)
	}
	wantScore := model.Score{
		CraftCount:                    0,
		StarCount:                     56,
		ItemCount:                     24,
		StarTargetBreadth:             17,
		StarReciprocalPairs:           14,
		StarSourceDefinitionDiversity: 48,
		PriorityCounts:                []int{6, 12},
	}
	if !reflect.DeepEqual(solution.Evaluation.Score, wantScore) {
		t.Fatalf("score=%+v want %+v", solution.Evaluation.Score, wantScore)
	}
	if len(solution.Evaluation.LooseStarPriorities) != 2 {
		t.Fatalf("V3 priorities=%+v want Spirit Biscuit and Spice", solution.Evaluation.LooseStarPriorities)
	}
	for _, priority := range solution.Evaluation.LooseStarPriorities {
		switch priority.SourceItemID {
		case "spirit_biscuit":
			if priority.TargetCount != 6 || priority.LinkCount != 6 || len(priority.InstanceTargetCounts) != 1 || priority.InstanceTargetCounts[0].SourceInstance != "spirit_biscuit#19" || priority.InstanceTargetCounts[0].TargetCount != 6 {
				t.Fatalf("Spirit Biscuit V3 priority=%+v", priority)
			}
		case "spice":
			if priority.TargetCount != 4 || priority.LinkCount != 12 || len(priority.InstanceTargetCounts) != 3 {
				t.Fatalf("Spice V3 priority=%+v", priority)
			}
			for _, count := range priority.InstanceTargetCounts {
				if count.TargetCount != 4 {
					t.Fatalf("Spice copy priority=%+v want four targets", count)
				}
			}
		default:
			t.Fatalf("unexpected V3 priority=%+v", priority)
		}
	}
	if len(solution.Evaluation.Stars) != len(outgoingPerInstanceFoodPrintStars) {
		t.Fatalf("stars=%d want %d", len(solution.Evaluation.Stars), len(outgoingPerInstanceFoodPrintStars))
	}
	actualStars := make(map[string]bool, len(solution.Evaluation.Stars))
	for _, star := range solution.Evaluation.Stars {
		actualStars[outgoingPerInstanceFoodFixtureStarKey(star)] = true
	}
	for _, expected := range outgoingPerInstanceFoodPrintStars {
		if !actualStars[expected] {
			t.Fatalf("missing literal star %s", expected)
		}
	}
}

func TestOutgoingPerInstanceFoodDiagnosticReferenceReturnsDefensiveCopy(t *testing.T) {
	placements := OutgoingPerInstanceFoodDiagnosticReference()
	placements[0].InstanceID = "changed"

	if got := OutgoingPerInstanceFoodDiagnosticReference()[0].InstanceID; got != "banana#0" {
		t.Fatalf("instance ID=%q want banana#0", got)
	}
}

func TestSearchDiscoveredRank4Fixture(t *testing.T) {
	placements := searchDiscoveredRank4FixturePlacements()
	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime catalog: %v", err)
	}
	loadedScenario, err := scenario.Load(filepath.Join("..", "..", "benchmarks", "scenarios", "outgoing-per-instance-food.json"))
	if err != nil {
		t.Fatalf("load food scenario: %v", err)
	}
	loadedCatalog, err = catalog.FilterForHeroes(loadedCatalog, loadedScenario.HeroFilter)
	if err != nil {
		t.Fatalf("filter runtime catalog: %v", err)
	}
	gridMask, err := geometry.ParseGridText(loadedScenario.GridText())
	if err != nil {
		t.Fatalf("parse scenario grid: %v", err)
	}
	solution, err := EvaluateKnownLayout(loadedCatalog, loadedScenario.ItemIDs(), gridMask, Config{
		InitialPlacements: placements,
		PrioritySemantics: loadedScenario.PrioritySemantics,
		Priorities:        loadedScenario.Priorities,
		CoverageGroups:    loadedScenario.ModelCoverageGroups(),
	})
	if err != nil {
		t.Fatalf("evaluate search_discovered fixture: %v", err)
	}
	want := model.Score{
		CraftCount:                    0,
		StarCount:                     58,
		ItemCount:                     24,
		StarTargetBreadth:             19,
		StarReciprocalPairs:           19,
		StarSourceDefinitionDiversity: 49,
		PriorityCounts:                []int{6, 12},
	}
	if !reflect.DeepEqual(solution.Evaluation.Score, want) {
		t.Fatalf("search_discovered score=%+v want %+v", solution.Evaluation.Score, want)
	}
	if len(solution.Evaluation.Stars) != 58 || solution.CanonicalLayoutHash != "f5aaf4a6c45b0655c4d65fd25e7cf2fc8e080e3ee2973b1ca677f372acb08ff5" {
		t.Fatalf("search_discovered identity stars=%d hash=%s", len(solution.Evaluation.Stars), solution.CanonicalLayoutHash)
	}
}

func outgoingPerInstanceFoodFixtureStarKey(star model.StarActivation) string {
	return star.SourceInstance + "|" + star.TargetInstance + "|" + strconv.Itoa(star.StarPosition.Row) + "," + strconv.Itoa(star.StarPosition.Col)
}

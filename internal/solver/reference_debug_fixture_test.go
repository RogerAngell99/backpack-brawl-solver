package solver

import (
	"math/bits"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
)

// This is a literal transcription of the confirmed player debug export. It is
// an evaluation oracle only: no automatic solver or benchmark receives these
// placements as an incumbent.
var referenceDebugItems = []string{
	"banana", "cactrio", "champion_s_ripper", "cleansing_crown", "death_essence", "discordant_harp", "donut",
	"ginseng_root", "ginseng_root", "green_snapper", "hooded_cowl", "longing_begonia", "pitahaya", "pitahaya",
	"spice", "spice", "spice", "spicy_sausage", "spiked_sickle", "spirit_biscuit", "steadfast_boots", "tender_sausage",
	"thornwall", "twinmaw",
}

var referenceDebugPriorities = []string{
	"star_source:discordant_harp", "star_source:hooded_cowl", "star_source:steadfast_boots", "star_source:death_essence",
	"star_source:pitahaya", "star_source:ginseng_root", "star_source:spicy_sausage", "star_source:spice",
	"star_source:spirit_biscuit", "star_source:banana", "star_source:tender_sausage", "star_source:donut",
}

type referencePlacement struct {
	instanceID string
	rotation   int
	origin     model.Coord
}

var referenceDebugPlacements = []referencePlacement{
	{"banana#0", 0, model.Coord{Row: 2, Col: 2}},
	{"cactrio#1", 0, model.Coord{Row: 0, Col: 0}},
	{"champion_s_ripper#2", 0, model.Coord{Row: 1, Col: 6}},
	{"cleansing_crown#3", 90, model.Coord{Row: 1, Col: 0}},
	{"death_essence#4", 0, model.Coord{Row: 3, Col: 7}},
	{"discordant_harp#5", 270, model.Coord{Row: 0, Col: 4}},
	{"donut#6", 0, model.Coord{Row: 4, Col: 3}},
	{"ginseng_root#7", 90, model.Coord{Row: 4, Col: 0}},
	{"ginseng_root#8", 270, model.Coord{Row: 3, Col: 5}},
	{"green_snapper#9", 0, model.Coord{Row: 5, Col: 5}},
	{"hooded_cowl#10", 0, model.Coord{Row: 0, Col: 7}},
	{"longing_begonia#11", 90, model.Coord{Row: 0, Col: 2}},
	{"pitahaya#12", 90, model.Coord{Row: 3, Col: 0}},
	{"pitahaya#13", 0, model.Coord{Row: 4, Col: 4}},
	{"spice#14", 0, model.Coord{Row: 5, Col: 1}},
	{"spice#15", 0, model.Coord{Row: 5, Col: 2}},
	{"spice#16", 270, model.Coord{Row: 3, Col: 4}},
	{"spicy_sausage#17", 180, model.Coord{Row: 4, Col: 1}},
	{"spiked_sickle#18", 180, model.Coord{Row: 1, Col: 4}},
	{"spirit_biscuit#19", 0, model.Coord{Row: 5, Col: 3}},
	{"steadfast_boots#20", 180, model.Coord{Row: 2, Col: 7}},
	{"tender_sausage#21", 0, model.Coord{Row: 2, Col: 1}},
	{"thornwall#22", 0, model.Coord{Row: 4, Col: 7}},
	{"twinmaw#23", 90, model.Coord{Row: 1, Col: 1}},
}

const referenceDebugLayoutKey = "000|banana|000|002|002;001|cactrio|000|000|000;002|champion_s_ripper|000|001|006;003|cleansing_crown|090|001|000;004|death_essence|000|003|007;005|discordant_harp|270|000|004;006|donut|000|004|003;007|ginseng_root|090|004|000;008|ginseng_root|270|003|005;009|green_snapper|000|005|005;010|hooded_cowl|000|000|007;011|longing_begonia|090|000|002;012|pitahaya|090|003|000;013|pitahaya|000|004|004;014|spice|000|005|001;015|spice|000|005|002;016|spice|270|003|004;017|spicy_sausage|180|004|001;018|spiked_sickle|180|001|004;019|spirit_biscuit|000|005|003;020|steadfast_boots|180|002|007;021|tender_sausage|000|002|001;022|thornwall|000|004|007;023|twinmaw|090|001|001;"

var referenceDebugStars = []string{
	"banana#0|pitahaya#12|3,1", "banana#0|tender_sausage#21|2,2", "banana#0|spicy_sausage#17|4,2", "banana#0|donut#6|4,3", "banana#0|spice#16|3,4",
	"death_essence#4|steadfast_boots#20|2,8", "death_essence#4|champion_s_ripper#2|4,6", "death_essence#4|thornwall#22|4,8",
	"discordant_harp#5|twinmaw#23|1,4", "discordant_harp#5|spiked_sickle#18|1,5", "discordant_harp#5|champion_s_ripper#2|1,6",
	"donut#6|spice#15|5,2", "donut#6|pitahaya#13|5,4", "donut#6|spice#16|3,4", "donut#6|banana#0|3,2",
	"ginseng_root#7|spicy_sausage#17|4,1", "ginseng_root#7|pitahaya#12|3,0", "ginseng_root#7|spice#14|5,1", "ginseng_root#8|pitahaya#13|4,4", "ginseng_root#8|spice#16|3,4",
	"hooded_cowl#10|champion_s_ripper#2|1,6",
	"pitahaya#12|ginseng_root#7|4,0", "pitahaya#12|banana#0|3,2", "pitahaya#12|spicy_sausage#17|4,1", "pitahaya#12|tender_sausage#21|2,1",
	"pitahaya#13|spirit_biscuit#19|5,3", "pitahaya#13|spice#16|3,4", "pitahaya#13|ginseng_root#8|4,5", "pitahaya#13|donut#6|4,3",
	"spice#14|spicy_sausage#17|4,1", "spice#14|banana#0|3,2", "spice#14|pitahaya#12|3,0", "spice#14|tender_sausage#21|2,1",
	"spice#15|spicy_sausage#17|4,2", "spice#15|banana#0|3,3", "spice#15|pitahaya#12|3,1", "spice#15|tender_sausage#21|2,2",
	"spice#16|banana#0|3,3", "spice#16|tender_sausage#21|2,2", "spice#16|spicy_sausage#17|4,2", "spice#16|pitahaya#12|3,1",
	"spicy_sausage#17|spice#15|5,2", "spicy_sausage#17|banana#0|3,2", "spicy_sausage#17|donut#6|4,3", "spicy_sausage#17|spice#14|5,1", "spicy_sausage#17|ginseng_root#7|4,0", "spicy_sausage#17|pitahaya#12|3,1",
	"spirit_biscuit#19|tender_sausage#21|2,2", "spirit_biscuit#19|spiked_sickle#18|2,4", "spirit_biscuit#19|spicy_sausage#17|4,1", "spirit_biscuit#19|ginseng_root#8|4,5", "spirit_biscuit#19|spice#15|5,2", "spirit_biscuit#19|pitahaya#13|5,4",
	"steadfast_boots#20|champion_s_ripper#2|2,6", "tender_sausage#21|pitahaya#12|3,1", "tender_sausage#21|banana#0|2,3",
}

func TestConfirmedPlayerDebugFixture(t *testing.T) {
	cat, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime catalog: %v", err)
	}
	instances := ExpandInventory(referenceDebugItems)
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	placements := make([]model.Placement, 0, len(referenceDebugPlacements))
	for _, expected := range referenceDebugPlacements {
		placements = append(placements, testPlacement(t, optionsByInstance[expected.instanceID], expected.origin, expected.rotation))
	}
	if got := len(placements); got != 24 {
		t.Fatalf("literal placement count=%d want 24", got)
	}
	for index, expected := range referenceDebugPlacements {
		got := placements[index]
		if got.InstanceID != expected.instanceID || got.Rotation != expected.rotation || got.Origin != expected.origin {
			t.Fatalf("placement[%d]=%+v want instance=%s rotation=%d origin=%s", index, got, expected.instanceID, expected.rotation, expected.origin)
		}
	}

	solutions, err := initialSolutionsForConfig(cat, instances, optionsByInstance, Config{
		AllowSkips:        false,
		InitialPlacements: placements,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        referenceDebugPriorities,
	})
	if err != nil {
		t.Fatalf("confirmed layout is invalid: %v", err)
	}
	if len(solutions) != 1 {
		t.Fatalf("solutions=%d want 1", len(solutions))
	}
	solution := solutions[0]
	var occupied uint64
	cellCount := 0
	for _, placement := range solution.Placements {
		occupied |= placement.Mask
		cellCount += len(placement.Cells)
	}
	if cellCount != 54 || bits.OnesCount64(occupied) != 54 {
		t.Fatalf("occupied cells total=%d distinct=%d want 54", cellCount, bits.OnesCount64(occupied))
	}
	if solution.LayoutKey != referenceDebugLayoutKey {
		t.Fatalf("layout key=%q want %q", solution.LayoutKey, referenceDebugLayoutKey)
	}
	if solution.CanonicalLayoutHash == "" {
		t.Fatal("canonical hash is missing from the literal fixture evaluation")
	}
	wantScore := model.Score{CraftCount: 0, StarCount: 56, ItemCount: 24, StarTargetBreadth: 17, StarReciprocalPairs: 14, StarSourceDefinitionDiversity: 48, PriorityCounts: []int{3, 1, 1, 3, 8, 5, 6, 12, 6, 5, 2, 4}}
	if !reflect.DeepEqual(solution.Evaluation.Score, wantScore) {
		t.Fatalf("score=%+v want %+v", solution.Evaluation.Score, wantScore)
	}
	if len(solution.Evaluation.Stars) != 56 || len(referenceDebugStars) != 56 {
		t.Fatalf("stars actual=%d expected=%d want 56", len(solution.Evaluation.Stars), len(referenceDebugStars))
	}
	actualStars := map[string]bool{}
	for _, star := range solution.Evaluation.Stars {
		actualStars[referenceStarKey(star)] = true
	}
	for _, expected := range referenceDebugStars {
		if !actualStars[expected] {
			t.Fatalf("missing literal star triple %s", expected)
		}
	}
}

func referenceStarKey(star model.StarActivation) string {
	return star.SourceInstance + "|" + star.TargetInstance + "|" + strconv.Itoa(star.StarPosition.Row) + "," + strconv.Itoa(star.StarPosition.Col)
}

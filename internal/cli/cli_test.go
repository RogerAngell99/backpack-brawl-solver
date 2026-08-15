package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func catalogPath() string {
	return filepath.Join("..", "..", "data", "catalog.json")
}

func scenarioPath() string {
	return filepath.Join("..", "..", "scenarios", "spinegrowth-basic.json")
}

func TestValidateCatalogCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"validate-catalog", "--catalog", catalogPath()}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Catalog valid: 1196 items, 477 recipes") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "excalibur: recipe ingredient \"knight_s_sigil\" is not in catalog yet") {
		t.Fatalf("knight_s_sigil should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "royal_seax: recipe ingredient \"life_essence\" is not in catalog yet") {
		t.Fatalf("life_essence should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "royal_seax: recipe anchor \"dagger\" is not in catalog yet") {
		t.Fatalf("dagger should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "poison_potion: recipe ingredient \"nightshade\" is not in catalog yet") {
		t.Fatalf("nightshade should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "champion_s_ripper: recipe anchor \"hooked_blade\" is not in catalog yet") {
		t.Fatalf("hooked_blade should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "champion_s_ripper: recipe ingredient \"knight_s_razor\" is not in catalog yet") {
		t.Fatalf("knight_s_razor should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "poison_potion: recipe anchor \"water_potion\" is not in catalog yet") {
		t.Fatalf("water_potion should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "bolstered_helmet: recipe anchor \"skull_cap\" is not in catalog yet") {
		t.Fatalf("skull_cap should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "stunbreaker_shield: recipe ingredient \"skull_cap\" is not in catalog yet") {
		t.Fatalf("skull_cap should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "brainsquasher: recipe anchor \"mallet\" is not in catalog yet") {
		t.Fatalf("mallet should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "brainsquasher: recipe ingredient \"mallet\" is not in catalog yet") {
		t.Fatalf("mallet should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "fine_sword: recipe anchor \"iron_sword\" is not in catalog yet") {
		t.Fatalf("iron_sword should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "fine_sword: recipe ingredient \"whetstone\" is not in catalog yet") {
		t.Fatalf("whetstone should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "spiked_shield: recipe anchor \"wooden_buckler\" is not in catalog yet") {
		t.Fatalf("wooden_buckler should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "spiked_shield: recipe ingredient \"tusk\" is not in catalog yet") {
		t.Fatalf("tusk should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "stunbreaker_shield: recipe anchor \"wooden_buckler\" is not in catalog yet") {
		t.Fatalf("wooden_buckler should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "amethyst_blade: recipe anchor \"fine_sword\" is not in catalog yet") {
		t.Fatalf("fine_sword should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "amethyst_blade: recipe ingredient \"amethyst_pendant\" is not in catalog yet") {
		t.Fatalf("amethyst_pendant should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "antivenom: recipe ingredient \"poison_apple\" is not in catalog yet") {
		t.Fatalf("poison_apple should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "gold_bar: recipe anchor \"bronze_bar\" is not in catalog yet") {
		t.Fatalf("bronze_bar should now be present in catalog: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "discordant_harp: recipe anchor \"bronze_bar\" is not in catalog yet") {
		t.Fatalf("bronze_bar should now be present in catalog: %s", stdout.String())
	}
	for _, staleWarning := range []string{
		"chillbane: recipe ingredient \"frost_tome\" is not in catalog yet",
		"cinderstaff: recipe anchor \"blazing_rod\" is not in catalog yet",
		"conflagration_staff: recipe ingredient \"phoenix_scroll\" is not in catalog yet",
		"cryoblaze_staff: recipe ingredient \"critical_focus\" is not in catalog yet",
		"elemental_barrier: recipe ingredient \"frostflame_orb\" is not in catalog yet",
		"frostbite_wand: recipe ingredient \"icicle_shard\" is not in catalog yet",
		"cinderstaff: recipe ingredient \"hungry_wand\" is not in catalog yet",
	} {
		if strings.Contains(stdout.String(), staleWarning) {
			t.Fatalf("warning should have disappeared now that item exists: %s\n%s", staleWarning, stdout.String())
		}
	}
}

func TestReviewCatalogCommandShowsStarFiltersAndSource(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"review-catalog", "--catalog", catalogPath()}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{
		"=== Kiwi Dewdrop (kiwi_dewdrop) ===",
		"Source: https://backpackbrawl.wiki.gg/wiki/Kiwi_Dewdrop",
		"Image URL: https://backpackbrawl.wiki.gg/images/Kiwi_Dewdrop.png?61ea77",
		"Image path: assets/items/kiwi_dewdrop.png",
		"Star filters:",
		"types: Jelly Bean",
		"=== Spinegrowth Breastplate (spinegrowth_breastplate) ===",
		"-> any item",
		"=== Apple (apple) ===",
		"types: Food, excluding same item",
		"=== Knight's Sigil (knight_s_sigil) ===",
		"types: Armor",
		"=== Brown Rat (brown_rat) ===",
		"types: Rat",
		"=== Leather Boots (leather_boots) ===",
		"=== Dagger (dagger) ===",
		"=== Jagged Blade (jagged_blade) ===",
		"=== Knight's Razor (knight_s_razor) ===",
		"=== Cauldron (cauldron) ===",
		"=== Water Potion (water_potion) ===",
		"=== Endurance Potion (endurance_potion) ===",
		"=== Antivenom (antivenom) ===",
		"=== Prickly Potion (prickly_potion) ===",
		"=== Skull Cap (skull_cap) ===",
		"=== Fur Boots (fur_boots) ===",
		"=== Mystery Stew (mystery_stew) ===",
		"=== Mallet (mallet) ===",
		"=== Ethereal Cloak (ethereal_cloak) ===",
		"=== Brainsquasher (brainsquasher) ===",
		"=== Iron Sword (iron_sword) ===",
		"=== Whetstone (whetstone) ===",
		"=== Magic Essence (magic_essence) ===",
		"=== Gold Bar (gold_bar) ===",
		"=== Reinforced Shield (reinforced_shield) ===",
		"=== Heater Shield (heater_shield) ===",
		"=== Tusk (tusk) ===",
		"=== Wooden Buckler (wooden_buckler) ===",
		"=== Sash (sash) ===",
		"=== Leather Belt (leather_belt) ===",
		"=== Vanguard Belt (vanguard_belt) ===",
		"=== Magic Belt (magic_belt) ===",
		"=== Bat (bat) ===",
		"=== Amethyst Pendant (amethyst_pendant) ===",
		"=== Amethyst Blade (amethyst_blade) ===",
		"=== Wing Clipper (wing_clipper) ===",
		"=== Twinmaw (twinmaw) ===",
		"=== Death Essence (death_essence) ===",
		"=== Hag's Hat (hag_s_hat) ===",
		"=== Tenacity Potion (tenacity_potion) ===",
		"=== Steadfast Boots (steadfast_boots) ===",
		"=== Ratomelon (ratomelon) ===",
		"=== Poison Dagger (poison_dagger) ===",
		"=== Bronze Bar (bronze_bar) ===",
		"=== Adamantite Bar (adamantite_bar) ===",
		"=== Lucky Tuna (lucky_tuna) ===",
		"=== Cursed Clover (cursed_clover) ===",
		"=== Mana Potion (mana_potion) ===",
		"=== Knight's Aegis (knight_s_aegis) ===",
		"=== Carp (carp) ===",
		"=== Wizard Robe (wizard_robe) ===",
		"=== Poison Apple (poison_apple) ===",
		"=== Night Dahlia (night_dahlia) ===",
		"=== Steel Bar (steel_bar) ===",
		"=== Muck Rat (muck_rat) ===",
		"=== Health Potion (health_potion) ===",
		"=== Fly Agaric (fly_agaric) ===",
		"=== Coconut (coconut) ===",
		"=== Carrot (carrot) ===",
		"=== Rock (rock) ===",
		"=== Fanged Axe (fanged_axe) ===",
		"=== Traveler's Hat (traveler_s_hat) ===",
		"=== Wooden Stick (wooden_stick) ===",
		"=== Watermelon (watermelon) ===",
		"=== Lucky Clover (lucky_clover) ===",
		"=== Leather Cloak (leather_cloak) ===",
		"=== Broom (broom) ===",
		"=== Ginseng Root (ginseng_root) ===",
		"=== Young Squire (young_squire) ===",
		"=== Knight's Blessing (knight_s_blessing) ===",
		"=== Ironwill Banner (ironwill_banner) ===",
		"=== Hungering Ward (hungering_ward) ===",
		"=== Frost Cleaver (frost_cleaver) ===",
		"=== Celestial Teapot (celestial_teapot) ===",
		"=== Tainted Soulstone (tainted_soulstone) ===",
		"=== Magic Dagger (magic_dagger) ===",
		"=== Green Snapper (green_snapper) ===",
		"=== Serpent's Tongue (serpent_s_tongue) ===",
		"=== Bloodletter (bloodletter) ===",
		"=== Fortress Ring (fortress_ring) ===",
		"=== Cursed Idol (cursed_idol) ===",
		"=== Gambler's Dice (gambler_s_dice) ===",
		"=== Marshmallow (marshmallow) ===",
		"=== Cosmos Ward (cosmos_ward) ===",
		"=== Elemental Barrier (elemental_barrier) ===",
		"=== Mana Shield (mana_shield) ===",
		"=== Searing Wand (searing_wand) ===",
		"=== Cryoblaze Staff (cryoblaze_staff) ===",
		"=== Frostbite Wand (frostbite_wand) ===",
		"=== Chillbane (chillbane) ===",
		"=== Frozen Stick (frozen_stick) ===",
		"=== Wand of Backdraft (wand_of_backdraft) ===",
		"=== Wildfire Essence (wildfire_essence) ===",
		"=== Frozen Sprite (frozen_sprite) ===",
		"=== Fire Sprite (fire_sprite) ===",
		"=== Conflagration Staff (conflagration_staff) ===",
		"=== Cinderstaff (cinderstaff) ===",
		"=== Wicked Stick (wicked_stick) ===",
		"=== Mana Crystal (mana_crystal) ===",
		"items: starbloom",
		"=== Belt of Firethrowing (belt_of_firethrowing) ===",
		"=== Starlight Potion (starlight_potion) ===",
		"Counts as: starbloom x1",
		"=== Starbloom (starbloom) ===",
		"=== Mana Stew (mana_stew) ===",
		"=== Critical Focus (critical_focus) ===",
		"=== Frostflame Orb (frostflame_orb) ===",
		"=== Fire Vortex (fire_vortex) ===",
		"=== Frost Barrier (frost_barrier) ===",
		"=== Moldemort (moldemort) ===",
		"=== Unholy Skull (unholy_skull) ===",
		"=== Blue Chilly Cheese (blue_chilly_cheese) ===",
		"=== Succulent Cheese (succulent_cheese) ===",
		"=== Wild Magic Scroll (wild_magic_scroll) ===",
		"=== Arcane Scroll (arcane_scroll) ===",
		"=== Frost Scroll (frost_scroll) ===",
		"=== Frost Dagger (frost_dagger) ===",
		"=== Frost Emblem (frost_emblem) ===",
		"=== Spell Scroll (spell_scroll) ===",
		"=== Flame Cloak (flame_cloak) ===",
		"=== Phoenix Scroll (phoenix_scroll) ===",
		"=== Phoenix Feather (phoenix_feather) ===",
		"=== Wizard Hat (wizard_hat) ===",
		"=== Icicle Shard (icicle_shard) ===",
		"=== Frost Tome (frost_tome) ===",
		"=== Blazing Rod (blazing_rod) ===",
		"=== Hungry Wand (hungry_wand) ===",
		"=== Tournament Lance (tournament_lance) ===",
		"=== Rhongomiant (rhongomiant) ===",
		"=== Errant Lance (errant_lance) ===",
		"=== Fanged Blade (fanged_blade) ===",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestSolveJSONCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"solve",
		"--catalog", catalogPath(),
		"--items", "kiwi_dewdrop:2",
		"--top", "1",
		"--max-nodes", "0",
		"--workers", "1",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload)=%d want 1", len(payload))
	}
}

func TestSolveScenarioCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"solve",
		"--catalog", catalogPath(),
		"--scenario", scenarioPath(),
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if len(payload) != 3 {
		t.Fatalf("len(payload)=%d want 3", len(payload))
	}
	score := payload[0]["score"].(map[string]any)
	if score["crafts"].(float64) != 1 {
		t.Fatalf("crafts=%v want 1", score["crafts"])
	}
}

func TestSolveScenarioFlagsOverrideFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"solve",
		"--catalog", catalogPath(),
		"--scenario", scenarioPath(),
		"--top", "1",
		"--workers", "2",
		"--max-nodes", "0",
		"--json",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload)=%d want 1", len(payload))
	}
}

func TestSolveScenarioInvalidGridReturnsError(t *testing.T) {
	path := writeScenarioFixture(t, `{
  "grid": ["111111111"],
  "items": {"cactus": 1}
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"solve",
		"--catalog", catalogPath(),
		"--scenario", path,
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "grid must have 6 rows") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestSolveWorkersProduceSameBestJSON(t *testing.T) {
	oneWorker := solveJSON(t, "1")
	fourWorkers := solveJSON(t, "4")

	if oneWorker != fourWorkers {
		t.Fatalf("worker outputs differ:\n1=%s\n4=%s", oneWorker, fourWorkers)
	}
}

func TestBenchmarkScenariosCommandWritesReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tiny.json"), []byte(`{
  "name": "tiny",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "benchmark.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"benchmark-scenarios",
		"--catalog", catalogPath(),
		"--dir", dir,
		"--budgets", "100,200",
		"--repeat", "1",
		"--out", outPath,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read benchmark: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(content))
	}
	if payload["repair_search_mode"] != "scenario" {
		t.Fatalf("repair_search_mode=%v want scenario", payload["repair_search_mode"])
	}
	runs := payload["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("len(runs)=%d want 2", len(runs))
	}
	for _, rawRun := range runs {
		run := rawRun.(map[string]any)
		if run["repair_search"].(bool) {
			t.Fatalf("repair_search=true want false from scenario")
		}
	}
}

func TestBenchmarkScenariosCommandRepairSearchModeOff(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tiny.json"), []byte(`{
  "name": "tiny",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"kiwi_dewdrop": 2},
  "repair_search": true
}`), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "benchmark.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"benchmark-scenarios",
		"--catalog", catalogPath(),
		"--dir", dir,
		"--budgets", "100",
		"--repeat", "1",
		"--repair-search-mode", "off",
		"--out", outPath,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read benchmark: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(content))
	}
	if payload["repair_search_mode"] != "off" {
		t.Fatalf("repair_search_mode=%v want off", payload["repair_search_mode"])
	}
	runs := payload["runs"].([]any)
	run := runs[0].(map[string]any)
	if run["repair_search"].(bool) {
		t.Fatalf("repair_search=true want false with mode off")
	}
}

func TestBenchmarkScenariosCommandRejectsInvalidRepairSearchMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"benchmark-scenarios",
		"--repair-search-mode", "bad",
	}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "repair search mode") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCompareBenchmarksCommandSelfTie(t *testing.T) {
	reportPath := writeBenchmarkReportFixture(t, `{
  "runs": [
    {
      "scenario": "tiny",
      "budget": 100,
      "repeat": 1,
      "elapsed_ms": 5,
      "score": {"priority_counts": [1], "crafts": 0, "stars": 2, "items": 2},
      "layout_key": "a",
      "search": {"nodes_explored": 100, "limited": true}
    }
  ]
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"compare-benchmarks", reportPath, reportPath}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wins=0 losses=0 ties=1") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestCompareBenchmarksCommandReturnsOneOnScoreLoss(t *testing.T) {
	baselinePath := writeBenchmarkReportFixture(t, `{
  "runs": [
    {
      "scenario": "tiny",
      "budget": 100,
      "repeat": 1,
      "elapsed_ms": 5,
      "score": {"priority_counts": [2], "crafts": 0, "stars": 2, "items": 2},
      "layout_key": "a",
      "search": {"nodes_explored": 100, "limited": true}
    }
  ]
}`)
	currentPath := writeBenchmarkReportFixture(t, `{
  "runs": [
    {
      "scenario": "tiny",
      "budget": 100,
      "repeat": 1,
      "elapsed_ms": 4,
      "score": {"priority_counts": [1], "crafts": 0, "stars": 9, "items": 2},
      "layout_key": "a",
      "search": {"nodes_explored": 100, "limited": true}
    }
  ]
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"compare-benchmarks", baselinePath, currentPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code=%d want 1 stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "Score regressions") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestImportHTMLRequiresPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"import-html"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code=%d want 2", code)
	}
	if !strings.Contains(stderr.String(), "expects one or more saved HTML files or directories") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestImportHTMLReviewCommand(t *testing.T) {
	path := writeHTMLFixture(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"import-html", path}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Grid (#=item, *=star, .=empty):") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestImportHTMLJSONCommand(t *testing.T) {
	path := writeHTMLFixture(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"import-html", "--json", path}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
}

func solveJSON(t *testing.T, workers string) string {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{
		"solve",
		"--catalog", catalogPath(),
		"--items", "kiwi_dewdrop:2,cactus",
		"--top", "1",
		"--max-nodes", "20000",
		"--workers", workers,
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%s", code, stderr.String())
	}
	return stdout.String()
}

func writeHTMLFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Sample - Backpack Brawl Wiki.html")
	content := `
<html>
<head><link rel="canonical" href="https://backpackbrawl.wiki.gg/wiki/Sample"></head>
<body>
<div class="druid-title">Sample</div>
<div class="druid-row druid-row-Type"><a title="Category:Plant_Type_Items">Plant</a></div>
<div class="druid-row druid-row-Cost"></div>
<div class="druid-row druid-row-Grid">
<table><tbody>
<tr><td><img alt="Star"></td><td><img alt="Empty Tile"></td></tr>
<tr><td><img alt="Item Tile"></td><td><img alt="Item Tile"></td></tr>
</tbody></table>
</div>
<h2><span id="Initial_Abilities">Initial Abilities</span></h2>
<p>Does something.</p>
</body>
</html>`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func writeScenarioFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	return path
}

func writeBenchmarkReportFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "benchmark.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write benchmark report: %v", err)
	}
	return path
}

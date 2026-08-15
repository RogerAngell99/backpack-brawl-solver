package wikihtml

import (
	"strings"
	"testing"
)

const cactrioFixture = `
<html>
<head>
<link rel="canonical" href="https://backpackbrawl.wiki.gg/wiki/Cactrio">
<meta property="og:image" content="https://backpackbrawl.wiki.gg/images/Cactrio.png?591dd1">
<meta name="description" content="Cactrio is a Plant type item for Ronan.&#10;When time of day changes:&#10;Gain 3&#10;This counts as 3 Cactus">
</head>
<body>
<div class="druid-title">Cactrio</div>
<div class="druid-row druid-row-Type"><div>Type</div>
<span><a title="Category:Plant_Type_Items"><img alt="Plant Type"></a> Plant</span>
</div><div class="druid-row druid-row-Cost">
<div class="druid-row druid-row-Grid">
<table><tbody>
<tr><td><img alt="Empty Tile"></td><td><img alt="Empty Tile"></td><td><img alt="Empty Tile"></td></tr>
<tr><td><img alt="Empty Tile"></td><td><img alt="Item Tile"></td><td><img alt="Item Tile"></td></tr>
<tr><td><img alt="Empty Tile"></td><td><img alt="Star"></td><td><img alt="Empty Tile"></td></tr>
</tbody></table>
</div>
<h2><span id="Initial_Abilities">Initial Abilities</span></h2>
<p><b>When time of day changes:</b></p>
<ul><li>Gain 3 <img alt="Thorns"></li></ul>
<p>This counts as 3 <a title="Cactus">Cactus</a><br /></p>
<h2><span id="Recipes">Recipes</span></h2>
<table>
<tr><th>Item</th><th>Image</th><th>Recipe</th></tr>
<tr><td rowspan="3"><b><a class="mw-selflink selflink">Cactrio</a></b></td><td rowspan="3"></td><td><a title="Cactus">Cactus</a></td></tr>
<tr><td><a title="Cactus">Cactus</a></td></tr>
<tr><td><a title="Cactus">Cactus</a></td></tr>
<tr><td rowspan="2"><b><a title="Thornwall">Thornwall</a></b></td><td rowspan="2"></td><td><a title="Spiked Shield">Spiked Shield</a></td></tr>
<tr><td><a class="mw-selflink selflink">Cactrio</a></td></tr>
</table>
</body>
</html>
`

func TestExtractCactrioProposal(t *testing.T) {
	proposal, err := Extract(cactrioFixture)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if proposal.Item.ID != "cactrio" || proposal.Item.Name != "Cactrio" {
		t.Fatalf("unexpected item identity: %+v", proposal.Item)
	}
	if len(proposal.Item.Types) != 1 || proposal.Item.Types[0] != "Plant" {
		t.Fatalf("types=%v want [Plant]", proposal.Item.Types)
	}
	assertCoordList(t, proposal.Item.Shape, [][]int{{0, 0}, {0, 1}})
	if len(proposal.Item.Stars) != 1 {
		t.Fatalf("len(stars)=%d want 1", len(proposal.Item.Stars))
	}
	assertCoordList(t, [][]int{proposal.Item.Stars[0].Offset}, [][]int{{1, 0}})

	if len(proposal.Recipes) != 1 {
		t.Fatalf("len(recipes)=%d want 1", len(proposal.Recipes))
	}
	recipe := proposal.Recipes[0]
	if recipe.Result != "cactrio" || recipe.Anchor != "cactus" {
		t.Fatalf("unexpected recipe: %+v", recipe)
	}
	assertStrings(t, recipe.Ingredients, []string{"cactus", "cactus", "cactus"})

	if proposal.Item.AbilityText == "" {
		t.Fatal("ability text should not be empty")
	}
	if proposal.Item.ImageURL != "https://backpackbrawl.wiki.gg/images/Cactrio.png?591dd1" {
		t.Fatalf("image_url=%q", proposal.Item.ImageURL)
	}
	if proposal.Item.ImagePath != "assets/items/cactrio.png" {
		t.Fatalf("image_path=%q", proposal.Item.ImagePath)
	}
	if proposal.Description == "" {
		t.Fatal("description should not be empty")
	}
}

func TestReviewTextIncludesHumanReadableGrid(t *testing.T) {
	proposal, err := Extract(cactrioFixture)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	review := ReviewText([]Proposal{proposal})

	for _, want := range []string{
		"=== Cactrio (cactrio) ===",
		"Image URL: https://backpackbrawl.wiki.gg/images/Cactrio.png?591dd1",
		"Image path: assets/items/cactrio.png",
		"Grid (#=item, *=star, .=empty):",
		"# #",
		"* .",
		"cactrio = cactus + cactus + cactus (anchor: cactus)",
	} {
		if !contains(review, want) {
			t.Fatalf("review missing %q:\n%s", want, review)
		}
	}
}

func assertCoordList(t *testing.T, got [][]int, want [][]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d", len(got), len(want))
	}
	for idx := range got {
		if len(got[idx]) != 2 || got[idx][0] != want[idx][0] || got[idx][1] != want[idx][1] {
			t.Fatalf("got[%d]=%v want %v", idx, got[idx], want[idx])
		}
	}
}

func contains(value string, substring string) bool {
	return len(substring) == 0 || (len(value) >= len(substring) && strings.Contains(value, substring))
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d", len(got), len(want))
	}
	for idx := range got {
		if got[idx] != want[idx] {
			t.Fatalf("got[%d]=%q want %q", idx, got[idx], want[idx])
		}
	}
}

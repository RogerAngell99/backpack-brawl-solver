# Hero Item Scope

Item availability is captured from `ItemDefinitionBuilder.connectedHeroes`.
Recipes use `RecipeData.hero`. The canonical hero universe comes from
`Heroes.HeroCollection.heroes`.

## Scope Values

| `hero_scope.kind` | Meaning |
| --- | --- |
| `shared` | Available to every playable hero in the captured universe |
| `hero_specific` | Available to exactly one hero |
| `multi_hero` | Available to more than one, but not every, hero |
| `unknown` | The runtime did not provide enough evidence |

An empty, present `connectedHeroes` collection is interpreted as shared. This is
confirmed by the runtime behavior of generic items such as Watermelon. A missing
field remains `unknown`.

The current capture contains 27 heroes, including 17 playable heroes, and 1,196
items. It classifies 366 items as shared and 830 as hero-specific. The raw capture
contains 364 hero-specific recipes, 106 shared recipes, and 18 unresolved generic
records. After catalog validation, 477 consolidated recipes remain, including 7
legacy recipes whose scope is still unknown.

## Capture

Use the lightweight profile when item graphs and levels already exist in a separate
deep capture:

```powershell
python analysis/tools/collect_runtime.py `
  --mode attach `
  --profile hero-scope `
  --wait 180 `
  --output analysis/derived/6.1.1/hero-scope
```

The profile records:

- Canonical hero IDs, names, English names, and NPC status.
- `connectedHeroes` for every materialized item definition.
- `RecipeData.hero` for every recipe.

Merge scope data with the deep item capture:

```powershell
python analysis/tools/normalize_runtime.py `
  --static analysis/derived/6.1.1 `
  --runtime analysis/derived/6.1.1/runtime-deep-attach-1/runtime.json `
  --scope-runtime analysis/derived/6.1.1/hero-scope/runtime.json `
  --output analysis/derived/6.1.1/normalized-hero
```

Then run `build_catalog.py` as documented in `analysis/README.md`.

## Filters

Scenario files support:

```json
"hero_filter": {
  "include_heroes": ["Warrior"],
  "exclude_heroes": [],
  "mode": "any",
  "exclude_mode": "strict",
  "unknown_policy": "exclude"
}
```

`include_heroes` uses the union of selected heroes, so shared items are included.
`mode: "all"` requires availability for every selected hero. `mode: "shared"`
keeps only shared items. `exclude_mode: "strict"` removes anything available to
an excluded hero; `exclusive_only` removes only items whose complete availability
set is contained in the excluded set, preserving shared and multi-hero items.

The CLI equivalents are:

```powershell
go run ./cmd/backpack-brawl-solver solve --hero Warrior --items excalibur,watermelon
go run ./cmd/backpack-brawl-solver solve --hero-mode shared --items watermelon
go run ./cmd/backpack-brawl-solver solve --exclude-hero Warrior --hero-exclude-mode exclusive_only --items watermelon
```

Unknown scopes are excluded by default when a filter is active. Use
`--hero-unknown-policy include` only when intentionally accepting incomplete data.

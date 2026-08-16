# Star Condition Capture Protocol

Use this protocol for targeted runtime captures. It is intentionally focused on a
small representative matrix rather than item-by-item collection.

## Preparation

- Use the installed mod APK already connected through ADB.
- Keep the game on the preparation/inventory screen.
- Do not start a fight, sell items, reroll unnecessarily, or change persistent data.
- Arrange only the items needed for one positive or negative case.
- Record the case name, item positions, rotation, levels, and event state before capture.

## Capture

```powershell
python analysis/tools/collect_runtime.py `
  --mode attach `
  --profile star-state `
  --wait 35 `
  --output analysis/derived/6.1.1/star-state-<case>
```

The collector hooks the game's original
`ItemStarSlotUpdater.StarConditionHasEffect` method, records its original return
value, and samples the rendered `has_effect`/`no_effect` sprite state. It also records
the complete `Inventory.managedItems`, `Storage.managedItems`, and active `ItemShape`
lists. For definition comparisons, it additionally probes the concrete
`DefinitionIsSame`, `DefinitionIsDifferent`, and `OtherItemIsExactly` nodes directly, so
short-circuiting in an `any` compound does not hide the result of an individual branch.

## Report

```powershell
python analysis/tools/summarize_star_state.py `
  --runtime analysis/derived/6.1.1/star-state-<case>/runtime.json `
  --metadata data/item-metadata.json `
  --output analysis/derived/6.1.1/star-state-<case>/report.json
```

Review these fields:

- `observation_count`.
- `result_counts`.
- `condition_coverage`.
- `direct_condition_coverage`.
- `direct_condition_observation_count`.
- `ui_mismatch_count`.
- `errors`.
- `latest_inventory_snapshot`.

## Fixture Extraction

After review, extract only the smallest context needed for a deterministic Go test.
Do not add the full runtime capture to Git. A fixture should contain the source item,
target item, relevant item types/stats, condition graph, layout relation, event state,
and expected tri-state result.

## Safety and Git

- Stop `frida-server` after the capture.
- Keep `analysis/.gitignore` entries for `derived/*/star-state-*/`.
- Do not stage with `git add .`.
- Review `git status`, `git diff --check`, and the intended file list before committing.
- Capture tooling, solver behavior, tests, and documentation use separate commits.

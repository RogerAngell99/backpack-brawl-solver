# Star Condition Coverage

This document tracks the runtime star-condition graph coverage for Backpack Brawl
6.1.1. The graph is structural data extracted from the client. A runtime result is
contextual and must be recorded with the layout and event state that produced it.

## Status

The latest targeted capture is `derived/6.1.1/star-condition-branches-final/` and is
ignored by Git. It recorded 108 root evaluations and 72 direct subcondition probes,
with zero collector errors and zero observed UI mismatches.

## Complete Status Matrix

Counts are distinct catalog items containing the class, not individual graph nodes.

Levels describe implementation maturity:

- **Nível 4:** evaluator implemented, catalog-integrated, and validated with runtime
  positive/negative cases used by the current solver.
- **Nível 3:** evaluator implemented and runtime/current-inventory evidence exists, but
  broader calibration is still pending.
- **Nível 2:** evaluator supports explicit context and has fixtures, but lacks regular
  runtime calibration or current-inventory evidence.
- **Nível 1:** runtime graph is preserved, but the evaluator returns `unknown`.

| Priority | Level | Class | Items | Status | Evidence |
| --- | ---: | --- | ---: | --- | --- |
| P0 | 4 | `CompoundStarCondition` | 395 | Supported | Runtime and current-inventory fixtures |
| P0 | 4 | `OtherItemIsOfType` | 405 | Supported | Runtime and current-inventory fixtures |
| P0 | 4 | `DefinitionIsDifferent` | 78 | Supported | Runtime and current-inventory fixtures |
| P0 | 3 | `OtherItemHasStatOfType` | 76 | Implemented; broad calibration pending | Errant Lance: Cactus/Cactrio/Pitahaya |
| P1 | 4 | `DefinitionIsSame` | 27 | Supported and directly runtime-validated | Hooded Cowl duplicate and Cactus probes |
| P1 | 4 | `OtherItemIsExactly` | 31 | Supported and directly runtime-validated | Steel Forge Hammer -> Rock positive; Mining Pick direct negative |
| P1 | 1 | `HasAvailableModificationSlots` | 27 | Evaluator pending | Runtime graph preserved |
| P1 | 1 | `OtherItemCanBeUsedAsModification` | 27 | Evaluator pending | Runtime graph preserved |
| P2 | 1 | `OtherItemCanAddSpecificStat` | 7 | Evaluator pending | Runtime graph preserved |
| P2 | 4 | `OtherItemHasItemActivatedSignal` | 16 | Any placed target is supported | Magic Essence runtime positives and empty-slot negative |
| P2 | 1 | `ItemIsNotConnectedToOwnerHero` | 6 | Evaluator pending | Runtime graph preserved |
| P2 | 1 | `ItemIsConnectedToOwnerHero` | 1 | Evaluator pending | Runtime graph preserved |
| P2 | 1 | `ThisItemIsAboveBaseLevel` | 2 | Evaluator pending | Runtime graph preserved |
| P2 | 2 | `OtherIsEmptyBag` | 2 | Context-supported | Context fixture available |
| P3 | 1 | `OtherItemHasTimeOfDayChangedSignal` | 1 | Evaluator pending | Runtime graph preserved |
| P3 | 1 | `CompareCondition` | 1 | Evaluator pending | Runtime graph preserved |
| P3 | 1 | `IsFirstOccurrenceOfSameDefinition` | 1 | Evaluator pending | Runtime graph preserved |

The evaluator returns `unknown` for the not-implemented classes. The current
inventory has no unknown source graph among `Errant Lance`, `Carp`, `Ginseng Root`,
`Pitahaya`, `Tender Sausage`, `Pet Collar`, `Traveler's Hat`, `Venomous Pincer`,
and `Weapons Rack`.

## Errant Lance Calibration

The capture `derived/6.1.1/star-state-errant-lance-1/` used the current inventory
with Cactus, Cactrio, and an empty Errant Lance star position. It recorded 548
evaluations, 9 sources, 28 unique contexts, zero errors, and zero UI mismatches.

For `OtherItemHasStatOfType(CactusCount)` the observed results were:

| Target | Runtime result |
| --- | --- |
| `Cactus` | `true` |
| `Cactrio` | `true` |
| `Pitahaya` | `false` |
| Empty star position | `no_effect` visually |

This confirms that the condition is stat-based and accepts both Cactus and Cactrio,
not only the legacy exact-target list.

The same inventory also exposed supported structural conditions in `Carp`,
`Ginseng Root`, `Pitahaya`, `Tender Sausage`, `Pet Collar`, `Traveler's Hat`,
`Venomous Pincer`, and `Weapons Rack`; these are included in the runtime regression
fixtures where their observed positive and negative states are available.

## DefinitionIsSame Calibration

The capture `derived/6.1.1/star-condition-branches-final/` contained two `Hooded Cowl`
instances. Direct runtime probes produced:

| Condition | Target | Result |
| --- | --- | --- |
| `DefinitionIsSame` | second `Hooded Cowl` | `true` |
| `DefinitionIsSame` | `Cactus` | `false` |
| `DefinitionIsDifferent` | second `Hooded Cowl` | `false` |
| `DefinitionIsDifferent` | `Cactus` | `true` |

The root visual result also agreed with the logical result in all cases.

The direct fixtures isolate `DefinitionIsSame` itself:

| Source | Target | Direct condition result |
| --- | --- | --- |
| `Hooded Cowl` | second `Hooded Cowl` | `true` |
| `Hooded Cowl` | `Cactus` | `false` |

The runtime graph is an `any` compound containing both `DefinitionIsSame` and
`DefinitionIsDifferent`, so a non-empty target activates the complete graph regardless
of which branch is responsible. Direct subcondition probes now isolate both branches
despite that short-circuit behavior.

## OtherItemIsExactly Calibration

The current runtime inventory contained `Steel Forge Hammer` and `Rock`. The root
condition returned `true` for that pair, and `Rock` is not a `Melee Weapon`, so the
positive result exercises the exact-definition branch of the `any` compound. The same
source also accepts `Melee Weapon` targets through its separate type branch.

With `Rock` removed from the active target and `Mining Pick` placed instead, the direct
runtime probe returned `false` for `OtherItemIsExactly(Steel Forge Hammer, Mining Pick)`
in six observations. The root condition remained `true` because `Mining Pick` is a
`Melee Weapon`; this confirms both branches independently rather than treating the
compound result as the exact-definition result.

The capture had zero collector errors and zero UI mismatches. The solver fixtures cover
the exact positive, exact negative, and `Melee Weapon` alternative cases.

The production catalog contains 528 runtime graphs. The solver uses a graph when the
item has one, falls back to the curated star rule when it does not, and treats an
unsupported or dynamic condition without context as `unknown`.

## Activated Signal Calibration

`OtherItemHasItemActivatedSignal` is a static placement predicate, not a requirement
that the target has already activated in combat. Three captures on the preparation screen
intercepted the original `ItemStarSlotUpdater.StarConditionHasEffect` return and matched
the rendered sprite in every observation:

| Capture | Placed Magic Essence targets | Result |
| --- | --- | --- |
| `star-state-magic-essence-current-1` | Lumps of Coal, Celestial Teapot, Crystal Cluster | all active |
| `star-state-magic-essence-gold-bar-1` | Gold Bar plus the existing targets | Gold Bar active; all placed targets active |
| `star-state-magic-essence-bag-1` | Medium Bag plus the existing targets | Medium Bag active; all placed targets active |

The captures recorded no UI mismatches. An empty Magic Essence star slot was inactive
in every layout. Gold Bar and Medium Bag have no `Cooldown` stat, so cooldown is not
the predicate.
For layout evaluation, the condition is therefore true for every placed target; the
solver already excludes empty star cells before condition evaluation.

## Coverage Matrix

The item names below are candidates selected from the runtime metadata. They are
representatives, not a requirement to capture every item.

| Condition class | Candidate items | Positive setup | Negative setup |
| --- | --- | --- | --- |
| `OtherItemIsOfType` | Adamantite Forge Hammer, Aeonglass, Watermelon | Put the required type on the star cell | Put a different type there |
| `DefinitionIsDifferent` | Watermelon, Apple, Banana | Use a different item definition | Use a duplicate of the source |
| `DefinitionIsSame` | Abyssal Embrace, Arcane Astrolabe, Chainmail | Put a duplicate on the star cell | Put a different definition there |
| `OtherItemHasStatOfType` | Arrow Sheath, Autotrigger, Bargain of Borrowed Time | Use a target with the required stat | Use a target without the stat |
| `OtherItemIsExactly` | Steel Forge Hammer, Adamantite Forge Hammer, Admiralty Anchor | Use the exact referenced item | Use another item of the same broad type |
| `OtherIsEmptyBag` | Blood Moon Berry, Bag of Maggots | Use an empty bag | Put an item inside the bag |
| `OtherItemHasItemActivatedSignal` | Magic Essence, Life Essence, Death Essence | Put any item on the star cell | Leave the star cell empty |
| `HasAvailableModificationSlots` | Admiralty Anchor, Apophian Saif, Big Chocolate Gift Box | Leave a modification slot available | Fill the modification slot |
| `OtherItemCanBeUsedAsModification` | Admiralty Anchor, Apophian Saif, Big Chocolate Gift Box | Use a valid modification item | Use an invalid item |
| `OtherItemCanAddSpecificStat` | Croissant, Dormant Dragon Egg, Loaf of Bread | Target can receive the player stat | Target cannot receive it |
| `ItemIsConnectedToOwnerHero` | Counterfeit Coin | Connect the item to the hero | Remove the connection |
| `ItemIsNotConnectedToOwnerHero` | Hypnodisc Deflector, Partial Invisibility Cloak, Counterfeit Coin | Remove the connection | Connect the item |
| `ThisItemIsAboveBaseLevel` | Galvanic Armor, Stormbreaker Armor | Use an upgraded level | Use base level |
| `OtherItemHasTimeOfDayChangedSignal` | Ocarina Of Thyme | Capture after the time signal | Capture before the signal |
| `CompareCondition` | Mysterious Lamp | Satisfy the comparison | Break the comparison |
| `IsFirstOccurrenceOfSameDefinition` | Nice Spice Rack | Evaluate the first duplicate | Evaluate a later duplicate |

## Evidence Rules

- A class is calibrated only after one positive and one negative runtime result.
- Dynamic classes require the event or game state in the capture context.
- A visual sprite result must agree with the logical `HasEffect` return.
- A failed or missing context is `unknown`, not `false`.
- Raw runtime JSON and logcat remain ignored; only sanitized fixtures are versioned.

## Recommended Order

1. Runtime-calibrate `OtherItemIsExactly`: static condition, 31 items, and no event/modification state.
2. `HasAvailableModificationSlots` plus `OtherItemCanBeUsedAsModification`: related
   conditions covering 27 items each; one carefully designed modification capture can
   calibrate both.
3. `OtherItemCanAddSpecificStat`: same modification capture, 7 items.
4. Hero connectivity and base-level conditions: 9 items total and relatively simple
   state toggles.
5. Empty-bag conditions: context-dependent but already modeled.
6. Time-of-day, comparison, and first-occurrence conditions: one item each and lower
   return on capture effort.

## Remaining Gaps

The graph classes are present structurally for the remaining items, but the runtime
matrix still needs broader positive and negative contexts for stat selectors,
exact-definition references, modification slots, hero connectivity, level comparisons,
time signals, and comparison conditions.

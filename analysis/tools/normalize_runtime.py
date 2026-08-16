"""Join static assets/localization with the runtime item and recipe capture."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

try:
    from analysis.tools.coordinates import unity_local_to_grid
except ModuleNotFoundError:
    from coordinates import unity_local_to_grid


def slug(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", value.lower()).strip("_")


def coordinate_pair(value: Any) -> list[int] | None:
    if not isinstance(value, list) or len(value) != 2:
        return None
    if not all(isinstance(component, (int, float)) for component in value):
        return None
    rounded = [round(component) for component in value]
    if any(abs(component - rounded[index]) > 0.0001 for index, component in enumerate(value)):
        return None
    return [int(component) for component in rounded]


def runtime_coordinates(value: Any) -> list[list[int]]:
    """Convert Unity local [x, y] positions to the catalog [row, col] contract."""
    coordinates: list[list[int]] = []
    for raw_coordinate in value if isinstance(value, list) else []:
        coordinate = coordinate_pair(raw_coordinate)
        if coordinate is not None:
            coordinates.append(unity_local_to_grid(coordinate[0], coordinate[1]))
    return coordinates


def static_coordinates(value: Any) -> list[list[int]]:
    """Read static extractor output, which is already stored as [row, col]."""
    coordinates: list[list[int]] = []
    for raw_coordinate in value if isinstance(value, list) else []:
        coordinate = coordinate_pair(raw_coordinate)
        if coordinate is not None:
            coordinates.append(coordinate)
    return coordinates


def normalized_hero(raw: dict[str, Any]) -> dict[str, Any]:
    hero_id = str(raw.get("id") or "").strip()
    return {
        "id": hero_id,
        "name": raw.get("name") or raw.get("english_name") or hero_id,
        "english_name": raw.get("english_name") or raw.get("name") or hero_id,
        "npc": bool(raw.get("npc", False)),
    }


def hero_scope(
    available_to: list[str] | None,
    present: bool,
    hero_ids: set[str],
    source: str,
) -> dict[str, Any]:
    available = sorted({value for value in (available_to or []) if value})
    if not present or not hero_ids:
        return {"available_to": available, "kind": "unknown", "status": "unknown", "source": source}
    if not available:
        available = sorted(hero_ids)
        return {"available_to": available, "kind": "shared", "status": "confirmed", "source": source}
    available_set = set(available)
    if available_set == hero_ids:
        kind = "shared"
    elif len(available_set) == 1:
        kind = "hero_specific"
    else:
        kind = "multi_hero"
    return {"available_to": available, "kind": kind, "status": "confirmed", "source": source}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--static", type=Path, required=True)
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--scope-runtime", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)

    assets = {item["name"]: item for item in json.loads((args.static / "items" / "assets.json").read_text(encoding="utf-8"))["items"]}
    localization = json.loads((args.static / "items" / "localization.json").read_text(encoding="utf-8"))
    runtime = json.loads(args.runtime.read_text(encoding="utf-8"))
    data = runtime.get("data") or {}
    scope_data = json.loads(args.scope_runtime.read_text(encoding="utf-8")).get("data", {}) if args.scope_runtime else {}
    scope_items = {item.get("id"): item for item in scope_data.get("items", []) if item.get("id")}
    scope_recipes = {
        json.dumps([recipe.get("primary"), recipe.get("secondaries", []), recipe.get("result")], sort_keys=True): recipe
        for recipe in scope_data.get("recipes", [])
    }
    heroes = [normalized_hero(hero) for hero in (scope_data.get("heroes") or data.get("heroes", [])) if hero.get("id")]
    hero_ids = {hero["id"] for hero in heroes if not hero.get("npc")}
    english = localization.get("tables", {}).get("en", {})
    portuguese = localization.get("tables", {}).get("pt-BR", {})

    normalized: list[dict[str, Any]] = []
    for raw in data.get("items", []):
        scope_item = scope_items.get(raw.get("id"), {})
        if scope_item:
            raw = {**raw, **{key: scope_item[key] for key in ("connected_heroes", "connected_heroes_present") if key in scope_item}}
        name = raw.get("asset_id") or raw.get("id")
        static = assets.get(name, {})
        levels = raw.get("levels") or {}
        level_effects = []
        for level in levels.get("levels", []):
            value = level.get("value") or {}
            serialized = value if isinstance(value, dict) else {}
            selector = serialized.get("_statType") or {}
            type_name = None
            if isinstance(selector, dict):
                candidate = selector.get("type_name") or selector.get("_typeName")
                type_name = candidate if isinstance(candidate, str) else None
            level_effects.append({
                "level": level.get("level"),
                "kind": serialized.get("class"),
                "value": serialized.get("_value"),
                "stat_target": type_name if isinstance(type_name, str) else None,
                "modifier_type": (serialized.get("_modifierType") or {}).get("name") if isinstance(serialized.get("_modifierType"), dict) else None,
                "raw": serialized,
            })
        display_name = raw.get("display_name") or raw.get("id")
        connected_heroes = raw.get("connected_heroes")
        connected_heroes_present = bool(raw.get("connected_heroes_present", connected_heroes is not None))
        connected_hero_ids = [str(hero.get("id")) for hero in connected_heroes or [] if hero.get("id")]
        runtime_shape = raw.get("base_shape")
        runtime_stars = raw.get("star_shape")
        normalized.append({
            "id": slug(raw.get("id") or name),
            "client_id": raw.get("id"),
            "name": display_name,
            "asset_id": raw.get("asset_id"),
            "types": raw.get("item_types", []),
            "rarity": raw.get("rarity"),
            "layer": raw.get("layer"),
            "shape": runtime_coordinates(runtime_shape) if runtime_shape else static_coordinates(static.get("cells", [])),
            "star_positions": runtime_coordinates(runtime_stars) if runtime_stars else static_coordinates(static.get("star_positions", [])),
            "stats": [{"type": item.get("class", "").split(".")[-1], "value": item.get("value")} for item in raw.get("stats", [])],
            "levels": {"max_level": levels.get("max_level"), "effects": level_effects},
            "ability_text": english.get(name) or english.get(display_name),
            "ability_text_pt_br": portuguese.get(name) or portuguese.get(display_name),
            "star_condition_graph": raw.get("star_condition_graph"),
            "hero_scope": hero_scope(connected_hero_ids, connected_heroes_present, hero_ids, "ItemDefinitionBuilder.connectedHeroes"),
            "star_status": {
                "available": False,
                "condition_graph_captured": bool(raw.get("star_condition_graph")),
                "reason": "condition graph captured; active effect depends on layout and context"
                if raw.get("star_condition_graph")
                else "star condition was not present in the materialized definition",
                "raw": raw.get("star_condition"),
            },
            "source": {
                "static_asset": bool(static),
                "runtime": True,
                "confidence": "runtime_partial",
            },
        })

    recipes = []
    for raw_recipe in data.get("recipes", []):
        recipe_key = json.dumps([raw_recipe.get("primary"), raw_recipe.get("secondaries", []), raw_recipe.get("result")], sort_keys=True)
        recipe = {**raw_recipe, **scope_recipes.get(recipe_key, {})}
        hero_id = str(recipe.get("hero_id") or "").strip()
        if hero_id:
            recipe["hero_scope"] = hero_scope([hero_id], True, hero_ids, "RecipeData.hero")
        elif recipe.get("source_class") == "Model.ItemCombinations.RecipeData":
            recipe["hero_scope"] = hero_scope(sorted(hero_ids), True, hero_ids, "RecipeData.hero")
        else:
            recipe["hero_scope"] = hero_scope([], False, hero_ids, "ItemCombinationData.hero")
        recipes.append(recipe)
    (args.output / "items.json").write_text(json.dumps({"heroes": heroes, "items": normalized}, indent=2, ensure_ascii=True), encoding="utf-8")
    (args.output / "recipes.json").write_text(json.dumps({"recipes": recipes}, indent=2, ensure_ascii=True), encoding="utf-8")
    summary = {
        "runtime_file": str(args.runtime),
        "items": len(normalized),
        "heroes": len(heroes),
        "items_with_stats": sum(bool(item["stats"]) for item in normalized),
        "items_with_levels": sum(bool(item["levels"]["effects"]) for item in normalized),
        "items_with_shapes": sum(bool(item["shape"]) for item in normalized),
        "items_with_star_positions": sum(bool(item["star_positions"]) for item in normalized),
        "items_with_star_condition_graph": sum(bool(item["star_condition_graph"]) for item in normalized),
        "level_effects_with_stat_target": sum(
            bool(effect["stat_target"])
            for item in normalized
            for effect in item["levels"]["effects"]
        ),
        "recipes": len(recipes),
        "unresolved": ["star_status", "level_effects.stat_target when ItemStatSelector is opaque"],
    }
    (args.output / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()

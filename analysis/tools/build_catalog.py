"""Build the solver catalog from curated data and client runtime captures."""

from __future__ import annotations

import argparse
import copy
import json
import re
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")


def slug(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", value.lower()).strip("_")


def as_list(value: Any) -> list[Any]:
    return value if isinstance(value, list) else []


def coordinate_pair(value: Any) -> list[int] | None:
    if not isinstance(value, list) or len(value) != 2:
        return None
    if not all(isinstance(component, (int, float)) for component in value):
        return None
    rounded = [round(component) for component in value]
    if any(abs(component - rounded[index]) > 0.0001 for index, component in enumerate(value)):
        return None
    return [int(component) for component in rounded]


def normalized_shape(item: dict[str, Any], static: dict[str, Any]) -> list[list[int]]:
    shape = item.get("shape") or static.get("cells") or []
    return [coordinate for cell in shape if (coordinate := coordinate_pair(cell)) is not None]


def normalized_star_positions(item: dict[str, Any], static: dict[str, Any]) -> list[list[int]]:
    positions = item.get("star_positions") or static.get("star_positions") or []
    return [coordinate for position in positions if (coordinate := coordinate_pair(position)) is not None]


def star_offsets(stars: list[dict[str, Any]]) -> list[list[int]]:
    return [list(star.get("offset", [])) for star in stars]


def rotate_coordinate(coordinate: list[int], rotation: int) -> tuple[int, int]:
    row, col = coordinate
    if rotation == 0:
        return row, col
    if rotation == 90:
        return col, -row
    if rotation == 180:
        return -row, -col
    return -col, row


def normalized_geometry(shape: list[list[int]], stars: list[list[int]], rotation: int) -> tuple[list[tuple[int, int]], list[tuple[int, int]]]:
    rotated_shape = [rotate_coordinate(coordinate, rotation) for coordinate in shape]
    if not rotated_shape:
        return [], []
    min_row = min(row for row, _ in rotated_shape)
    min_col = min(col for _, col in rotated_shape)
    normalize = lambda coordinate: (coordinate[0] - min_row, coordinate[1] - min_col)
    return sorted(normalize(coordinate) for coordinate in rotated_shape), sorted(
        normalize(rotate_coordinate(coordinate, rotation)) for coordinate in stars
    )


def geometry_equivalent(
    left_shape: list[list[int]],
    left_stars: list[list[int]],
    right_shape: list[list[int]],
    right_stars: list[list[int]],
) -> bool:
    for rotation in (0, 90, 180, 270):
        rotated_shape, rotated_stars = normalized_geometry(left_shape, left_stars, rotation)
        right_normalized_shape, right_normalized_stars = normalized_geometry(right_shape, right_stars, 0)
        if rotated_shape == right_normalized_shape and set(rotated_stars) == set(right_normalized_stars):
            return True
    return False


def shape_equivalent(left_shape: list[list[int]], right_shape: list[list[int]]) -> bool:
    return geometry_equivalent(left_shape, [], right_shape, [])


def build_unknown_stars(positions: list[list[int]]) -> list[dict[str, Any]]:
    return [
        {
            "offset": position,
            "target_types": [],
            "target_items": [],
            "rule_status": "unknown",
            "effect_text": "",
        }
        for position in positions
    ]


def build_known_stars(positions: list[list[int]], templates: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [
        {"offset": position, **copy.deepcopy(template)}
        for position in positions
        for template in templates
    ]


def star_templates(stars: list[dict[str, Any]]) -> list[dict[str, Any]]:
    templates: list[dict[str, Any]] = []
    seen: set[str] = set()
    for star in stars:
        template = copy.deepcopy(star)
        template.pop("offset", None)
        key = json.dumps(template, sort_keys=True)
        if key in seen:
            continue
        seen.add(key)
        templates.append(template)
    return templates


def copy_known_stars(stars: list[dict[str, Any]]) -> list[dict[str, Any]]:
    copied = []
    for star in stars:
        value = copy.deepcopy(star)
        if value.get("rule_status") == "known":
            value.pop("rule_status")
        copied.append(value)
    return copied


def resolve_id(value: str, runtime_to_catalog: dict[str, str]) -> str:
    return runtime_to_catalog.get(value, value)


def merge_item(
    runtime: dict[str, Any],
    static: dict[str, Any],
    existing: dict[str, Any] | None,
    conflicts: list[dict[str, Any]],
    resolved_overrides: list[dict[str, Any]],
    asset_available: bool = False,
) -> dict[str, Any]:
    item_id = slug(runtime.get("id") or runtime.get("asset_id") or runtime.get("name") or "")
    runtime_types = [str(value) for value in as_list(runtime.get("types"))]
    shape = normalized_shape(runtime, static)
    positions = normalized_star_positions(runtime, static)

    if existing is not None:
        if existing.get("types") and runtime_types and existing["types"] != runtime_types:
            resolved_overrides.append({"item_id": item_id, "field": "types", "curated": existing["types"], "runtime": runtime_types})
        if existing.get("shape") and existing["shape"] != shape:
            resolved_overrides.append({"item_id": item_id, "field": "shape", "curated": existing["shape"], "runtime": shape})
        if existing.get("stars") and star_offsets(existing["stars"]) != positions:
            resolved_overrides.append({"item_id": item_id, "field": "star_positions", "curated": star_offsets(existing["stars"]), "runtime": positions})
        templates = star_templates(existing.get("stars") or [])
        stars = build_known_stars(positions, templates) if templates else build_unknown_stars(positions)
        types = list(runtime_types or existing.get("types") or [])
        shape = copy.deepcopy(shape)
        name = existing.get("name") or runtime.get("name") or runtime.get("client_id") or item_id
        ability_text = existing.get("ability_text") or runtime.get("ability_text") or ""
        source_url = existing.get("source_url") or ""
        image_url = existing.get("image_url") or ""
        image_path = existing.get("image_path") or f"assets/items/{item_id}.png"
        needs_review = bool(existing.get("needs_review", True))
        result = copy.deepcopy(existing)
    else:
        stars = build_unknown_stars(positions)
        types = runtime_types
        name = runtime.get("name") or runtime.get("client_id") or item_id
        ability_text = runtime.get("ability_text") or ""
        source_url = ""
        image_url = ""
        image_path = f"assets/items/{item_id}.png" if asset_available else ""
        needs_review = True
        result = {}

    result.update(
        {
            "id": item_id,
            "name": name,
            "types": types,
            "shape": shape,
            "stars": stars,
            "ability_text": ability_text,
            "source_url": source_url,
            "image_url": image_url,
            "image_path": image_path,
            "needs_review": needs_review,
        }
    )
    if not asset_available:
        result["image_path"] = ""
    return result


def build_recipe(
    raw: dict[str, Any], item_ids: set[str], runtime_to_catalog: dict[str, str] | None = None
) -> tuple[dict[str, Any] | None, dict[str, Any] | None]:
    runtime_to_catalog = runtime_to_catalog or {}
    primary = resolve_id(slug(str(raw.get("primary") or "")), runtime_to_catalog)
    secondaries = [resolve_id(slug(str(value)), runtime_to_catalog) for value in as_list(raw.get("secondaries"))]
    result = resolve_id(slug(str(raw.get("result") or "")), runtime_to_catalog)
    ingredients = [primary, *secondaries]
    if not primary or not result or any(not ingredient for ingredient in ingredients):
        return None, {"recipe": raw, "reason": "empty recipe identifier"}
    missing = sorted({value for value in [*ingredients, result] if value not in item_ids})
    if missing:
        return None, {"recipe": raw, "reason": "missing item id", "missing": missing}
    return {
        "result": result,
        "anchor": primary,
        "ingredients": ingredients,
        "source_url": "",
    }, None


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--current", type=Path, required=True)
    parser.add_argument("--static", type=Path, required=True)
    parser.add_argument("--normalized", type=Path, required=True)
    parser.add_argument("--catalog-output", type=Path, required=True)
    parser.add_argument("--metadata-output", type=Path, required=True)
    parser.add_argument("--report-output", type=Path, required=True)
    parser.add_argument("--id-map-output", type=Path, required=True)
    parser.add_argument("--asset-dir", type=Path, default=Path("assets/items"))
    parser.add_argument("--id-map-input", type=Path, default=Path("data/catalog-id-map.json"))
    args = parser.parse_args()

    current = load_json(args.current)
    assets = {item.get("name"): item for item in load_json(args.static / "items" / "assets.json").get("items", [])}
    normalized = load_json(args.normalized / "items.json").get("items", [])
    runtime_recipes = load_json(args.normalized / "recipes.json").get("recipes", [])
    id_map_input = load_json(args.id_map_input) if args.id_map_input.is_file() else {}
    runtime_to_catalog = {
        slug(str(runtime_id)): slug(str(catalog_id))
        for runtime_id, catalog_id in (id_map_input.get("runtime_to_catalog") or {}).items()
    }

    current_items = {item["id"]: item for item in current.get("items", [])}
    output_items = {item_id: copy.deepcopy(item) for item_id, item in current_items.items()}
    conflicts: list[dict[str, Any]] = []
    resolved_overrides: list[dict[str, Any]] = []
    new_ids: list[str] = []
    matched_ids: list[str] = []
    metadata_items: list[dict[str, Any]] = []

    for runtime in normalized:
        runtime_id = slug(runtime.get("id") or runtime.get("asset_id") or runtime.get("name") or "")
        item_id = resolve_id(runtime_id, runtime_to_catalog)
        static = assets.get(runtime.get("asset_id"), {})
        existing = current_items.get(item_id)
        if existing is None:
            new_ids.append(item_id)
        else:
            matched_ids.append(item_id)
        runtime_for_catalog = copy.deepcopy(runtime)
        runtime_for_catalog["id"] = item_id
        output_items[item_id] = merge_item(
            runtime_for_catalog,
            static,
            existing,
            conflicts,
            resolved_overrides,
            asset_available=(args.asset_dir / f"{item_id}.png").is_file(),
        )
        metadata = copy.deepcopy(runtime)
        metadata["runtime_id"] = runtime_id
        metadata["catalog_id"] = item_id
        metadata["catalog_status"] = "matched" if existing is not None else "new"
        metadata_items.append(metadata)

    catalog_item_ids = set(output_items)
    recipes = {(
        recipe.get("result"),
        recipe.get("anchor"),
        tuple(recipe.get("ingredients", [])),
    ): copy.deepcopy(recipe) for recipe in current.get("recipes", [])}
    invalid_recipes: list[dict[str, Any]] = []
    valid_runtime_recipes = 0
    for raw_recipe in runtime_recipes:
        recipe, error = build_recipe(raw_recipe, catalog_item_ids, runtime_to_catalog)
        if error is not None:
            invalid_recipes.append(error)
            continue
        assert recipe is not None
        valid_runtime_recipes += 1
        key = (recipe["result"], recipe["anchor"], tuple(recipe["ingredients"]))
        recipes.setdefault(key, recipe)

    catalog = {
        "items": [output_items[item_id] for item_id in sorted(output_items)],
        "recipes": [recipes[key] for key in sorted(recipes)],
    }
    metadata = {
        "schema_version": 1,
        "game_version": "6.1.1",
        "source": "client-runtime-capture",
        "items": sorted(metadata_items, key=lambda item: item.get("catalog_id", "")),
    }
    runtime_catalog_ids = {
        resolve_id(slug(item.get("id") or item.get("asset_id") or item.get("name") or ""), runtime_to_catalog)
        for item in normalized
    }
    catalog_only_ids = sorted(set(current_items) - runtime_catalog_ids)
    report = {
        "catalog_items": len(catalog["items"]),
        "catalog_recipes": len(catalog["recipes"]),
        "runtime_items": len(normalized),
        "runtime_recipes": len(runtime_recipes),
        "valid_runtime_recipes": valid_runtime_recipes,
        "matched_ids": len(set(matched_ids)),
        "new_ids": sorted(new_ids),
        "catalog_only_ids": catalog_only_ids,
        "conflicts": conflicts,
        "resolved_overrides": resolved_overrides,
        "invalid_runtime_recipes": invalid_recipes,
        "unresolved": ["star_status", "level_effects.stat_target when ItemStatSelector is opaque"],
    }
    id_map = {
        "runtime_to_catalog": {
            runtime_id: resolve_id(runtime_id, runtime_to_catalog)
            for runtime_id in sorted(
                slug(item.get("id") or item.get("asset_id") or item.get("name") or "") for item in normalized
            )
        },
        "manual_aliases": runtime_to_catalog,
        "catalog_only_ids": catalog_only_ids,
        "manual_review_required": sorted(set(catalog_only_ids) | {entry["item_id"] for entry in conflicts}),
    }

    write_json(args.catalog_output, catalog)
    write_json(args.metadata_output, metadata)
    write_json(args.report_output, report)
    write_json(args.id_map_output, id_map)
    print(json.dumps(report, indent=2, ensure_ascii=True))


if __name__ == "__main__":
    main()

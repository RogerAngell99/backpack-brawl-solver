"""Join static assets/localization with the runtime item and recipe capture."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any


def slug(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", value.lower()).strip("_")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--static", type=Path, required=True)
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)

    assets = {item["name"]: item for item in json.loads((args.static / "items" / "assets.json").read_text(encoding="utf-8"))["items"]}
    localization = json.loads((args.static / "items" / "localization.json").read_text(encoding="utf-8"))
    runtime = json.loads(args.runtime.read_text(encoding="utf-8"))
    data = runtime.get("data") or {}
    english = localization.get("tables", {}).get("en", {})
    portuguese = localization.get("tables", {}).get("pt-BR", {})

    normalized: list[dict[str, Any]] = []
    for raw in data.get("items", []):
        name = raw.get("asset_id") or raw.get("id")
        static = assets.get(name, {})
        levels = raw.get("levels") or {}
        level_effects = []
        for level in levels.get("levels", []):
            value = level.get("value") or {}
            serialized = value if isinstance(value, dict) else {}
            selector = serialized.get("_statType") or {}
            type_name = selector.get("_typeName") if isinstance(selector, dict) else None
            level_effects.append({
                "level": level.get("level"),
                "kind": serialized.get("class"),
                "value": serialized.get("_value"),
                "stat_target": type_name if isinstance(type_name, str) else None,
                "modifier_type": (serialized.get("_modifierType") or {}).get("name") if isinstance(serialized.get("_modifierType"), dict) else None,
                "raw": serialized,
            })
        display_name = raw.get("display_name") or raw.get("id")
        normalized.append({
            "id": slug(raw.get("id") or name),
            "client_id": raw.get("id"),
            "name": display_name,
            "asset_id": raw.get("asset_id"),
            "types": raw.get("item_types", []),
            "rarity": raw.get("rarity"),
            "layer": raw.get("layer"),
            "shape": raw.get("base_shape") or static.get("cells", []),
            "star_positions": raw.get("star_shape") or static.get("star_positions", []),
            "stats": [{"type": item.get("class", "").split(".")[-1], "value": item.get("value")} for item in raw.get("stats", [])],
            "levels": {"max_level": levels.get("max_level"), "effects": level_effects},
            "ability_text": english.get(name) or english.get(display_name),
            "ability_text_pt_br": portuguese.get(name) or portuguese.get(display_name),
            "star_status": {
                "available": False,
                "reason": "star_condition not resolved by safe runtime capture",
                "raw": raw.get("star_condition"),
            },
            "source": {
                "static_asset": bool(static),
                "runtime": True,
                "confidence": "runtime_partial",
            },
        })

    recipes = data.get("recipes", [])
    (args.output / "items.json").write_text(json.dumps({"items": normalized}, indent=2, ensure_ascii=True), encoding="utf-8")
    (args.output / "recipes.json").write_text(json.dumps({"recipes": recipes}, indent=2, ensure_ascii=True), encoding="utf-8")
    summary = {
        "runtime_file": str(args.runtime),
        "items": len(normalized),
        "items_with_stats": sum(bool(item["stats"]) for item in normalized),
        "items_with_levels": sum(bool(item["levels"]["effects"]) for item in normalized),
        "items_with_shapes": sum(bool(item["shape"]) for item in normalized),
        "items_with_star_positions": sum(bool(item["star_positions"]) for item in normalized),
        "recipes": len(recipes),
        "unresolved": ["star_status", "level_effects.stat_target when ItemStatSelector is opaque"],
    }
    (args.output / "summary.json").write_text(json.dumps(summary, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()

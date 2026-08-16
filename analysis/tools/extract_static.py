"""Extract Unity sprites, prefab geometry and localization tables."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import Counter
from pathlib import Path
from typing import Any

import UnityPy

try:
    from analysis.tools.coordinates import unity_local_to_grid
except ModuleNotFoundError:
    from coordinates import unity_local_to_grid


def pointer(value: Any) -> int | None:
    if value is None:
        return None
    if isinstance(value, dict):
        return value.get("m_PathID")
    return getattr(value, "path_id", None)


def read_object(obj: Any) -> Any | None:
    try:
        return obj.read()
    except Exception:
        return None


def read_tree(obj: Any) -> dict[str, Any] | None:
    try:
        value = obj.read_typetree()
        return value if isinstance(value, dict) else None
    except Exception:
        return None


def json_value(value: Any) -> Any:
    if isinstance(value, bytes):
        return {"bytes": len(value)}
    if isinstance(value, (str, int, float, bool)) or value is None:
        return value
    if isinstance(value, dict):
        return {str(key): json_value(item) for key, item in value.items()}
    if isinstance(value, list):
        return [json_value(item) for item in value]
    return str(value)


def find_sprite_records(env: Any, output: Path, item_names: set[str]) -> dict[str, dict[str, Any]]:
    records: dict[str, dict[str, Any]] = {}
    sprite_objects: dict[tuple[str, int], Any] = {}
    for obj in env.objects:
        if obj.type.name != "Sprite":
            continue
        value = read_object(obj)
        name = getattr(value, "m_Name", "") if value else ""
        if not name or name not in item_names:
            continue
        sprite_objects[(obj.assets_file.name, obj.path_id)] = obj
        tree = read_tree(obj) or {}
        rect = tree.get("m_Rect", {})
        record = {
            "name": name,
            "asset_file": obj.assets_file.name,
            "path_id": obj.path_id,
            "confidence": "static",
            "rect": json_value(rect),
            "pixels_to_units": tree.get("m_PixelsToUnits"),
            "atlas_path_id": pointer(tree.get("m_SpriteAtlas")),
        }
        records.setdefault(name, record)
        image = getattr(value, "image", None)
        if image is not None:
            image_dir = output / "items" / "assets"
            image_dir.mkdir(parents=True, exist_ok=True)
            image_path = image_dir / f"{slug(name)}.png"
            try:
                image.save(image_path)
                records[name]["image_path"] = image_path.relative_to(output).as_posix()
            except Exception as error:
                records[name]["image_error"] = str(error)
    return records


def slug(value: str) -> str:
    result = re.sub(r"[^a-z0-9]+", "_", value.lower()).strip("_")
    return result or "unnamed"


def build_indexes(env: Any) -> tuple[dict[tuple[str, int], Any], dict[int, str], dict[tuple[str, int], dict[str, Any]]]:
    objects = {(obj.assets_file.name, obj.path_id): obj for obj in env.objects}
    scripts: dict[int, str] = {}
    for obj in env.objects:
        if obj.type.name != "MonoScript":
            continue
        value = read_object(obj)
        if value:
            scripts[obj.path_id] = getattr(value, "m_ClassName", "")
    transforms: dict[tuple[str, int], dict[str, Any]] = {}
    for obj in env.objects:
        if obj.type.name == "Transform":
            tree = read_tree(obj)
            if tree:
                transforms[(obj.assets_file.name, obj.path_id)] = tree
    return objects, scripts, transforms


def component_classes(game_object: dict[str, Any], objects: dict[tuple[str, int], Any], scripts: dict[int, str], asset_file: str) -> set[str]:
    result: set[str] = set()
    for entry in game_object.get("m_Component", []):
        component_id = pointer(entry.get("component"))
        component = objects.get((asset_file, component_id))
        if not component or component.type.name != "MonoBehaviour":
            continue
        # Some IL2CPP managed-reference components have no usable type tree;
        # their MonoBehaviour header still contains the script path.
        script_id = None
        try:
            header = component.parse_monobehaviour_head()
            script_id = getattr(getattr(header, "m_Script", None), "path_id", None)
        except Exception:
            tree = read_tree(component)
            script_id = pointer(tree.get("m_Script")) if tree else None
        result.add(scripts.get(script_id, ""))
    return result


def descendants(transform_id: int, transforms: dict[tuple[str, int], dict[str, Any]], asset_file: str) -> list[tuple[int, dict[str, Any]]]:
    result: list[tuple[int, dict[str, Any]]] = []
    pending = [transform_id]
    while pending:
        current = pending.pop()
        tree = transforms.get((asset_file, current))
        if not tree:
            continue
        result.append((current, tree))
        pending.extend(pointer(child) for child in tree.get("m_Children", []) if pointer(child) is not None)
    return result


def prefab_geometry(env: Any, item_names: set[str]) -> dict[str, dict[str, Any]]:
    objects, scripts, transforms = build_indexes(env)
    records: dict[str, dict[str, Any]] = {}
    for key, obj in objects.items():
        if obj.type.name != "GameObject":
            continue
        value = read_object(obj)
        if not value or value.m_Name not in item_names:
            continue
        tree = read_tree(obj)
        if not tree:
            continue
        classes = component_classes(tree, objects, scripts, obj.assets_file.name)
        if "ItemPrefabSetup" not in classes:
            continue
        root_transform = next(
            (pointer(entry.get("component")) for entry in tree.get("m_Component", [])
             if objects.get((obj.assets_file.name, pointer(entry.get("component"))))
             and objects[(obj.assets_file.name, pointer(entry.get("component")))].type.name == "Transform"),
            None,
        )
        if root_transform is None:
            continue
        cells: list[list[int]] = []
        stars: list[list[int]] = []
        for transform_id, transform in descendants(root_transform, transforms, obj.assets_file.name):
            game_object_id = pointer(transform.get("m_GameObject"))
            child = objects.get((obj.assets_file.name, game_object_id))
            child_tree = read_tree(child) if child else None
            if not child_tree:
                continue
            child_classes = component_classes(child_tree, objects, scripts, obj.assets_file.name)
            position = transform.get("m_LocalPosition", {})
            coordinate = unity_local_to_grid(float(position.get("x", 0)), float(position.get("y", 0)))
            if "ItemPrefabSlot" in child_classes:
                cells.append(coordinate)
            if "ItemStarSlot" in child_classes:
                stars.append(coordinate)
        records.setdefault(value.m_Name, {
            "name": value.m_Name,
            "prefab_asset_file": obj.assets_file.name,
            "prefab_path_id": obj.path_id,
            "cells": sorted(cells),
            "star_positions": sorted(stars),
            "confidence": "static",
        })
    return records


def localization(env: Any) -> dict[str, Any]:
    shared: dict[int, str] = {}
    tables: dict[str, dict[str, str]] = {}
    for obj in env.objects:
        if obj.type.name != "MonoBehaviour":
            continue
        value = read_object(obj)
        if not value:
            continue
        name = getattr(value, "m_Name", "")
        tree = read_tree(obj)
        if not tree:
            continue
        if name == "Items Shared Data":
            shared = {int(entry["m_Id"]): entry["m_Key"] for entry in tree.get("m_Entries", [])}
        elif name.startswith("Items_"):
            locale = name.removeprefix("Items_")
            tables[locale] = {
                shared[entry["m_Id"]]: entry.get("m_Localized", "")
                for entry in tree.get("m_TableData", [])
                if entry.get("m_Id") in shared
            }
    item_names = sorted({name for name in shared.values() if not name.endswith(" Levels")})
    return {"shared_keys": len(shared), "item_names": item_names, "tables": tables}


def il2cpp_schema(il2cpp_dir: Path) -> dict[str, Any]:
    dump = il2cpp_dir / "dump.cs"
    if not dump.exists():
        return {"available": False}
    text = dump.read_text(encoding="utf-8", errors="replace")
    wanted = {
        "ConfigurableItemDefinition", "ItemDefinition", "ItemPrefabSetup", "ItemLevels",
        "ItemLevelsBase", "ModifyItemStat", "ItemCombinationData", "ItemCombinationEntry",
        "RecipeData", "BaseShape", "ItemStarSlot",
    }
    classes: dict[str, dict[str, Any]] = {}
    current: str | None = None
    for line in text.splitlines():
        match = re.match(r"(?:public |private |internal |abstract |sealed |static )*(?:class|interface|struct) ([^ :{]+)", line.strip())
        if match:
            current = match.group(1)
            if current in wanted:
                classes[current] = {"fields": [], "source": "il2cpp/dump.cs"}
            continue
        if current in classes:
            field = re.match(r"\s*(?:public|private|protected|internal)\s+(.+?)\s+([A-Za-z_<][A-Za-z0-9_<>]*)\s*; // (0x[0-9A-Fa-f]+)", line)
            if field:
                classes[current]["fields"].append({"type": field.group(1), "name": field.group(2), "offset": field.group(3)})
        if line.startswith("}"):
            current = None
    return {"available": True, "classes": classes}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--il2cpp", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--extra-names", type=Path, help="JSON file containing runtime item names to include")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)

    env = UnityPy.load(str(args.input))
    localized = localization(env)
    localized_item_names = set(localized["item_names"])
    item_names = set(localized_item_names)
    if args.extra_names and args.extra_names.exists():
        extra = json.loads(args.extra_names.read_text(encoding="utf-8"))
        for item in extra.get("items", []) if isinstance(extra, dict) else extra:
            for key in ("name", "client_id", "asset_id"):
                value = item.get(key) if isinstance(item, dict) else None
                if isinstance(value, str) and value:
                    item_names.add(value)
    assets = find_sprite_records(env, args.output, item_names)
    geometry = prefab_geometry(env, item_names)
    merged: dict[str, dict[str, Any]] = {}
    for name in sorted(set(assets) | set(geometry)):
        merged[name] = {**assets.get(name, {}), **geometry.get(name, {})}
    (args.output / "items").mkdir(parents=True, exist_ok=True)
    (args.output / "items" / "assets.json").write_text(json.dumps({"items": list(merged.values())}, indent=2, ensure_ascii=True), encoding="utf-8")
    (args.output / "items" / "localization.json").write_text(json.dumps(localized, indent=2, ensure_ascii=True), encoding="utf-8")
    (args.output / "schema.json").write_text(json.dumps(il2cpp_schema(args.il2cpp) if args.il2cpp else {"available": False}, indent=2, ensure_ascii=True), encoding="utf-8")

    counts = Counter(obj.type.name for obj in env.objects)
    manifest = {
        "source": str(args.source) if args.source else None,
        "source_sha256": sha256(args.source) if args.source and args.source.exists() else None,
        "unity_input": str(args.input),
        "unity_objects": len(env.objects),
        "unity_object_types": dict(counts),
        "items_in_shared_table": len(localized_item_names),
        "localized_items_in_shared_table": len(localized_item_names),
        "items_considered": len(item_names),
        "items_with_sprite": len(assets),
        "items_with_prefab_geometry": len(geometry),
        "locales": sorted(localized["tables"]),
        "confidence": {"assets": "static", "localization": "static", "stats": "runtime_required", "recipes": "runtime_required"},
    }
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps({key: manifest[key] for key in ("unity_objects", "items_in_shared_table", "items_with_sprite", "items_with_prefab_geometry")}, indent=2))


if __name__ == "__main__":
    main()

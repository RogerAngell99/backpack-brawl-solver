"""Build visual orientation metadata from canonical item geometry and sprites."""

from __future__ import annotations

import argparse
import json
import math
import struct
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def item_bounds(shape: list[list[int]]) -> tuple[int, int] | None:
    coordinates = [coordinate for coordinate in shape if isinstance(coordinate, list) and len(coordinate) == 2]
    if not coordinates:
        return None
    rows = [int(coordinate[0]) for coordinate in coordinates]
    cols = [int(coordinate[1]) for coordinate in coordinates]
    return max(rows) - min(rows) + 1, max(cols) - min(cols) + 1


def png_dimensions(path: Path) -> tuple[int, int] | None:
    try:
        payload = path.read_bytes()
    except OSError:
        return None
    if len(payload) < 24 or payload[:8] != b"\x89PNG\r\n\x1a\n" or payload[12:16] != b"IHDR":
        return None
    width, height = struct.unpack(">II", payload[16:24])
    return width, height


def sprite_dimensions(asset: dict[str, Any], image_path: Path) -> tuple[float, float] | None:
    rect = asset.get("rect") if isinstance(asset, dict) else None
    if isinstance(rect, dict):
        width = rect.get("width")
        height = rect.get("height")
        if isinstance(width, (int, float)) and isinstance(height, (int, float)) and width > 0 and height > 0:
            return float(width), float(height)
    dimensions = png_dimensions(image_path)
    return tuple(float(value) for value in dimensions) if dimensions else None


def infer_base_rotation(shape: list[list[int]], sprite: tuple[float, float] | None) -> int:
    bounds = item_bounds(shape)
    if bounds is None or sprite is None:
        return 0
    rows, cols = bounds
    if rows == cols:
        return 0
    shape_ratio = cols / rows
    sprite_ratio = sprite[0] / sprite[1]
    direct_error = abs(math.log(sprite_ratio / shape_ratio))
    transposed_error = abs(math.log(sprite_ratio * shape_ratio))
    return 90 if transposed_error + 0.15 < direct_error else 0


def build_metadata(catalog: dict[str, Any], normalized: dict[str, Any], assets: dict[str, Any], asset_root: Path) -> dict[str, dict[str, int]]:
    normalized_by_id = {str(item.get("id")): item for item in normalized.get("items", []) if item.get("id")}
    assets_by_name = {str(item.get("name")): item for item in assets.get("items", []) if item.get("name")}
    result: dict[str, dict[str, int]] = {}
    for item in catalog.get("items", []):
        item_id = str(item.get("id") or "")
        if not item_id:
            continue
        normalized_item = normalized_by_id.get(item_id, {})
        asset_name = str(normalized_item.get("asset_id") or normalized_item.get("name") or item.get("name") or "")
        image_path = str(item.get("image_path") or "")
        sprite = sprite_dimensions(assets_by_name.get(asset_name, {}), asset_root / image_path)
        rotation = infer_base_rotation(item.get("shape") or [], sprite)
        if rotation:
            result[item_id] = {"base_rotation": rotation}
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--catalog", type=Path, required=True)
    parser.add_argument("--normalized", type=Path, required=True)
    parser.add_argument("--assets", type=Path, required=True)
    parser.add_argument("--asset-root", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    metadata = build_metadata(
        load_json(args.catalog),
        load_json(args.normalized),
        load_json(args.assets),
        args.asset_root,
    )
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(metadata, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
    print(json.dumps({"items_with_base_rotation": len(metadata)}, indent=2))


if __name__ == "__main__":
    main()

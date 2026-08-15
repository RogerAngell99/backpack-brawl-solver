"""Render catalog/runtime geometry conflicts as readable Markdown matrices."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def coordinates(value: Any) -> list[tuple[int, int]]:
    result: list[tuple[int, int]] = []
    for coordinate in value or []:
        if isinstance(coordinate, list) and len(coordinate) == 2:
            result.append((int(round(coordinate[0])), int(round(coordinate[1]))))
    return result


def draw_matrix(
    shape: list[tuple[int, int]],
    stars: list[tuple[int, int]],
    bounds: tuple[int, int, int, int] | None = None,
) -> str:
    occupied = set(shape)
    star_cells = set(stars)
    all_cells = occupied | star_cells
    if not all_cells and bounds is None:
        return "(empty)"

    if bounds is None:
        min_row = min(row for row, _ in all_cells)
        max_row = max(row for row, _ in all_cells)
        min_col = min(col for _, col in all_cells)
        max_col = max(col for _, col in all_cells)
    else:
        min_row, max_row, min_col, max_col = bounds
    column_width = max(2, max(len(str(col)) for col in range(min_col, max_col + 1)))
    row_width = max(2, max(len(str(row)) for row in range(min_row, max_row + 1)))
    header = " " * (row_width + 2) + " ".join(f"{col:>{column_width}}" for col in range(min_col, max_col + 1))
    rows = [header]
    for row in range(min_row, max_row + 1):
        cells = []
        for col in range(min_col, max_col + 1):
            coordinate = (row, col)
            if coordinate in occupied and coordinate in star_cells:
                marker = "@"
            elif coordinate in occupied:
                marker = "#"
            elif coordinate in star_cells:
                marker = "*"
            else:
                marker = "."
            cells.append(f"{marker:>{column_width}}")
        rows.append(f"{row:>{row_width}}  " + " ".join(cells))
    return "\n".join(rows)


def format_coordinates(value: list[tuple[int, int]]) -> str:
    return ", ".join(f"[{row}, {col}]" for row, col in value) or "none"


def format_counts(value: list[tuple[int, int]]) -> str:
    duplicates = [(coordinate, count) for coordinate, count in Counter(value).items() if count > 1]
    if not duplicates:
        return ""
    return " Duplicates: " + ", ".join(f"{coordinate} x{count}" for coordinate, count in duplicates) + "."


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--curated", type=Path, required=True)
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    report = load_json(args.report)
    curated = {item["id"]: item for item in load_json(args.curated).get("items", [])}
    runtime = {item["id"]: item for item in load_json(args.runtime).get("items", [])}
    conflicts_by_item: dict[str, list[dict[str, Any]]] = {}
    for conflict in report.get("conflicts", []):
        conflicts_by_item.setdefault(conflict["item_id"], []).append(conflict)

    sections = [
        "# Geometry Conflict Review",
        "",
        "Legend: `#` item cell, `*` star position, `@` item and star overlap, `.` empty cell.",
        "Rows are the first coordinate and columns are the second coordinate.",
        "The matrices use the same bounding box for curated and runtime data.",
        "",
        f"Remaining conflicts: {len(report.get('conflicts', []))}",
        "",
    ]

    for item_id in sorted(conflicts_by_item):
        curated_item = curated.get(item_id, {})
        runtime_item = runtime.get(item_id, {})
        curated_shape = coordinates(curated_item.get("shape"))
        runtime_shape = coordinates(runtime_item.get("shape"))
        curated_stars = coordinates([star.get("offset") for star in curated_item.get("stars", [])])
        runtime_stars = coordinates(runtime_item.get("star_positions"))
        all_coordinates = curated_shape + runtime_shape + curated_stars + runtime_stars
        bounds = (
            min(row for row, _ in all_coordinates),
            max(row for row, _ in all_coordinates),
            min(col for _, col in all_coordinates),
            max(col for _, col in all_coordinates),
        )
        names = curated_item.get("name") or runtime_item.get("name") or item_id
        fields = ", ".join(sorted({conflict["field"] for conflict in conflicts_by_item[item_id]}))

        sections.extend(
            [
                f"## {names} (`{item_id}`)",
                "",
                f"Conflicting field(s): `{fields}`",
                "",
                "### Curated",
                "",
                "```text",
                draw_matrix(curated_shape, curated_stars, bounds),
                "```",
                f"Shape: `{format_coordinates(curated_shape)}`",
                f"Star positions: `{format_coordinates(curated_stars)}`{format_counts(curated_stars)}",
                "",
                "### Runtime",
                "",
                "```text",
                draw_matrix(runtime_shape, runtime_stars, bounds),
                "```",
                f"Shape: `{format_coordinates(runtime_shape)}`",
                f"Star positions: `{format_coordinates(runtime_stars)}`{format_counts(runtime_stars)}",
                "",
            ]
        )

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text("\n".join(sections) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()

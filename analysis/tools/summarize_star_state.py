"""Summarize runtime star-condition observations and UI agreement."""

from __future__ import annotations

import argparse
import json
import re
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def graph_classes(value: Any) -> set[str]:
    if not isinstance(value, dict):
        return set()
    result = {value["class"]} if isinstance(value.get("class"), str) else set()
    for child in value.get("conditions", []):
        result.update(graph_classes(child))
    return result


def point_key(value: Any) -> str | None:
    if not isinstance(value, dict):
        return None
    if not isinstance(value.get("x"), (int, float)) or not isinstance(value.get("y"), (int, float)):
        return None
    return f"{value['x']},{value['y']}"


def item_key(value: Any) -> str:
    return re.sub(r"[^a-z0-9]+", "_", str(value).lower()).strip("_")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--metadata", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    runtime = load_json(args.runtime)
    metadata: dict[str, dict[str, Any]] = {}
    for item in load_json(args.metadata).get("items", []):
        for field in ("catalog_id", "runtime_id", "client_id", "name"):
            if item.get(field):
                metadata[item_key(item[field])] = item
    star_state = runtime.get("star_state") or {}
    observations = [entry.get("observation", {}) for entry in star_state.get("observations", [])]
    snapshots = [entry for entry in star_state.get("visual_snapshots", [])]
    inventory_snapshots = [entry for entry in star_state.get("inventory_snapshots", [])]
    condition_observations = [entry for entry in star_state.get("condition_observations", [])]
    direct_condition_observations = [
        entry for entry in condition_observations if entry.get("source") == "direct_probe"
    ]

    visual_states: dict[tuple[str, str], list[str]] = defaultdict(list)
    for entry in snapshots:
        for updater in entry.get("updaters", []):
            source_id = (updater.get("item") or {}).get("id")
            for star in updater.get("stars", []):
                position = point_key(star.get("position"))
                if source_id and position:
                    visual_states[(source_id, position)].append(star.get("state", "unknown"))

    by_source: dict[str, dict[str, Any]] = {}
    by_condition: dict[str, Counter[str]] = defaultdict(Counter)
    direct_by_condition: dict[str, Counter[str]] = defaultdict(Counter)
    direct_contexts: dict[tuple[str, str, str], set[str]] = defaultdict(set)
    mismatches: list[dict[str, Any]] = []
    context_keys: set[str] = set()
    for observation in observations:
        source = observation.get("source") or observation.get("item2") or {}
        source_id = source.get("id") or "unknown"
        position = point_key(observation.get("position"))
        result = bool(observation.get("result"))
        entry = by_source.setdefault(source_id, {"observations": 0, "true": 0, "false": 0, "positions": set()})
        entry["observations"] += 1
        entry["true" if result else "false"] += 1
        if position:
            entry["positions"].add(position)
        context_keys.add(json.dumps({
            "source": source_id,
            "position": position,
            "other": observation.get("other_item"),
            "others": observation.get("other_items"),
            "players": observation.get("player_items"),
        }, sort_keys=True, ensure_ascii=True))
        metadata_item = metadata.get(item_key(source_id)) or metadata.get(item_key(source.get("name", ""))) or {}
        graph = metadata_item.get("star_condition_graph")
        for condition_class in graph_classes(graph) or {"unknown"}:
            by_condition[condition_class]["true" if result else "false"] += 1
        visual = visual_states.get((source_id, position), []) if position else []
        if visual:
            expected = "has_effect" if result else "no_effect"
            if expected not in visual:
                mismatches.append({
                    "source_id": source_id,
                    "position": observation.get("position"),
                    "result": result,
                    "visual_states": sorted(set(visual)),
                })

    for condition_observation in direct_condition_observations:
        condition_class = condition_observation.get("condition") or "unknown"
        result = "true" if condition_observation.get("result") else "false"
        direct_by_condition[condition_class][result] += 1
        parameters = condition_observation.get("parameters") or []
        source = parameters[0] if len(parameters) > 0 else None
        target = parameters[1] if len(parameters) > 1 else None
        source_id = source.get("id") if isinstance(source, dict) else "unknown"
        target_id = target.get("id") if isinstance(target, dict) else "unknown"
        direct_contexts[(condition_class, source_id or "unknown", target_id or "unknown")].add(result)

    for entry in by_source.values():
        entry["positions"] = sorted(entry["positions"])

    report = {
        "runtime": str(args.runtime),
        "observation_count": len(observations),
        "visual_snapshot_count": len(snapshots),
        "inventory_snapshot_count": len(inventory_snapshots),
        "latest_inventory_snapshot": inventory_snapshots[-1] if inventory_snapshots else None,
        "unique_context_count": len(context_keys),
        "source_count": len(by_source),
        "result_counts": dict(Counter("true" if entry.get("result") else "false" for entry in observations)),
        "condition_coverage": {name: dict(counts) for name, counts in sorted(by_condition.items())},
        "condition_observation_count": len(condition_observations),
        "direct_condition_observation_count": len(direct_condition_observations),
        "direct_condition_coverage": {
            name: {
                **dict(counts),
                "contexts": [
                    {"source": source, "target": target, "results": sorted(results)}
                    for (condition, source, target), results in sorted(direct_contexts.items())
                    if condition == name
                ],
            }
            for name, counts in sorted(direct_by_condition.items())
        },
        "ui_mismatch_count": len(mismatches),
        "ui_mismatches": mismatches,
        "sources": dict(sorted(by_source.items())),
        "errors": [entry.get("error") for entry in star_state.get("errors", [])],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
    print(json.dumps({key: report[key] for key in ("observation_count", "source_count", "unique_context_count", "ui_mismatch_count")}, indent=2))


if __name__ == "__main__":
    main()

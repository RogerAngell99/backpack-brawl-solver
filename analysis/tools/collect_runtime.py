"""Capture materialized item definitions from the installed Android client."""

from __future__ import annotations

import argparse
import json
import subprocess
import time
from pathlib import Path

import frida


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", default="com.rapidfiregames.backpackbrawl")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--wait", type=int, default=40)
    parser.add_argument(
        "--bridge",
        type=Path,
        default=Path.home() / "OneDrive" / "Documents" / "node_modules" / "frida-il2cpp-bridge" / "dist" / "index.js",
    )
    args = parser.parse_args()
    script_path = Path(__file__).with_name("runtime_collect.js")
    source = args.bridge.read_text(encoding="utf-8") + "\n" + script_path.read_text(encoding="utf-8")
    args.output.mkdir(parents=True, exist_ok=True)

    device = frida.get_usb_device(timeout=10)
    spawned = True
    try:
        pid = device.spawn([args.package])
    except frida.NotSupportedError:
        # Some Android builds reject Frida spawn while allowing attach.
        spawned = False
        subprocess.run(["adb", "shell", "monkey", "-p", args.package, "1"], check=True, capture_output=True)
        pid = None
        deadline = time.time() + 15
        while time.time() < deadline and pid is None:
            process = next(
                (item for item in device.enumerate_processes()
                 if item.name.lower() in {args.package.lower(), "backpack brawl"}),
                None,
            )
            pid = process.pid if process else None
            time.sleep(0.5)
        if pid is None:
            raise RuntimeError(f"processo nao encontrado: {args.package}")
    session = device.attach(pid)
    received: list[dict] = []

    def on_message(message: dict, _data: bytes | None) -> None:
        if message.get("type") == "send":
            payload = message.get("payload", {})
            received.append(payload)
            print(json.dumps(payload, ensure_ascii=True)[:400])
        elif message.get("type") == "error":
            received.append({"type": "frida_error", "error": message.get("stack", message)})
            print(json.dumps(received[-1], ensure_ascii=True))

    script = session.create_script(source)
    script.on("message", on_message)
    script.load()
    if spawned:
        device.resume(pid)
    time.sleep(args.wait)
    session.detach()

    data_messages = [entry for entry in received if entry.get("type") in {"runtime_data", "runtime_chunk"}]
    chunks = {}
    recipes = []
    total = 0
    for entry in data_messages:
        data = entry.get("data", {})
        chunk = data.get("chunk")
        if chunk:
            chunks[chunk.get("start", 0)] = data.get("items", [])
            total = max(total, chunk.get("total", 0))
            if data.get("recipes"):
                recipes = data["recipes"]
    best = max(data_messages, key=lambda entry: entry.get("data", {}).get("item_count", 0), default=None)
    if chunks:
        merged = []
        seen_ids = set()
        for start in sorted(chunks):
            for item in chunks[start]:
                if item.get("id") in seen_ids:
                    continue
                seen_ids.add(item.get("id"))
                merged.append(item)
        best = {"data": {"items": merged, "recipes": recipes, "item_count": total, "chunks": len(chunks)}}
    result = {
        "package": args.package,
        "wait_seconds": args.wait,
        "messages": len(received),
        "attempts": [{"type": entry.get("type"), "item_count": entry.get("data", {}).get("item_count")} for entry in received],
        "data": best.get("data") if best else None,
    }
    (args.output / "runtime.json").write_text(json.dumps(result, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps({"item_count": result["data"].get("item_count", 0) if result["data"] else 0, "output": str(args.output / "runtime.json")}, indent=2))


if __name__ == "__main__":
    main()

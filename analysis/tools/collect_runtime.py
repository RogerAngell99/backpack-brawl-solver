"""Capture materialized item definitions from the installed Android client."""

from __future__ import annotations

import argparse
import json
import subprocess
import time
from pathlib import Path

import frida


def process_snapshot(device: frida.core.Device, package: str) -> list[dict[str, object]]:
    processes = []
    for process in device.enumerate_processes():
        identifier = getattr(process, "identifier", None)
        if identifier == package or process.name.lower() == "backpack brawl":
            processes.append({"pid": process.pid, "name": process.name, "identifier": identifier})
    return processes


def start_logcat(output: Path) -> tuple[subprocess.Popen[str], object]:
    handle = output.open("w", encoding="utf-8", errors="replace")
    process = subprocess.Popen(
        ["adb", "logcat", "-v", "threadtime"],
        stdout=handle,
        stderr=subprocess.STDOUT,
        text=True,
    )
    return process, handle


def stop_logcat(process: subprocess.Popen[str] | None, handle: object | None) -> None:
    if process is not None:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
    if handle is not None:
        handle.close()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", default="com.rapidfiregames.backpackbrawl")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--wait", type=int, default=40)
    parser.add_argument("--mode", choices=["auto", "spawn", "attach"], default="auto")
    parser.add_argument(
        "--profile",
        choices=["minimal", "enumerate", "items", "stars", "recipes", "hero-scope", "star-state", "full"],
        default="full",
    )
    parser.add_argument(
        "--bridge",
        type=Path,
        default=Path.home() / "OneDrive" / "Documents" / "node_modules" / "frida-il2cpp-bridge" / "dist" / "index.js",
    )
    args = parser.parse_args()
    script_path = Path(__file__).with_name("runtime_collect.js")
    source = (
        f"globalThis.RUNTIME_CAPTURE_PROFILE = {json.dumps(args.profile)};\n"
        + args.bridge.read_text(encoding="utf-8")
        + "\n"
        + script_path.read_text(encoding="utf-8")
    )
    args.output.mkdir(parents=True, exist_ok=True)

    device = frida.get_usb_device(timeout=10)
    started_at = time.time()
    logcat_process: subprocess.Popen[str] | None = None
    logcat_handle: object | None = None
    session = None
    script = None
    spawned = False
    pid = None
    received: list[dict] = []
    pid_history: list[dict[str, object]] = []
    lifecycle_errors: list[str] = []

    def snapshot(label: str) -> list[dict[str, object]]:
        value = {"at": time.time(), "label": label, "processes": process_snapshot(device, args.package)}
        pid_history.append(value)
        return value["processes"]

    def on_message(message: dict, _data: bytes | None) -> None:
        if message.get("type") == "send":
            payload = dict(message.get("payload", {}))
            payload["received_at"] = time.time()
            payload["pid"] = pid
            received.append(payload)
            print(json.dumps(payload, ensure_ascii=True)[:800])
        elif message.get("type") == "error":
            error = {"type": "frida_error", "error": message.get("stack", message), "received_at": time.time()}
            error["pid"] = pid
            received.append(error)
            print(json.dumps(received[-1], ensure_ascii=True))

    try:
        logcat_process, logcat_handle = start_logcat(args.output / "logcat.txt")
        snapshot("before_launch")
        if args.mode != "attach":
            try:
                pid = device.spawn([args.package])
                spawned = True
            except frida.NotSupportedError:
                if args.mode == "spawn":
                    raise
        if not spawned:
            subprocess.run(["adb", "shell", "monkey", "-p", args.package, "1"], check=True, capture_output=True)
            deadline = time.time() + 15
            while time.time() < deadline and pid is None:
                processes = process_snapshot(device, args.package)
                pid = processes[0]["pid"] if processes else None
                if pid is None:
                    time.sleep(0.5)
            if pid is None:
                raise RuntimeError(f"processo nao encontrado: {args.package}")
        snapshot("before_attach")
        session = device.attach(pid)
        snapshot("after_attach")
        script = session.create_script(source)
        script.on("message", on_message)
        script.load()
        snapshot("after_script_load")
        if spawned:
            device.resume(pid)
            snapshot("after_resume")
        time.sleep(args.wait)
    except Exception as error:
        lifecycle_errors.append(f"{type(error).__name__}: {error}")
        print(json.dumps({"type": "collector_error", "error": lifecycle_errors[-1]}, ensure_ascii=True))
    finally:
        snapshot("before_detach")
        if session is not None:
            try:
                session.detach()
            except Exception as error:
                lifecycle_errors.append(f"detach {type(error).__name__}: {error}")
        snapshot("after_detach")
        stop_logcat(logcat_process, logcat_handle)

    data_messages = [entry for entry in received if entry.get("type") in {"runtime_data", "runtime_chunk"}]
    star_observations = [entry for entry in received if entry.get("type") == "star_observation"]
    star_condition_methods = [entry for entry in received if entry.get("type") == "star_condition_methods"]
    star_condition_observations = [entry for entry in received if entry.get("type") == "star_condition_observation"]
    star_condition_probe_debug = [entry for entry in received if entry.get("type") == "star_condition_probe_debug"]
    star_visual_snapshots = [entry for entry in received if entry.get("type") == "star_visual_snapshot"]
    star_state_errors = [entry for entry in received if entry.get("type") == "star_state_error"]
    inventory_snapshots = [entry for entry in received if entry.get("type") == "inventory_snapshot"]
    chunks = {}
    recipes = []
    heroes = []
    total = 0
    for entry in data_messages:
        data = entry.get("data", {})
        if data.get("heroes"):
            heroes = data["heroes"]
        chunk = data.get("chunk")
        if chunk:
            chunks[chunk.get("start", 0)] = data.get("items", [])
            total = max(total, chunk.get("total", 0))
            if data.get("recipes"):
                recipes = data["recipes"]
    best = max(data_messages, key=lambda entry: entry.get("data", {}).get("item_count") or 0, default=None)
    if chunks:
        merged = []
        seen_ids = set()
        for start in sorted(chunks):
            for item in chunks[start]:
                if item.get("id") in seen_ids:
                    continue
                seen_ids.add(item.get("id"))
                merged.append(item)
        best = {"data": {"items": merged, "recipes": recipes, "heroes": heroes, "item_count": total, "chunks": len(chunks)}}
    result = {
        "package": args.package,
        "wait_seconds": args.wait,
        "requested_mode": args.mode,
        "profile": args.profile,
        "actual_mode": "spawn" if spawned else "attach",
        "initial_pid": pid,
        "started_at": started_at,
        "finished_at": time.time(),
        "messages": len(received),
        "attempts": [
            {
                "type": entry.get("type"),
                "item_count": entry.get("data", {}).get("item_count"),
                "chunk": entry.get("data", {}).get("chunk"),
                "error": entry.get("error"),
                "received_at": entry.get("received_at"),
            }
            for entry in received
        ],
        "errors": [entry.get("error") for entry in received if entry.get("type") in {"runtime_error", "frida_error"}]
        + lifecycle_errors,
        "pid_history": pid_history,
        "logcat": str(args.output / "logcat.txt"),
        "data": best.get("data") if best else None,
        "star_state": {
            "observations": star_observations,
            "visual_snapshots": star_visual_snapshots,
            "errors": star_state_errors,
            "condition_methods": star_condition_methods,
            "condition_observations": star_condition_observations,
            "condition_probe_debug": star_condition_probe_debug,
            "inventory_snapshots": inventory_snapshots,
        },
    }
    (args.output / "runtime.json").write_text(json.dumps(result, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps({"item_count": result["data"].get("item_count", 0) if result["data"] else 0, "output": str(args.output / "runtime.json")}, indent=2))


if __name__ == "__main__":
    main()

import { readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const webRoot = join(fileURLToPath(new URL("..", import.meta.url)));
const wasmDir = join(webRoot, "public", "wasm");
const wasmPath = join(wasmDir, "solver.wasm");
const execPath = join(wasmDir, "wasm_exec.js");

for (const path of [wasmPath, execPath]) {
  const size = statSync(path).size;
  if (size === 0) {
    throw new Error(`WASM asset is empty: ${path}`);
  }
}

const wasmHeader = readFileSync(wasmPath).subarray(0, 4);
if (!wasmHeader.equals(Buffer.from([0x00, 0x61, 0x73, 0x6d]))) {
  throw new Error(`Invalid WebAssembly header: ${wasmPath}`);
}

console.log(`WASM assets verified (${statSync(wasmPath).size} bytes)`);

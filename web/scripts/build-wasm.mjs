import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const webRoot = dirname(fileURLToPath(import.meta.url));
const projectRoot = join(webRoot, "..", "..");
const wasmDir = join(webRoot, "..", "public", "wasm");

mkdirSync(wasmDir, { recursive: true });

const committedWasm = join(wasmDir, "solver.wasm");
const committedExec = join(wasmDir, "wasm_exec.js");
const goEnv = spawnSync("go", ["env", "GOROOT"], {
  cwd: projectRoot,
  encoding: "utf8",
});
if (goEnv.error) {
  if (goEnv.error.code === "ENOENT" && existsSync(committedWasm) && existsSync(committedExec)) {
    console.warn("Go is unavailable; using checked-in WASM assets");
    process.exit(0);
  }
  throw goEnv.error;
}
if (goEnv.status !== 0) {
  process.stderr.write(goEnv.stderr || "go env GOROOT failed\n");
  process.exit(goEnv.status || 1);
}

const goRoot = goEnv.stdout.trim();
const wasmExecCandidates = [
  join(goRoot, "misc", "wasm", "wasm_exec.js"),
  join(goRoot, "lib", "wasm", "wasm_exec.js"),
];
const wasmExec = wasmExecCandidates.find((candidate) => existsSync(candidate));
if (!wasmExec) {
  throw new Error(`wasm_exec.js not found under ${goRoot}`);
}
copyFileSync(wasmExec, join(wasmDir, "wasm_exec.js"));

const build = spawnSync(
  "go",
  ["build", "-o", join("web", "public", "wasm", "solver.wasm"), "./cmd/wasm-solver"],
  {
    cwd: projectRoot,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  },
);

if (build.status !== 0) {
  process.exit(build.status ?? 1);
}

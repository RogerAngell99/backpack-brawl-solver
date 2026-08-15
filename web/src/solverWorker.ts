import type { Catalog, Scenario, Solution, SolveProgress } from "./types";

declare global {
  interface WorkerGlobalScope {
    Go?: new () => {
      importObject: WebAssembly.Imports;
      run(instance: WebAssembly.Instance): Promise<void>;
    };
    solveScenario?: (input: string, progressCallback?: (progress: SolveProgress) => void) => string;
  }
}

interface SolveRequest {
  type: "solve";
  requestId: number;
  catalog: Catalog;
  scenario: Scenario;
}

type WorkerRequest = SolveRequest;

let loadPromise: Promise<void> | null = null;

self.onmessage = (event: MessageEvent<WorkerRequest>) => {
  if (event.data.type !== "solve") {
    return;
  }
  void runSolve(event.data);
};

async function runSolve(request: SolveRequest): Promise<void> {
  try {
    await loadWasmSolver();
    if (!self.solveScenario) {
      throw new Error("WASM solver did not initialize");
    }

    const output = self.solveScenario(JSON.stringify({ catalog: request.catalog, scenario: request.scenario }), (progress) => {
      self.postMessage({
        type: "progress",
        requestId: request.requestId,
        progress: normalizeProgress(progress),
      });
    });
    if (typeof output !== "string") {
      throw new Error("WASM solver aborted before returning a result. Try a lower Max nodes value or fewer enabled coverage sources.");
    }

    const parsed = JSON.parse(output) as Solution[] | { error?: string };
    if (!Array.isArray(parsed)) {
      throw new Error(parsed.error || "Solver returned an unknown error");
    }
    self.postMessage({ type: "result", requestId: request.requestId, solutions: parsed });
  } catch (error) {
    self.postMessage({
      type: "error",
      requestId: request.requestId,
      error: error instanceof Error ? error.message : "Solver failed",
    });
  }
}

async function loadWasmSolver(): Promise<void> {
  if (self.solveScenario) {
    return;
  }
  if (!loadPromise) {
    loadPromise = loadWasmSolverOnce();
  }
  return loadPromise;
}

async function loadWasmSolverOnce(): Promise<void> {
  await loadWasmExecScript();
  if (!self.Go) {
    throw new Error("Go WASM runtime is unavailable");
  }

  const go = new self.Go();
  const result = await instantiateSolver(go.importObject);
  void go.run(result.instance);

  if (!self.solveScenario) {
    throw new Error("solveScenario was not registered by the WASM module");
  }
}

async function loadWasmExecScript(): Promise<void> {
  const response = await fetch("/wasm/wasm_exec.js");
  if (!response.ok) {
    throw new Error("Failed to load /wasm/wasm_exec.js");
  }
  const source = await response.text();
  (0, eval)(source);
}

async function instantiateSolver(importObject: WebAssembly.Imports): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
  try {
    return await WebAssembly.instantiateStreaming(fetch("/wasm/solver.wasm"), importObject);
  } catch {
    const response = await fetch("/wasm/solver.wasm");
    const bytes = await response.arrayBuffer();
    return WebAssembly.instantiate(bytes, importObject);
  }
}

function normalizeProgress(progress: SolveProgress): SolveProgress {
  const normalized: SolveProgress = {
    phase: progress.phase,
    nodes_explored: Number(progress.nodes_explored) || 0,
    elapsed_ms: Number(progress.elapsed_ms) || 0,
  };
  if (typeof progress.nodes_total === "number" && progress.nodes_total > 0) {
    normalized.nodes_total = progress.nodes_total;
  }
  if (typeof progress.percent === "number" && Number.isFinite(progress.percent)) {
    normalized.percent = Math.max(0, Math.min(100, progress.percent));
  }
  if (typeof progress.nodes_per_second === "number" && progress.nodes_per_second > 0) {
    normalized.nodes_per_second = progress.nodes_per_second;
  }
  if (typeof progress.eta_ms === "number" && progress.eta_ms > 0) {
    normalized.eta_ms = progress.eta_ms;
  }
  if (Array.isArray(progress.partial_solutions) && progress.partial_solutions.length > 0) {
    normalized.partial_solutions = progress.partial_solutions;
  }
  return normalized;
}

export {};

import type { Scenario, Solution, SolveProgress } from "./types";

declare global {
  interface WorkerGlobalScope {
    Go?: new () => {
      importObject: WebAssembly.Imports;
      run(instance: WebAssembly.Instance): Promise<void>;
    };
    solveScenario?: (input: string, progressCallback?: (progress: SolveProgress) => void) => string;
    installCatalog?: (input: string) => string;
    solvePreparedScenario?: (input: string, progressCallback?: (progress: SolveProgress) => void) => string;
  }
}

interface CatalogKey {
  catalogIdentity: number;
  catalogRevision: number;
}

interface InstallCatalogRequest extends CatalogKey {
  type: "installCatalog";
  requestId: number;
  catalogJSON: string;
}

interface SolveRequest {
  type: "solve";
  requestId: number;
  catalogIdentity: number;
  catalogRevision: number;
  scenario: Scenario;
}

type WorkerRequest = InstallCatalogRequest | SolveRequest;

let loadPromise: Promise<void> | null = null;
let installedCatalog: CatalogKey | null = null;
let activeRequestId: number | null = null;
const WASM_STARTUP_TIMEOUT_MS = 10_000;

self.onmessage = (event: MessageEvent<WorkerRequest>) => {
  const request = event.data;
  if (!request || (request.type !== "installCatalog" && request.type !== "solve")) {
    return;
  }
  if (activeRequestId !== null) {
    self.postMessage({
      type: "error",
      requestId: request.requestId,
      error: "Solver worker accepts one request at a time",
    });
    return;
  }
  activeRequestId = request.requestId;
  if (request.type === "installCatalog") {
    void installPreparedCatalog(request);
    return;
  }
  void runSolve(request);
};

async function installPreparedCatalog(request: InstallCatalogRequest): Promise<void> {
  try {
    await loadWasmSolver();
    if (!catalogMatches(installedCatalog, request)) {
      const installCatalog = self.installCatalog;
      if (!installCatalog) {
        throw new Error("WASM catalog installer did not initialize");
      }
      const output = installCatalog(request.catalogJSON);
      if (typeof output !== "string") {
        throw new Error("WASM catalog installer aborted before returning a result");
      }
      const parsed = JSON.parse(output) as { ok?: boolean; error?: string };
      if (!parsed.ok) {
        throw new Error(parsed.error || "WASM catalog installer returned an unknown error");
      }
      installedCatalog = {
        catalogIdentity: request.catalogIdentity,
        catalogRevision: request.catalogRevision,
      };
    }
    self.postMessage({
      type: "catalogInstalled",
      requestId: request.requestId,
      catalogIdentity: request.catalogIdentity,
      catalogRevision: request.catalogRevision,
    });
  } catch (error) {
    reportError(request.requestId, error);
  } finally {
    activeRequestId = null;
  }
}

async function runSolve(request: SolveRequest): Promise<void> {
  try {
    await loadWasmSolver();
    if (!catalogMatches(installedCatalog, request)) {
      throw new Error("Catalog is not installed for this solver worker");
    }
    const solvePreparedScenario = self.solvePreparedScenario as
      | ((input: string, progressCallback?: (progress: SolveProgress) => void) => string)
      | undefined;
    if (!solvePreparedScenario) {
      throw new Error("WASM solver did not initialize");
    }

    const output = solvePreparedScenario(JSON.stringify(request.scenario), (progress) => {
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
    reportError(request.requestId, error);
  } finally {
    activeRequestId = null;
  }
}

function catalogMatches(catalog: CatalogKey | null, request: CatalogKey): boolean {
  return (
    catalog !== null &&
    catalog.catalogIdentity === request.catalogIdentity &&
    catalog.catalogRevision === request.catalogRevision
  );
}

function reportError(requestId: number, error: unknown): void {
  self.postMessage({
    type: "error",
    requestId,
    error: error instanceof Error ? error.message : "Solver failed",
  });
}

async function loadWasmSolver(): Promise<void> {
  if (self.solveScenario) {
    return;
  }
  if (!loadPromise) {
    loadPromise = withTimeout(loadWasmSolverOnce(), WASM_STARTUP_TIMEOUT_MS).catch((error) => {
      loadPromise = null;
      throw error;
    });
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
  const runPromise = go.run(result.instance);
  await Promise.race([
    waitForSolverRegistration(),
    runPromise.then(() => {
      throw new Error("WASM module exited before registering solveScenario");
    }),
  ]);
}

function waitForSolverRegistration(): Promise<void> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + WASM_STARTUP_TIMEOUT_MS;
    const check = () => {
      if (self.solveScenario) {
        resolve();
        return;
      }
      if (Date.now() >= deadline) {
        reject(new Error(`WASM solver did not initialize within ${WASM_STARTUP_TIMEOUT_MS / 1000} seconds`));
        return;
      }
      setTimeout(check, 10);
    };
    check();
  });
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error(`WASM solver did not initialize within ${timeoutMs / 1000} seconds`));
    }, timeoutMs);
  });
  return Promise.race([promise, timeout]).finally(() => {
    if (timer !== undefined) {
      clearTimeout(timer);
    }
  });
}

async function loadWasmExecScript(): Promise<void> {
  const response = await fetch("/wasm/wasm_exec.js");
  if (!response.ok) {
    throw new Error(`Failed to load /wasm/wasm_exec.js (${response.status} ${response.statusText})`);
  }
  const source = await response.text();
  (0, eval)(source);
}

async function instantiateSolver(importObject: WebAssembly.Imports): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
  try {
    const response = await fetch("/wasm/solver.wasm");
    if (!response.ok) {
      throw new Error(`Failed to load /wasm/solver.wasm (${response.status} ${response.statusText})`);
    }
    return await WebAssembly.instantiateStreaming(Promise.resolve(response), importObject);
  } catch (streamingError) {
    try {
      const response = await fetch("/wasm/solver.wasm");
      if (!response.ok) {
        throw new Error(`Failed to load /wasm/solver.wasm (${response.status} ${response.statusText})`);
      }
      const bytes = await response.arrayBuffer();
      return await WebAssembly.instantiate(bytes, importObject);
    } catch (fallbackError) {
      const message = fallbackError instanceof Error ? fallbackError.message : String(fallbackError);
      const original = streamingError instanceof Error ? streamingError.message : String(streamingError);
      throw new Error(`${message}; streaming attempt: ${original}`);
    }
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

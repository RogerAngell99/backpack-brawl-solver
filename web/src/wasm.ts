import type { Catalog, RemoteSolveMetadata, Scenario, Solution, SolveProgress } from "./types";

declare global {
  interface Window {
    Go?: new () => {
      importObject: WebAssembly.Imports;
      run(instance: WebAssembly.Instance): Promise<void>;
    };
    solveScenario?: (input: string) => string;
    installCatalog?: (input: string) => string;
    solvePreparedScenario?: (input: string) => string;
  }
}

let loadPromise: Promise<void> | null = null;
let solverWorker: Worker | null = null;
let nextWorkerRequestId = 1;
let activeWorkerRequestId: number | null = null;
let installedWorkerCatalog: CatalogKey | null = null;
let nextCatalogIdentity = 1;
let nextCatalogRevision = 1;
let installedDirectCatalog: CatalogKey | null = null;
const catalogIdentities = new WeakMap<Catalog, number>();
const catalogVersions = new WeakMap<Catalog, CatalogVersion>();

interface CatalogKey {
  catalogIdentity: number;
  catalogRevision: number;
}

interface CatalogVersion extends CatalogKey {
  catalogJSON: string;
}

type WorkerMessage =
  | ({ type: "catalogInstalled"; requestId: number } & CatalogKey)
  | { type: "progress"; requestId: number; progress: SolveProgress }
  | { type: "result"; requestId: number; solutions: Solution[] }
  | { type: "error"; requestId: number; error: string };

export interface RemoteSolveResult {
  solutions: Solution[];
  metadata: RemoteSolveMetadata;
}

export class RemoteSolveError extends Error {
  fallbackAllowed: boolean;

  constructor(message: string, fallbackAllowed: boolean) {
    super(message);
    this.name = "RemoteSolveError";
    this.fallbackAllowed = fallbackAllowed;
  }
}

interface OciJobResponse {
  id: string;
  status: "running" | "done" | "error" | "canceled";
  progress?: SolveProgress;
  solutions?: Solution[];
  partial_solutions?: Solution[];
  error?: string;
  metadata?: RemoteSolveMetadata;
}

export async function solveWithRemote(catalog: Catalog, scenario: Scenario, signal?: AbortSignal): Promise<RemoteSolveResult> {
  if (signal?.aborted) {
    throw createAbortError();
  }
  let response: Response;
  try {
    response = await fetch("/api/solve", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ catalog, scenario }),
      signal,
    });
  } catch (error) {
    if (isAbortError(error)) {
      throw createAbortError();
    }
    throw new RemoteSolveError(error instanceof Error ? error.message : "Remote solver is unavailable", true);
  }

  const metadata = remoteMetadataFromHeaders(response.headers);
  let text: string;
  try {
    text = await response.text();
  } catch (error) {
    if (isAbortError(error)) {
      throw createAbortError();
    }
    throw new RemoteSolveError(error instanceof Error ? error.message : "Remote solver response could not be read", true);
  }
  let parsed: Solution[] | { error?: string };
  try {
    parsed = text ? (JSON.parse(text) as Solution[] | { error?: string }) : [];
  } catch (parseError) {
    throw new RemoteSolveError(
      `Remote solver returned invalid JSON: ${parseError instanceof Error ? parseError.message : "unknown parse error"}`,
      response.status === 404 || response.status >= 500,
    );
  }

  if (!response.ok) {
    const message = Array.isArray(parsed) ? response.statusText : parsed.error || response.statusText || "Remote solver failed";
    throw new RemoteSolveError(message, response.status === 404 || response.status === 405 || response.status >= 500);
  }
  if (!Array.isArray(parsed)) {
    throw new RemoteSolveError(parsed.error || "Remote solver returned an unknown response", false);
  }
  return { solutions: parsed, metadata };
}

export async function solveWithOci(
  catalog: Catalog,
  scenario: Scenario,
  endpoint: string,
  token: string,
  onProgress?: (progress: SolveProgress) => void,
  onPartial?: (solutions: Solution[]) => void,
  signal?: AbortSignal,
): Promise<RemoteSolveResult> {
  const baseURL = normalizeEndpoint(endpoint);
  if (!baseURL) {
    throw new RemoteSolveError("OCI VM endpoint is required", false);
  }
  if (signal?.aborted) {
    throw createAbortError();
  }
  const createResponse = await fetch(`${baseURL}/api/jobs`, {
    method: "POST",
    headers: solveHeaders(token),
    body: JSON.stringify({ catalog, scenario }),
    signal,
  }).catch((error: unknown) => {
    if (isAbortError(error)) {
      throw createAbortError();
    }
    throw new RemoteSolveError(error instanceof Error ? error.message : "OCI VM solver is unavailable", false);
  });
  const createJob = await parseOciResponse(createResponse);
  if (!createResponse.ok) {
    throw new RemoteSolveError(createJob.error || createResponse.statusText || "OCI VM solver failed to start", false);
  }
  if (!createJob.id) {
    throw new RemoteSolveError("OCI VM solver did not return a job id", false);
  }
  onProgress?.(createJob.progress || { phase: "remote", elapsed_ms: 0, nodes_explored: 0 });
  if (Array.isArray(createJob.partial_solutions) && createJob.partial_solutions.length > 0) {
    onPartial?.(createJob.partial_solutions);
  }

  let canceled = false;
  const cancelRemote = async () => {
    if (canceled) {
      return;
    }
    canceled = true;
    try {
      await fetch(`${baseURL}/api/jobs/${encodeURIComponent(createJob.id)}/cancel`, {
        method: "POST",
        headers: solveHeaders(token),
        keepalive: true,
      });
    } catch {
      // Best-effort remote cancellation.
    }
  };

  return new Promise((resolve, reject) => {
    let timer: number | undefined;
    const cleanup = () => {
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
      signal?.removeEventListener("abort", handleAbort);
    };
    const fail = (error: Error) => {
      cleanup();
      reject(error);
    };
    const handleAbort = () => {
      void cancelRemote();
      fail(createAbortError());
    };
    const poll = async () => {
      if (signal?.aborted) {
        handleAbort();
        return;
      }
      try {
        const response = await fetch(`${baseURL}/api/jobs/${encodeURIComponent(createJob.id)}`, {
          method: "GET",
          headers: solveHeaders(token),
          signal,
        });
        const job = await parseOciResponse(response);
        if (!response.ok) {
          fail(new RemoteSolveError(job.error || response.statusText || "OCI VM solver polling failed", false));
          return;
        }
        if (job.progress) {
          onProgress?.(job.progress);
          if (Array.isArray(job.progress.partial_solutions) && job.progress.partial_solutions.length > 0) {
            onPartial?.(job.progress.partial_solutions);
          }
        }
        if (Array.isArray(job.partial_solutions) && job.partial_solutions.length > 0) {
          onPartial?.(job.partial_solutions);
        }
        if (job.status === "done") {
          cleanup();
          resolve({ solutions: job.solutions || [], metadata: job.metadata || { backend: "oci-vm" } });
          return;
        }
        if (job.status === "canceled") {
          fail(createAbortError());
          return;
        }
        if (job.status === "error") {
          fail(new RemoteSolveError(job.error || "OCI VM solver failed", false));
          return;
        }
        timer = window.setTimeout(poll, 500);
      } catch (error) {
        if (isAbortError(error)) {
          handleAbort();
          return;
        }
        fail(new RemoteSolveError(error instanceof Error ? error.message : "OCI VM solver polling failed", false));
      }
    };

    signal?.addEventListener("abort", handleAbort, { once: true });
    timer = window.setTimeout(poll, 500);
  });
}

export function solveWithWorker(
  catalog: Catalog,
  scenario: Scenario,
  onProgress?: (progress: SolveProgress) => void,
  signal?: AbortSignal,
): Promise<Solution[]> {
  if (signal?.aborted) {
    return Promise.reject(createAbortError());
  }
  if (activeWorkerRequestId !== null) {
    return Promise.reject(new Error("A local solver request is already running."));
  }
  let catalogVersion: CatalogVersion;
  try {
    catalogVersion = versionCatalog(catalog);
  } catch (error) {
    return Promise.reject(error instanceof Error ? error : new Error("Catalog could not be serialized"));
  }
  const worker = getSolverWorker();
  const requestId = nextWorkerRequestId++;
  activeWorkerRequestId = requestId;

  return new Promise((resolve, reject) => {
    let settled = false;
    const postSolve = () => {
      worker.postMessage({
        type: "solve",
        requestId,
        catalogIdentity: catalogVersion.catalogIdentity,
        catalogRevision: catalogVersion.catalogRevision,
        scenario,
      });
    };
    const cleanup = () => {
      worker.removeEventListener("message", handleMessage);
      worker.removeEventListener("error", handleError);
      worker.removeEventListener("messageerror", handleMessageError);
      signal?.removeEventListener("abort", handleAbort);
      if (activeWorkerRequestId === requestId) {
        activeWorkerRequestId = null;
      }
    };
    const settle = (callback: () => void) => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      callback();
    };
    const fail = (error: Error) => {
      settle(() => {
        resetSolverWorker();
        reject(error);
      });
    };
    const handleMessage = (event: MessageEvent<WorkerMessage>) => {
      const message = event.data;
      if (!message || message.requestId !== requestId) {
        return;
      }
      if (message.type === "progress") {
        onProgress?.(message.progress);
        return;
      }
      if (message.type === "catalogInstalled") {
        if (!catalogMatches(message, catalogVersion)) {
          fail(new Error("Solver worker installed an unexpected catalog revision"));
          return;
        }
        installedWorkerCatalog = catalogVersion;
        postSolve();
        return;
      }
      if (message.type === "result") {
        settle(() => resolve(message.solutions));
        return;
      }
      fail(new Error(message.error || "Solver failed"));
    };
    const handleError = (event: ErrorEvent) => {
      fail(new Error(event.message || "Solver worker failed"));
    };
    const handleMessageError = () => {
      fail(new Error("Solver worker sent an unreadable message"));
    };
    const handleAbort = () => {
      fail(createAbortError());
    };

    worker.addEventListener("message", handleMessage);
    worker.addEventListener("error", handleError);
    worker.addEventListener("messageerror", handleMessageError);
    signal?.addEventListener("abort", handleAbort, { once: true });
    if (signal?.aborted) {
      handleAbort();
      return;
    }
    if (catalogMatches(installedWorkerCatalog, catalogVersion)) {
      postSolve();
      return;
    }
    worker.postMessage({
      type: "installCatalog",
      requestId,
      catalogIdentity: catalogVersion.catalogIdentity,
      catalogRevision: catalogVersion.catalogRevision,
      catalogJSON: catalogVersion.catalogJSON,
    });
  });
}

export async function solveWithWasm(catalog: Catalog, scenario: Scenario): Promise<Solution[]> {
  await loadWasmSolver();
  if (!window.installCatalog || !window.solvePreparedScenario) {
    throw new Error("WASM solver did not initialize");
  }

  const catalogVersion = versionCatalog(catalog);
  if (!catalogMatches(installedDirectCatalog, catalogVersion)) {
    const installOutput = window.installCatalog(catalogVersion.catalogJSON);
    const installResult = parseWasmResult<{ ok?: boolean; error?: string }>(installOutput);
    if (!installResult.ok) {
      throw new Error(installResult.error || "WASM catalog installer returned an unknown error");
    }
    installedDirectCatalog = catalogVersion;
  }

  const output = window.solvePreparedScenario(JSON.stringify(scenario));
  const parsed = parseWasmResult<Solution[] | { error?: string }>(output);
  if (!Array.isArray(parsed)) {
    throw new Error(parsed.error || "Solver returned an unknown error");
  }
  return parsed;
}

function parseWasmResult<T>(output: unknown): T {
  if (typeof output !== "string") {
    throw new Error("WASM solver aborted before returning a result. Try a lower Max nodes value or fewer enabled coverage sources.");
  }
  try {
    return JSON.parse(output) as T;
  } catch (parseError) {
    throw new Error(`Solver returned invalid JSON: ${parseError instanceof Error ? parseError.message : "unknown parse error"}`);
  }
}

function getSolverWorker(): Worker {
  if (!solverWorker) {
    solverWorker = new Worker(new URL("./solverWorker.ts", import.meta.url), { type: "module" });
  }
  return solverWorker;
}

function resetSolverWorker(): void {
  if (solverWorker) {
    solverWorker.terminate();
    solverWorker = null;
  }
  installedWorkerCatalog = null;
}

function versionCatalog(catalog: Catalog): CatalogVersion {
	const previous = catalogVersions.get(catalog);
	if (previous) {
		return previous;
	}
	// The application replaces catalogs instead of mutating them in place, so
	// identity is a constant-time cache key for repeated local solves.
	let catalogIdentity = catalogIdentities.get(catalog);
  if (catalogIdentity === undefined) {
    catalogIdentity = nextCatalogIdentity++;
    catalogIdentities.set(catalog, catalogIdentity);
  }
  const version = {
    catalogIdentity,
    catalogRevision: nextCatalogRevision++,
    catalogJSON: JSON.stringify(catalog),
  };
  catalogVersions.set(catalog, version);
  return version;
}

function catalogMatches(left: CatalogKey | null, right: CatalogKey): boolean {
  return left !== null && left.catalogIdentity === right.catalogIdentity && left.catalogRevision === right.catalogRevision;
}

function createAbortError(): Error {
  const error = new Error("Solve canceled.");
  error.name = "AbortError";
  return error;
}

function remoteMetadataFromHeaders(headers: Headers): RemoteSolveMetadata {
  return {
    backend: headers.get("X-Solver-Backend") || "vercel-go",
    server_elapsed_ms: headerNumber(headers, "X-Solver-Server-Ms"),
    workers: headerNumber(headers, "X-Solver-Workers"),
    max_nodes_applied: headerNumber(headers, "X-Solver-Max-Nodes-Applied"),
    max_nodes_capped: headers.get("X-Solver-Max-Nodes-Capped") === "true",
  };
}

function normalizeEndpoint(endpoint: string): string {
  return endpoint.trim().replace(/\/+$/, "");
}

function solveHeaders(token: string): HeadersInit {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token.trim() !== "") {
    headers["X-Solver-Token"] = token.trim();
  }
  return headers;
}

async function parseOciResponse(response: Response): Promise<OciJobResponse> {
  const text = await response.text();
  if (!text) {
    return { id: "", status: "error", error: response.statusText };
  }
  try {
    return JSON.parse(text) as OciJobResponse;
  } catch (error) {
    throw new RemoteSolveError(
      `OCI VM solver returned invalid JSON: ${error instanceof Error ? error.message : "unknown parse error"}`,
      false,
    );
  }
}

function headerNumber(headers: Headers, name: string): number | undefined {
  const value = headers.get(name);
  if (!value) {
    return undefined;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function isAbortError(error: unknown): boolean {
  return typeof error === "object" && error !== null && "name" in error && (error as { name?: string }).name === "AbortError";
}

async function loadWasmSolver(): Promise<void> {
  if (window.solveScenario) {
    return;
  }
  if (!loadPromise) {
    loadPromise = loadWasmSolverOnce();
  }
  return loadPromise;
}

async function loadWasmSolverOnce(): Promise<void> {
  await loadScript("/wasm/wasm_exec.js");
  if (!window.Go) {
    throw new Error("Go WASM runtime is unavailable");
  }

  const go = new window.Go();
  const result = await WebAssembly.instantiateStreaming(fetch("/wasm/solver.wasm"), go.importObject);
  void go.run(result.instance);

  if (!window.solveScenario) {
    throw new Error("solveScenario was not registered by the WASM module");
  }
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${src}"]`);
    if (existing) {
      resolve();
      return;
    }
    const script = document.createElement("script");
    script.src = src;
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`Failed to load ${src}`));
    document.head.appendChild(script);
  });
}

import { expect, test } from "@playwright/test";
import type { Catalog } from "../src/types";

interface WorkerMessageLog {
  type?: string;
  catalogIdentity?: number;
  catalogRevision?: number;
  hasCatalogJSON: boolean;
  hasCatalog: boolean;
}

test("installs a catalog once per revision and sends scenario-only local solves", async ({ page }) => {
  await page.goto("/");

  const { messages, concurrentError } = await page.evaluate(async (): Promise<{
    messages: WorkerMessageLog[];
    concurrentError: string | null;
  }> => {
    const messages: WorkerMessageLog[] = [];
    const NativeWorker = window.Worker;
    class TrackingWorker extends NativeWorker {
      postMessage(message: unknown, transfer?: Transferable[]): void {
        if (typeof message === "object" && message !== null) {
          const payload = message as Record<string, unknown>;
          messages.push({
            type: typeof payload.type === "string" ? payload.type : undefined,
            catalogIdentity: typeof payload.catalogIdentity === "number" ? payload.catalogIdentity : undefined,
            catalogRevision: typeof payload.catalogRevision === "number" ? payload.catalogRevision : undefined,
            hasCatalogJSON: "catalogJSON" in payload,
            hasCatalog: "catalog" in payload,
          });
        }
        if (transfer) {
          super.postMessage(message, transfer);
          return;
        }
        super.postMessage(message);
      }
    }
    Object.defineProperty(window, "Worker", { configurable: true, value: TrackingWorker });

    const catalog = (await fetch("/data/catalog.json").then((response) => response.json())) as Catalog;
    const { solveWithWorker } = await import("/src/wasm.ts");
    const scenario = { items: { scalemail: 1 }, top: 1, max_nodes: 1 };

    const firstSolve = solveWithWorker(catalog, scenario);
    const concurrentError = await solveWithWorker(catalog, scenario).then(
      () => null,
      (error: unknown) => (error instanceof Error ? error.message : String(error)),
    );
    await firstSolve;
    await solveWithWorker(catalog, scenario);
    const replacementCatalog = structuredClone(catalog);
    await solveWithWorker(replacementCatalog, scenario);
    return { messages, concurrentError };
  });

  const installations = messages.filter((message) => message.type === "installCatalog");
  const solves = messages.filter((message) => message.type === "solve");
  expect(concurrentError).toBe("A local solver request is already running.");
  expect(installations).toHaveLength(2);
  expect(solves).toHaveLength(3);
  expect(installations[0]).toMatchObject({ hasCatalogJSON: true, hasCatalog: false });
  expect(installations[1]).toMatchObject({
    hasCatalogJSON: true,
    hasCatalog: false,
  });
  expect(installations[1].catalogIdentity).not.toBe(installations[0].catalogIdentity);
  expect(installations[1].catalogRevision).not.toBe(installations[0].catalogRevision);
  expect(solves[0]).toMatchObject({
    hasCatalogJSON: false,
    hasCatalog: false,
    catalogIdentity: installations[0].catalogIdentity,
    catalogRevision: installations[0].catalogRevision,
  });
  expect(solves[1]).toMatchObject({
    hasCatalogJSON: false,
    hasCatalog: false,
    catalogIdentity: installations[0].catalogIdentity,
    catalogRevision: installations[0].catalogRevision,
  });
  expect(solves[2]).toMatchObject({
    hasCatalogJSON: false,
    hasCatalog: false,
    catalogIdentity: installations[1].catalogIdentity,
    catalogRevision: installations[1].catalogRevision,
  });
});

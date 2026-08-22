import { expect, test } from "@playwright/test";

test("sends the selected outgoing priority semantics to local WASM", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Review Banana", { exact: true }).getByRole("button", { name: "+" }).click();

  await page.evaluate(() => {
    const scenarios: unknown[] = [];
    const NativeWorker = window.Worker;
    class TrackingWorker extends NativeWorker {
      postMessage(message: unknown, transfer?: Transferable[]): void {
        if (typeof message === "object" && message !== null) {
          const payload = message as { type?: string; scenario?: unknown };
          if (payload.type === "solve") {
            scenarios.push(payload.scenario);
          }
        }
        if (transfer) {
          super.postMessage(message, transfer);
          return;
        }
        super.postMessage(message);
      }
    }
    Object.defineProperty(window, "Worker", { configurable: true, value: TrackingWorker });
    Object.defineProperty(window, "__prioritySemanticsScenarios", { configurable: true, value: scenarios });
  });

  const semantics = page.getByLabel("Outgoing source objective");
  await semantics.selectOption("outgoing-per-instance-v3");
  await expect(semantics).toHaveValue("outgoing-per-instance-v3");
  await page.getByRole("button", { name: "Solve" }).click();

  await expect.poll(async () =>
    page.evaluate(() => {
      const scenarios = (window as unknown as { __prioritySemanticsScenarios?: Array<{ priority_semantics?: string }> })
        .__prioritySemanticsScenarios;
      return scenarios?.[0]?.priority_semantics;
    }),
  ).toBe("outgoing-per-instance-v3");
});

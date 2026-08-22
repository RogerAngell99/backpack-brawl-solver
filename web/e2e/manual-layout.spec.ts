import { expect, test } from "@playwright/test";

test("evaluates and reports the print reconstruction manually", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Preview print" }).click();

  const panel = page.getByText("Manual layout comparison", { exact: true });
  await panel.click();
	await page.locator(".manual-layout-grid .placed-cell[data-item-id='thornwall']").first().click();
	await page.getByLabel("Manual layout rotation").selectOption("90");
  await page.getByRole("button", { name: "Evaluate manual layout" }).click();

  await expect(page.getByText("Evaluated: 56 stars, priority 6/12.")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText("spirit_biscuit#19 ->", { exact: false })).toHaveCount(6);
  await expect(page.getByText("spice#14 ->", { exact: false })).toHaveCount(4);
});

test("places an item through the manual layout grid", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Review Banana", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByText("Manual layout comparison", { exact: true }).click();
  await page.getByLabel("Manual layout item").selectOption("banana");
  await page.getByLabel("Manual layout cell r0 c0").click();

  await expect(page.locator(".manual-layout-grid [data-item-id='banana']").first()).toBeVisible();
	const geometry = await page.locator(".manual-layout-grid").evaluate((grid) => {
		const item = grid.querySelector(".placed-item");
		const cell = grid.querySelector('[aria-label="Manual layout cell r0 c0"]');
		if (!item || !cell) throw new Error("manual placement elements missing");
		const itemRect = item.getBoundingClientRect();
		const cellRect = cell.getBoundingClientRect();
		return { itemX: itemRect.x, itemY: itemRect.y, cellX: cellRect.x, cellY: cellRect.y };
	});
	expect(geometry.itemX).toBeCloseTo(geometry.cellX, 1);
	expect(geometry.itemY).toBeCloseTo(geometry.cellY, 1);
  await page.getByRole("button", { name: "Evaluate manual layout" }).click();
  await expect(page.getByText("Evaluated: 0 stars, priority none.")).toBeVisible({ timeout: 30_000 });
});

test("uses an occupied anchor for a concave item", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Review Tender Sausage", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByLabel("Review Banana", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByText("Manual layout comparison", { exact: true }).click();

  const itemPicker = page.getByLabel("Manual layout item");
  await itemPicker.selectOption("tender_sausage");
  await page.getByLabel("Manual layout cell r2 c1").click();
  await itemPicker.selectOption("banana");
  await page.locator(".manual-layout-grid .manual-layout-cell").nth(2 * 9 + 2).click();

  await expect(page.locator(".manual-layout-grid .placed-item[data-item-id='banana']")).toHaveAttribute("style", /grid-area: 3 \/ 3/);
});

test("chooses rotation before placing an item", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Review Banana", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByText("Manual layout comparison", { exact: true }).click();

  await page.getByLabel("Manual layout item").selectOption("banana");
  await page.getByLabel("Manual layout rotation").selectOption("90");
  await expect(page.getByLabel("Manual layout rotation")).toHaveValue("90");
  await page.getByLabel("Manual layout cell r0 c0").click();

  await expect(page.locator(".manual-layout-grid .placed-item[data-item-id='banana']")).toHaveAttribute("data-rotation", "90");
});

test("attempts movement through an occupied anchor and reports a collision", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Review Tender Sausage", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByLabel("Review Banana", { exact: true }).getByRole("button", { name: "+" }).click();
  await page.getByText("Manual layout comparison", { exact: true }).click();

  const itemPicker = page.getByLabel("Manual layout item");
  await itemPicker.selectOption("tender_sausage");
  await page.getByLabel("Manual layout cell r2 c1").click();
  await itemPicker.selectOption("banana");
  await page.locator(".manual-layout-grid .manual-layout-cell").nth(2 * 9 + 2).click();
  await page.getByRole("button", { name: "Move left" }).click();

  await expect(page.getByText("Cannot move banana left: its occupied cells would overlap another item or leave the grid.")).toBeVisible();
  await expect(page.locator(".manual-layout-grid .placed-item[data-item-id='banana']")).toHaveAttribute("style", /grid-area: 3 \/ 3/);
});

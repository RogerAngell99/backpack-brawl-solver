import { expect, test } from "@playwright/test";

const rightEdgeSolution = [
  {
    score: { crafts: 0, stars: 1, items: 2 },
    search: { nodes_explored: 1, limited: false, refined: false },
    placements: [
      {
        instance_id: "starbloom#edge",
        item_id: "starbloom",
        rotation: 0,
        origin: [3, 8],
        cells: [
          [3, 8],
          [2, 8],
        ],
        star_positions: [[2, 7]],
      },
      {
        instance_id: "target#1",
        item_id: "thornwall",
        rotation: 0,
        origin: [2, 7],
        cells: [[2, 7]],
        star_positions: [],
      },
    ],
    crafts: [],
    stars: [
      {
        source_instance: "starbloom#edge",
        target_instance: "target#1",
        star_position: [2, 7],
        effect_text: "On Star activation: 35% chance to gain 1 Mana.",
      },
    ],
  },
];

test("catalog thumbnails load lazily and decode asynchronously", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText(/items loaded$/)).toBeVisible();

  const thumbnail = page.locator('[aria-label="Review Starbloom"] img');
  await expect(thumbnail).toHaveAttribute("loading", "lazy");
  await expect(thumbnail).toHaveAttribute("decoding", "async");
});

test("keeps the hover preview and active star visible on a right-edge item", async ({ page }) => {
  await page.route("**/api/solve", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(rightEdgeSolution),
    });
  });

  await page.goto("/");
  await expect(page.getByText(/items loaded$/)).toBeVisible();

  await page.getByPlaceholder("Search").fill("Starbloom");
  const itemRow = page.locator('[aria-label="Review Starbloom"]');
  await itemRow.getByRole("button", { name: "+" }).click();
  await page.getByLabel("Backend").selectOption("remote");
  await page.getByRole("button", { name: "Solve" }).click();

  const rightEdgeItem = page.locator('[data-placement-id="starbloom#edge"]').first();
  await expect(rightEdgeItem).toBeVisible();
  await rightEdgeItem.hover();

  const card = page.locator(".item-info-card");
  const activeStar = page.locator(".result-star-marker.active");
  await expect(card).toContainText("Starbloom");
  const itemBox = await rightEdgeItem.boundingBox();
  const cardBox = await card.boundingBox();
  if (!itemBox || !cardBox) {
    throw new Error("Expected the right-edge item and hover card to have layout boxes");
  }
  expect(cardBox.x).toBeLessThan(itemBox.x + itemBox.width);
  expect(cardBox.x + cardBox.width).toBeGreaterThan(itemBox.x);
  expect(cardBox.y).toBeLessThan(itemBox.y + itemBox.height);
  expect(cardBox.y + cardBox.height).toBeGreaterThan(itemBox.y);
  await expect(card).toHaveCSS("pointer-events", "none");
  await expect(activeStar).toBeVisible();

  await page.waitForTimeout(300);
  await expect(card).toBeVisible();
  await expect(activeStar).toBeVisible();

  await rightEdgeItem.click();
  await expect(card).toHaveCSS("pointer-events", "auto");
  await card.getByRole("button", { name: "Close item review" }).click();
  await expect(card).toBeHidden();
});

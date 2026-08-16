import { copyFileSync, cpSync, mkdirSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const webRoot = dirname(fileURLToPath(import.meta.url));
const projectRoot = join(webRoot, "..", "..");
const publicRoot = join(webRoot, "..", "public");

mkdirSync(join(publicRoot, "data"), { recursive: true });
mkdirSync(join(publicRoot, "assets"), { recursive: true });
mkdirSync(join(publicRoot, "scenarios"), { recursive: true });
mkdirSync(join(publicRoot, "wasm"), { recursive: true });

copyFileSync(join(projectRoot, "data", "catalog.json"), join(publicRoot, "data", "catalog.json"));
copyFileSync(join(projectRoot, "data", "item-metadata.json"), join(publicRoot, "data", "item-metadata.json"));
copyFileSync(join(projectRoot, "data", "item-visual-metadata.json"), join(publicRoot, "data", "item-visual-metadata.json"));
copyFileSync(
  join(projectRoot, "scenarios", "spinegrowth-basic.json"),
  join(publicRoot, "scenarios", "spinegrowth-basic.json"),
);

const itemAssetsTarget = join(publicRoot, "assets", "items");
rmSync(itemAssetsTarget, { recursive: true, force: true });
cpSync(join(projectRoot, "assets", "items"), itemAssetsTarget, { recursive: true });

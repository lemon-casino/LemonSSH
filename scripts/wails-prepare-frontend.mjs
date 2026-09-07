// Syncs the Vite build output (dist/) into cmd/netcatty/frontend/dist so the
// Go `//go:embed all:frontend/dist` in cmd/netcatty serves the real React
// bundle. Run after `npm run build`; wails:build chains both automatically.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const source = path.join(repoRoot, "dist");
const target = path.join(repoRoot, "cmd", "netcatty", "frontend", "dist");

if (!fs.existsSync(path.join(source, "index.html"))) {
  console.error("dist/index.html not found; run `npm run build` first");
  process.exit(1);
}

fs.rmSync(target, { recursive: true, force: true });
fs.mkdirSync(path.dirname(target), { recursive: true });
fs.cpSync(source, target, { recursive: true });
console.log(`Synced ${path.relative(repoRoot, source)} -> ${path.relative(repoRoot, target)}`);

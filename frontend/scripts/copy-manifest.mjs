// Copies Vite's build manifest from dist/.vite/manifest.json to
// dist/manifest.json. Go's //go:embed excludes hidden directories (".vite"),
// so the manifest must live at a non-hidden path for the backend to embed it
// and resolve the true entry chunk for server-side rendering.
import { copyFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const outDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'backend', 'internal', 'public', 'dist');
const src = join(outDir, '.vite', 'manifest.json');
const dest = join(outDir, 'manifest.json');

try {
  mkdirSync(dirname(dest), { recursive: true });
  copyFileSync(src, dest);
  console.log(`Copied Vite manifest to ${dest}`);
} catch (err) {
  console.error(`Failed to copy Vite manifest: ${err.message}`);
  process.exit(1);
}

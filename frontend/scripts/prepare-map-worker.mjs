import { copyFile, mkdir, readFile } from "node:fs/promises";

const packageUrl = import.meta.resolve("maplibre-gl/package.json");
const { version } = JSON.parse(await readFile(new URL(packageUrl), "utf8"));
const target = new URL(`../public/maplibre/${version}/`, import.meta.url);

await mkdir(target, { recursive: true });
for (const filename of ["maplibre-gl-worker.mjs", "maplibre-gl-worker.mjs.map"]) {
  const source = new URL(`dist/${filename}`, packageUrl);
  await copyFile(source, new URL(filename, target));
}
await copyFile(new URL("LICENSE.txt", packageUrl), new URL("LICENSE.txt", target));

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const directory = path.dirname(fileURLToPath(import.meta.url));
const npmDirectory = path.resolve(directory, "..");
const platformsDirectory = path.join(npmDirectory, "platforms");

function packageJSON(directory) {
  return JSON.parse(fs.readFileSync(path.join(directory, "package.json"), "utf8"));
}

test("source package versions stay release-driven", () => {
  const directories = [
    path.join(npmDirectory, "root"),
    ...fs.readdirSync(platformsDirectory).map((name) => path.join(platformsDirectory, name)),
  ];
  for (const directory of directories) {
    assert.equal(packageJSON(directory).version, "0.0.0");
  }
});

test("root package includes every platform package", () => {
  const root = packageJSON(path.join(npmDirectory, "root"));
  const platformPackages = fs
    .readdirSync(platformsDirectory)
    .map((name) => packageJSON(path.join(platformsDirectory, name)).name)
    .sort();
  assert.deepEqual(Object.keys(root.optionalDependencies).sort(), platformPackages);
});

test("platform metadata matches its directory", () => {
  for (const name of fs.readdirSync(platformsDirectory)) {
    const platform = packageJSON(path.join(platformsDirectory, name));
    const [operatingSystem, architecture] = name.split("-");
    assert.deepEqual(platform.os, [operatingSystem]);
    assert.deepEqual(platform.cpu, [architecture]);
  }
});

test("publisher includes every platform directory", () => {
  const publisher = fs.readFileSync(path.join(npmDirectory, "scripts", "publish.mjs"), "utf8");
  for (const name of fs.readdirSync(platformsDirectory)) {
    assert.match(publisher, new RegExp(`directory: "${name}"`));
  }
  for (const match of publisher.matchAll(/directory:\s*"([^"]+)"/g)) {
    assert.ok(fs.existsSync(path.join(platformsDirectory, match[1])));
  }
});

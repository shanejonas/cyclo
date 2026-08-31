import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const directory = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(directory, "..", "..");
const shim = path.join(repository, "npm", "root", "bin", "cyclo.js");
const testBinary = process.env.CYCLO_TEST_BINARY
  ? path.resolve(process.env.CYCLO_TEST_BINARY)
  : "";

const platformPackages = {
  "darwin-arm64": "@shanejonas/cyclo-darwin-arm64",
  "darwin-x64": "@shanejonas/cyclo-darwin-x64",
  "linux-arm64": "@shanejonas/cyclo-linux-arm64",
  "linux-x64": "@shanejonas/cyclo-linux-x64",
  "win32-arm64": "@shanejonas/cyclo-win32-arm64",
  "win32-x64": "@shanejonas/cyclo-win32-x64",
};

function testLayout(platformPackage, includeBinary) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "cyclo-npm-"));
  const scope = path.join(root, "node_modules", "@shanejonas");
  const shimDirectory = path.join(scope, "cyclo", "bin");
  fs.mkdirSync(shimDirectory, { recursive: true });
  const installedShim = path.join(shimDirectory, "cyclo.js");
  fs.copyFileSync(shim, installedShim);

  if (!includeBinary) {
    return installedShim;
  }

  assert.ok(testBinary, "CYCLO_TEST_BINARY is required");
  const packageDirectory = path.join(scope, platformPackage.replace(/^@shanejonas\//, ""));
  const binaryDirectory = path.join(packageDirectory, "bin");
  fs.mkdirSync(binaryDirectory, { recursive: true });
  const binaryName = process.platform === "win32" ? "cyclo.exe" : "cyclo";
  fs.copyFileSync(testBinary, path.join(binaryDirectory, binaryName));
  fs.chmodSync(path.join(binaryDirectory, binaryName), 0o755);
  return installedShim;
}

const hostPackage = platformPackages[`${process.platform}-${process.arch}`];

test("shim runs the host binary", () => {
  if (!hostPackage) {
    return;
  }
  const installedShim = testLayout(hostPackage, true);
  const output = execFileSync(process.execPath, [installedShim, "--skill"], {
    encoding: "utf8",
  });
  assert.match(output, /^---\nname: cyclo\n/);
});

test("shim returns the host binary exit code", () => {
  if (!hostPackage) {
    return;
  }
  const installedShim = testLayout(hostPackage, true);
  assert.throws(
    () =>
      execFileSync(process.execPath, [installedShim, "--skill", "extra"], {
        encoding: "utf8",
        stdio: "pipe",
      }),
    (error) => error.status === 1 && /usage: cyclo --skill/.test(error.stderr)
  );
});

test("shim explains a missing platform package", () => {
  const platformPackage = hostPackage ?? "@shanejonas/cyclo-linux-x64";
  const installedShim = testLayout(platformPackage, false);
  assert.throws(
    () =>
      execFileSync(process.execPath, [installedShim, "--skill"], {
        encoding: "utf8",
        stdio: "pipe",
      }),
    (error) => error.status === 1 && error.stderr.includes(platformPackage)
  );
});

test("shim rejects unsupported platforms", () => {
  const installedShim = testLayout(hostPackage ?? "@shanejonas/cyclo-linux-x64", false);
  const script = [
    'Object.defineProperty(process, "platform", { value: "haiku" });',
    'Object.defineProperty(process, "arch", { value: "mips" });',
    `require(${JSON.stringify(installedShim)});`,
  ].join("\n");
  assert.throws(
    () => execFileSync(process.execPath, ["-e", script], { encoding: "utf8", stdio: "pipe" }),
    (error) => error.status === 1 && /unsupported platform "haiku-mips"/.test(error.stderr)
  );
});

#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");

const platformPackages = {
  "darwin-arm64": "@shanejonas/cyclo-darwin-arm64",
  "darwin-x64": "@shanejonas/cyclo-darwin-x64",
  "linux-arm64": "@shanejonas/cyclo-linux-arm64",
  "linux-x64": "@shanejonas/cyclo-linux-x64",
  "win32-arm64": "@shanejonas/cyclo-win32-arm64",
  "win32-x64": "@shanejonas/cyclo-win32-x64",
};

const platform = `${process.platform}-${process.arch}`;
const platformPackage = platformPackages[platform];
if (!platformPackage) {
  console.error(
    `cyclo: unsupported platform "${platform}". Supported: ${Object.keys(platformPackages).join(", ")}`
  );
  process.exit(1);
}

const binary = process.platform === "win32" ? "cyclo.exe" : "cyclo";
let binaryPath;
try {
  binaryPath = require.resolve(`${platformPackage}/bin/${binary}`);
} catch {
  console.error(
    `cyclo: optional dependency "${platformPackage}" is missing. ` +
      "Reinstall with optional dependencies enabled."
  );
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`cyclo: failed to run: ${result.error.message}`);
  process.exit(1);
}

process.exit(result.status ?? 1);

#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const directory = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(directory, "..", "..");
const npmDirectory = path.join(repository, "npm");
const outputDirectory = path.join(repository, "dist", "npm");
const dryRun = process.argv.includes("--dry-run");

const rawVersion = process.env.VERSION;
if (!rawVersion) {
  console.error("publish.mjs: VERSION is required");
  process.exit(1);
}
const version = rawVersion.replace(/^v/, "");

const platforms = [
  { goos: "darwin", goarch: "arm64", directory: "darwin-arm64" },
  { goos: "darwin", goarch: "amd64", directory: "darwin-x64" },
  { goos: "linux", goarch: "arm64", directory: "linux-arm64" },
  { goos: "linux", goarch: "amd64", directory: "linux-x64" },
  { goos: "windows", goarch: "arm64", directory: "win32-arm64" },
  { goos: "windows", goarch: "amd64", directory: "win32-x64" },
];

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function writeJSON(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function findBinary(artifacts, platform) {
  const artifact = artifacts.find(
    (candidate) =>
      candidate.type === "Binary" &&
      candidate.goos === platform.goos &&
      candidate.goarch === platform.goarch
  );
  if (!artifact) {
    throw new Error(`missing ${platform.goos}/${platform.goarch} binary`);
  }
  return path.join(repository, artifact.path);
}

function packagePublished(name) {
  if (dryRun) {
    return false;
  }
  try {
    execFileSync("npm", ["view", `${name}@${version}`, "version"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function publish(directory) {
  const packageJSON = readJSON(path.join(directory, "package.json"));
  if (packagePublished(packageJSON.name)) {
    console.log(`${packageJSON.name}@${version} already published`);
    return;
  }

  const args = ["publish", "--access", "public"];
  if (dryRun) {
    args.push("--dry-run", "--provenance=false");
  } else {
    args.push("--provenance");
  }
  execFileSync("npm", args, { cwd: directory, stdio: "inherit" });
}

function preparePlatformPackage(artifacts, platform) {
  const source = path.join(npmDirectory, "platforms", platform.directory);
  const destination = path.join(outputDirectory, "platforms", platform.directory);
  fs.cpSync(source, destination, { recursive: true });

  const packageFile = path.join(destination, "package.json");
  const packageJSON = readJSON(packageFile);
  packageJSON.version = version;
  writeJSON(packageFile, packageJSON);

  const binaryName = platform.goos === "windows" ? "cyclo.exe" : "cyclo";
  const binaryDirectory = path.join(destination, "bin");
  fs.mkdirSync(binaryDirectory, { recursive: true });
  const binary = path.join(binaryDirectory, binaryName);
  fs.copyFileSync(findBinary(artifacts, platform), binary);
  fs.chmodSync(binary, 0o755);

  return destination;
}

function prepareRootPackage() {
  const source = path.join(npmDirectory, "root");
  const destination = path.join(outputDirectory, "root");
  fs.cpSync(source, destination, { recursive: true });

  const packageFile = path.join(destination, "package.json");
  const packageJSON = readJSON(packageFile);
  packageJSON.version = version;
  for (const name of Object.keys(packageJSON.optionalDependencies)) {
    packageJSON.optionalDependencies[name] = version;
  }
  writeJSON(packageFile, packageJSON);

  return destination;
}

function main() {
  const artifacts = readJSON(path.join(repository, "dist", "artifacts.json"));
  fs.rmSync(outputDirectory, { force: true, recursive: true });

  const platformPackages = platforms.map((platform) =>
    preparePlatformPackage(artifacts, platform)
  );
  for (const platformPackage of platformPackages) {
    publish(platformPackage);
  }
  publish(prepareRootPackage());
}

main();

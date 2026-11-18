#!/usr/bin/env bun
// Rewrites the release-controlled fields of backend/wails.yaml before a build.
//
// Wake (wails v3) reads Go linker flags only from the manifest: the CLI has no
// -ldflags flag and GOFLAGS cannot carry them because Wake always emits its own
// -ldflags for stripping. Release scripts therefore patch the manifest in place
// and restore it afterwards.
//
// Usage:
//   bun scripts/wails-manifest-patch.mjs --manifest backend/wails.yaml \
//     --version v2.2.5 --ldflags "-X pkg.Version=v2.2.5 -X pkg.Commit=abc"
//   bun scripts/wails-manifest-patch.mjs --manifest backend/wails.yaml --restore
import { copyFileSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";

const args = new Map();
for (let i = 2; i < process.argv.length; i += 2) {
    args.set(process.argv[i], process.argv[i + 1]);
}

const manifestPath = args.get("--manifest");
if (!manifestPath) {
    fail("--manifest is required");
}

const backupPath = `${manifestPath}.release-backup`;
const restore = process.argv.includes("--restore");

if (restore) {
    try {
        renameSync(backupPath, manifestPath);
    } catch (error) {
        if (error.code !== "ENOENT") throw error;
    }
    process.exit(0);
}

const version = args.get("--version");
const ldflags = args.get("--ldflags") ?? "";
if (!version) fail("--version is required");

copyFileSync(manifestPath, backupPath);

const original = readFileSync(manifestPath, "utf8");
const lines = original.split(/\r?\n/);

const projectIndex = findBlock("project");
if (projectIndex < 0) fail("project block missing from manifest");

const buildIndex = findBlock("build");

setScalar(projectIndex, "version", version.replace(/^v/, ""));

if (ldflags.trim()) {
    if (buildIndex < 0) {
        lines.push("build:", `  ldflags:${yamlList(ldflags.trim())}`);
    } else {
        setScalar(buildIndex, "ldflags", null, ldflags.trim());
    }
}

const next = `${lines.join("\n").replace(/\n*$/, "")}\n`;
writeFileSync(manifestPath, next);
process.exit(0);

function fail(message) {
    process.stderr.write(`wails-manifest-patch: ${message}\n`);
    process.exit(1);
}

function findBlock(name) {
    const index = lines.findIndex((line) => line === `${name}:`);
    if (index < 0) return -1;
    // A top-level block ends at the next line that starts in column 0.
    for (let i = index + 1; i < lines.length; i++) {
        const line = lines[i];
        if (line.length > 0 && !line.startsWith(" ") && !line.startsWith("#")) {
            return index;
        }
    }
    return index;
}

// Replaces a direct child key of a top-level block, removing any nested lines it
// owned. `value === null` means the replacement is a block sequence (`ldflags`).
function setScalar(blockIndex, key, value, sequenceItem) {
    let end = lines.length;
    for (let i = blockIndex + 1; i < lines.length; i++) {
        const line = lines[i];
        if (line.length > 0 && !line.startsWith(" ") && !line.startsWith("#")) {
            end = i;
            break;
        }
    }
    const keyIndex = lines.findIndex(
        (line, i) => i > blockIndex && i < end && line.match(new RegExp(`^ {2}${key}:`)),
    );
    const replacement =
        value === null ? [`  ${key}:`, ...yamlList(sequenceItem)] : [`  ${key}: ${value}`];
    if (keyIndex < 0) {
        lines.splice(blockIndex + 1, 0, ...replacement);
        return;
    }
    let owned = 1;
    while (keyIndex + owned < end && lines[keyIndex + owned].startsWith("    ")) {
        owned++;
    }
    lines.splice(keyIndex, owned, ...replacement);
}

// A single quoted scalar keeps the whole linker flag string on one line; Wake
// joins manifest list entries with a space when invoking go build.
function yamlList(item) {
    return [`    - ${JSON.stringify(item)}`];
}

#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function readStableVersion(value) {
    const version = value.trim();
    if (!/^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) {
        throw new Error(`VERSION ${JSON.stringify(version)} 不是稳定 vMAJOR.MINOR.PATCH`);
    }
    return version;
}

function projectVersion(wails) {
    const projectStart = wails.split(/\r?\n/).findIndex((line) => line === "project:");
    if (projectStart < 0) throw new Error("backend/wails.yaml 缺少 project 块");
    for (const line of wails.split(/\r?\n/).slice(projectStart + 1)) {
        if (/^[^\s#].*:$/.test(line)) break;
        const match = /^\s{2}version:\s*(\S+)\s*$/.exec(line);
        if (match) return match[1].replace(/^['"]|['"]$/g, "");
    }
    throw new Error("backend/wails.yaml 缺少 project.version");
}

function changelogHasVersion(changelog, version) {
    return changelog.split(/\r?\n/).includes(`## ${version}`);
}

export function checkReleaseConsistency(repositoryRoot = root, tag = "") {
    const version = readStableVersion(readFileSync(join(repositoryRoot, "VERSION"), "utf8"));
    const wails = projectVersion(readFileSync(join(repositoryRoot, "backend", "wails.yaml"), "utf8"));
    if (wails !== version.slice(1)) {
        throw new Error(`backend/wails.yaml project.version=${wails}，应为 ${version.slice(1)}`);
    }
    if (!changelogHasVersion(readFileSync(join(repositoryRoot, "CHANGELOG.md"), "utf8"), version)) {
        throw new Error(`CHANGELOG.md 缺少 ## ${version}`);
    }
    if (tag && tag !== version) throw new Error(`Git tag ${tag} 与 VERSION ${version} 不一致`);
    return { version, wailsVersion: wails, tag: tag || undefined };
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
    try {
        const tagIndex = process.argv.indexOf("--tag");
        const tag = tagIndex >= 0 ? process.argv[tagIndex + 1] || "" : "";
        const result = checkReleaseConsistency(root, tag);
        process.stdout.write(`release consistency passed: ${result.version}\n`);
    } catch (error) {
        process.stderr.write(`check-release-consistency: ${error.message}\n`);
        process.exitCode = 1;
    }
}

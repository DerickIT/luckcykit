#!/usr/bin/env node
import { readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const defaultRoot = resolve(scriptDir, "..");

export function parseStableVersion(value) {
    const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(value.trim());
    if (!match) throw new Error(`版本 ${JSON.stringify(value)} 不是稳定 vMAJOR.MINOR.PATCH`);
    return [BigInt(match[1]), BigInt(match[2]), BigInt(match[3])];
}

function canonicalStableVersion(value) {
    const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(value.trim());
    if (!match) throw new Error(`版本 ${JSON.stringify(value)} 不是稳定 vMAJOR.MINOR.PATCH`);
    return `v${match[1]}.${match[2]}.${match[3]}`;
}

function compareStableVersionText(left, right) {
    const leftParts = canonicalStableVersion(left).slice(1).split(".");
    const rightParts = canonicalStableVersion(right).slice(1).split(".");
    for (let index = 0; index < leftParts.length; index += 1) {
        const leftPart = BigInt(leftParts[index]);
        const rightPart = BigInt(rightParts[index]);
        if (leftPart !== rightPart) return leftPart > rightPart ? 1 : -1;
    }
    return 0;
}

function readVersion(root) {
    return readFileSync(join(root, "VERSION"), "utf8").trim();
}

function readWailsVersion(root) {
    const lines = readFileSync(join(root, "backend", "wails.yaml"), "utf8").split(/\r?\n/);
    const projectStart = lines.findIndex((line) => line === "project:");
    if (projectStart < 0) throw new Error("backend/wails.yaml 缺少 project 块");
    for (let index = projectStart + 1; index < lines.length; index += 1) {
        const line = lines[index];
        if (/^[^\s#].*:$/.test(line)) break;
        const match = /^\s{2}version:\s*(\S+)\s*$/.exec(line);
        if (match) return match[1].replace(/^['\"]|['\"]$/g, "");
    }
    throw new Error("backend/wails.yaml 缺少 project.version");
}

function updateWailsVersion(root, version) {
    const path = join(root, "backend", "wails.yaml");
    const lines = readFileSync(path, "utf8").split(/\r?\n/);
    const projectStart = lines.findIndex((line) => line === "project:");
    if (projectStart < 0) throw new Error("backend/wails.yaml 缺少 project 块");
    for (let index = projectStart + 1; index < lines.length; index += 1) {
        const line = lines[index];
        if (/^[^\s#].*:$/.test(line)) break;
        if (/^\s{2}version:\s*\S+\s*$/.test(line)) {
            lines[index] = `  version: ${version.slice(1)}`;
            writeFileSync(path, `${lines.join("\n").replace(/\n*$/, "")}\n`);
            return;
        }
    }
    throw new Error("backend/wails.yaml 缺少 project.version");
}

function readChangelog(root) {
    return readFileSync(join(root, "CHANGELOG.md"), "utf8").replaceAll("\r\n", "\n");
}

function sectionBody(changelog, heading) {
    const lines = changelog.split("\n");
    const start = lines.indexOf(heading);
    if (start < 0) return null;
    const end = lines.findIndex((line, index) => index > start && line.startsWith("## "));
    return lines.slice(start + 1, end < 0 ? lines.length : end).join("\n").trim();
}

function assertReleaseMetadata(root, targetVersion) {
    const currentVersion = canonicalStableVersion(readVersion(root));
    const wailsVersion = readWailsVersion(root);
    if (wailsVersion !== currentVersion.slice(1)) {
        throw new Error(`backend/wails.yaml project.version=${wailsVersion}，应与 VERSION ${currentVersion.slice(1)} 一致`);
    }
    const normalizedTargetVersion = canonicalStableVersion(targetVersion);
    if (compareStableVersionText(normalizedTargetVersion, currentVersion) <= 0) {
        throw new Error(`${normalizedTargetVersion} 必须高于当前 VERSION ${currentVersion}`);
    }

    const changelog = readChangelog(root);
    const unreleased = sectionBody(changelog, "## Unreleased");
    if (!unreleased) throw new Error("CHANGELOG.md 的 Unreleased 不能为空");
    const releaseHeading = `## ${normalizedTargetVersion}`;
    if (changelog.split("\n").includes(releaseHeading)) {
        throw new Error(`CHANGELOG.md 已存在 ${releaseHeading}`);
    }

    return { currentVersion, targetVersion: normalizedTargetVersion, unreleased, changelog };
}

function applyReleaseMetadata(root, metadata) {
    const lines = metadata.changelog.split("\n");
    const start = lines.indexOf("## Unreleased");
    const end = lines.findIndex((line, index) => index > start && line.startsWith("## "));
    const releaseLines = ["## Unreleased", "", `## ${metadata.targetVersion}`, "", metadata.unreleased, ""];
    lines.splice(start, (end < 0 ? lines.length : end) - start, ...releaseLines);
    writeFileSync(join(root, "VERSION"), `${metadata.targetVersion}\n`);
    writeFileSync(join(root, "CHANGELOG.md"), `${lines.join("\n").replace(/\n*$/, "\n")}`);
}

function gitStatus(root) {
    return execFileSync("git", ["-C", root, "status", "--short"], { encoding: "utf8" }).trim();
}

function assertReleaseTargetsAreClean(root) {
    const status = gitStatus(root);
    const protectedFiles = new Set(["VERSION", "CHANGELOG.md", "backend/wails.yaml"]);
    const changedProtectedFiles = status
        .split("\n")
        .map((line) => line.slice(3).trim())
        .filter((path) => protectedFiles.has(path));
    if (changedProtectedFiles.length) {
        throw new Error(`VERSION 或 CHANGELOG.md 已有未提交修改，发布准备不会覆盖：${changedProtectedFiles.join(", ")}`);
    }
}

export function run(argv, root = defaultRoot) {
    let targetVersion = "";
    let apply = false;
    let allowDirty = false;
    for (let index = 0; index < argv.length; index += 1) {
        const argument = argv[index];
        if (argument === "--version") targetVersion = argv[++index] ?? "";
        else if (argument === "--apply") apply = true;
        else if (argument === "--allow-dirty") allowDirty = true;
        else if (argument === "--check") apply = false;
        else if (argument === "--help" || argument === "-h") return "用法: node scripts/prepare-release.mjs --version vX.Y.Z [--check|--apply] [--allow-dirty]";
        else throw new Error(`未知参数 ${argument}`);
    }
    if (!targetVersion) throw new Error("需要 --version vX.Y.Z");
    if (apply) {
        if (!allowDirty && gitStatus(root)) {
            throw new Error("工作树不干净；发布准备不会覆盖已有修改，请先提交或显式传入 --allow-dirty");
        }
        assertReleaseTargetsAreClean(root);
    }

    const metadata = assertReleaseMetadata(root, targetVersion);
    if (apply) {
        applyReleaseMetadata(root, metadata);
        updateWailsVersion(root, metadata.targetVersion);
    }
    return apply
        ? `已准备 ${targetVersion}：更新 VERSION 和 CHANGELOG.md`
        : `检查通过：可准备 ${targetVersion}（使用 --apply 才会写入文件）`;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
    try {
        process.stdout.write(`${run(process.argv.slice(2))}\n`);
    } catch (error) {
        process.stderr.write(`prepare-release: ${error.message}\n`);
        process.exitCode = 1;
    }
}

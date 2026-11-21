#!/usr/bin/env node
// 依赖风险报告：把 Go 模块图与 npm 已解析依赖里的公告列出来，并区分
// 「参与构建」与「只在模块图里」。可达漏洞由 govulncheck 与 bun audit 在 CI 拦截，
// 本脚本用于维护 docs/technical/dependency-security.md 的残留清单。
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const OSV_BATCH = "https://api.osv.dev/v1/querybatch";
const BATCH_SIZE = 500;
const GO_BINARIES = ["./cmd/desktop", "./cmd/server", "./cmd/luckytv", "./cmd/cloud"];

function run(command, args, cwd) {
    const result = spawnSync(command, args, { cwd, encoding: "utf8", shell: false });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(`${command} ${args.join(" ")} failed: ${result.stderr || result.stdout}`);
    return result.stdout;
}

function goModules() {
    const cwd = join(repoRoot, "backend");
    const listed = run("go", ["list", "-m", "-f", "{{if .Version}}{{.Path}} {{.Version}}{{end}}", "all"], cwd);
    const modules = listed
        .split("\n")
        .map((line) => line.trim().split(" "))
        .filter((parts) => parts.length === 2 && !parts[0].startsWith("luckytv/backend"))
        .map(([name, version]) => ({ ecosystem: "Go", name, version }));
    const deps = run("go", ["list", "-deps", ...GO_BINARIES], cwd);
    const imported = deps.split("\n").map((line) => line.trim()).filter(Boolean);
    const used = new Set();
    for (const module of modules) {
        const prefix = `${module.name}/`;
        if (imported.some((path) => path === module.name || path.startsWith(prefix))) used.add(module.name);
    }
    return modules.map((module) => ({ ...module, built: used.has(module.name) }));
}

function npmModules(directory) {
    const cwd = join(repoRoot, directory);
    const listed = run("bun", ["pm", "ls", "--all"], cwd);
    const seen = new Set();
    const modules = [];
    for (const line of listed.split("\n")) {
        const match = /([@A-Za-z0-9][^\s]*?)@(\d[^\s]*)$/.exec(line.trim());
        if (!match) continue;
        const [, name, version] = match;
        const key = `${name}@${version}`;
        if (seen.has(key)) continue;
        seen.add(key);
        modules.push({ ecosystem: "npm", name, version, built: true, source: directory });
    }
    return modules;
}

async function queryOsv(modules) {
    const results = new Map();
    for (let index = 0; index < modules.length; index += BATCH_SIZE) {
        const batch = modules.slice(index, index + BATCH_SIZE);
        const response = await fetch(OSV_BATCH, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({
                queries: batch.map((module) => ({ package: { name: module.name, ecosystem: module.ecosystem }, version: module.version })),
            }),
        });
        if (!response.ok) throw new Error(`OSV query failed: ${response.status}`);
        const body = await response.json();
        body.results.forEach((result, offset) => {
            if (result.vulns?.length) results.set(`${batch[offset].ecosystem}:${batch[offset].name}`, result.vulns.map((vuln) => vuln.id));
        });
    }
    return results;
}

const modules = [...goModules(), ...npmModules("web"), ...npmModules("agent-host")];
const advisories = await queryOsv(modules);
const affected = modules
    .filter((module) => advisories.has(`${module.ecosystem}:${module.name}`))
    .map((module) => ({ ...module, advisories: advisories.get(`${module.ecosystem}:${module.name}`) }));

const importedVulnerabilities = affected.filter((module) => module.built);
const graphOnly = affected.filter((module) => !module.built);

if (process.argv.includes("--json")) {
    console.log(JSON.stringify({ affected, importedVulnerabilities: importedVulnerabilities.length, graphOnly: graphOnly.length }, null, 2));
} else {
    const row = (module) =>
        `| \`${module.name}\` | ${module.version} | ${module.advisories.length} | ${module.built ? "参与构建" : "仅模块图"} |`;
    console.log(`扫描 ${modules.length} 个依赖单元（Go 模块图 + npm 已解析依赖）`);
    console.log(`命中公告：参与构建 ${importedVulnerabilities.length} 个，仅模块图 ${graphOnly.length} 个`);
    if (affected.length) {
        console.log("");
        console.log("| 依赖 | 版本 | 公告数 | 状态 |");
        console.log("| --- | --- | --- | --- |");
        for (const module of affected) console.log(row(module));
    }
    console.log("");
    console.log("参与构建的命中项必须修复；仅模块图的条目记录在 docs/technical/dependency-security.md。");
}

if (process.argv.includes("--fail-on-imported") && importedVulnerabilities.length) process.exitCode = 1;

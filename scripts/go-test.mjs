#!/usr/bin/env node
// 在 Go 环境自带的临时目录里跑测试。
//
// testing.T.TempDir() 使用进程的 TEMP/TMP，而不是 GOTMPDIR；Windows 上若它落在
// C 盘，一次全量测试可能遗留数 GB。默认 GOTMPDIR 已经是 Go 安装目录下的 temp，
// 这里把 TEMP/TMP 对齐到同一位置，让测试临时库与构建临时目录落在同一个可清理位置。
//
// 用法：node scripts/go-test.mjs [go test 参数...]（默认 ./...）
import { spawnSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const backendDir = join(repoRoot, "backend");

function goEnv(key) {
    const result = spawnSync("go", ["env", key], { cwd: backendDir, encoding: "utf8" });
    if (result.error) throw result.error;
    return result.stdout.trim();
}

const gotmp = goEnv("GOTMPDIR") || join(goEnv("GOROOT"), "tmp");
mkdirSync(gotmp, { recursive: true });

const args = process.argv.slice(2);
const packages = args.length ? args : ["./..."];
const result = spawnSync("go", ["test", ...packages], {
    cwd: backendDir,
    stdio: "inherit",
    env: { ...process.env, TEMP: gotmp, TMP: gotmp, TMPDIR: gotmp },
});
process.exit(result.status ?? 1);
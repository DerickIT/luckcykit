#!/usr/bin/env node
// 清理测试与构建留下的临时目录。
//
// Windows 上 SQLite 句柄未关闭时 testing.T.TempDir 的 RemoveAll 会失败，
// 每次 go test 都可能在 %TEMP% 留下 Test* 目录（实测一次全量套件数 GB）。
// 本脚本只删除 %TEMP% 下形如 Test* 的目录以及 .local/cache 下的构建缓存。
import { existsSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const dryRun = process.argv.includes("--dry-run");

function remove(path) {
    if (!existsSync(path)) return 0;
    if (dryRun) {
        console.log(`would remove ${path}`);
        return 1;
    }
    rmSync(path, { recursive: true, force: true, maxRetries: 3 });
    return 1;
}

let removed = 0;
const tempRoot = tmpdir();
for (const entry of readdirSync(tempRoot, { withFileTypes: true })) {
    if (!entry.isDirectory() || !entry.name.startsWith("Test")) continue;
    removed += remove(join(tempRoot, entry.name));
}

// 本地验证期间产生的 Go/Wails 缓存；可重新生成。
const localCache = join(repoRoot, ".local", "cache");
const scratch = ["go-build", "go-build-local", "go-build-check", "go-build-final", "go-build-final2", "go-build-elevated", "go-cache-seeded-20261119b", "go-mod", "go-mod-scan", "go-mod-1274-clean", "go1274-repaired", "wails-localappdata", "bun", "tmp"];
for (const name of scratch) {
    removed += remove(join(localCache, name));
}

console.log(dryRun ? `dry run: ${removed} path(s) would be removed` : `removed ${removed} path(s)`);
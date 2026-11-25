import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { parseStableVersion, run } from "./prepare-release.mjs";

function fixture() {
    const root = mkdtempSync(join(tmpdir(), "luckytv-release-"));
    writeFileSync(join(root, "VERSION"), "v2.2.5\n");
    writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n\n## Unreleased\n\n- 修复示例问题\n\n## v2.2.5\n\n- 已发布\n");
    mkdirSync(join(root, "backend"));
    writeFileSync(join(root, "backend", "wails.yaml"), "project:\n  version: 2.2.5\n  name: test\n");
    execFileSync("git", ["-C", root, "init", "--quiet"]);
    execFileSync("git", ["-C", root, "add", "VERSION", "CHANGELOG.md", "backend/wails.yaml"]);
    execFileSync("git", ["-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "fixture"]);
    return root;
}

test("parseStableVersion rejects prerelease and malformed versions", () => {
    assert.deepEqual(parseStableVersion("v2.3.0"), [2n, 3n, 0n]);
    assert.deepEqual(parseStableVersion("v9007199254740993.0.0"), [9007199254740993n, 0n, 0n]);
    assert.throws(() => parseStableVersion("2.3.0"));
    assert.throws(() => parseStableVersion("v2.3.0-rc.1"));
});

test("release check is read-only", () => {
    const root = fixture();
    const beforeVersion = readFileSync(join(root, "VERSION"), "utf8");
    const beforeChangelog = readFileSync(join(root, "CHANGELOG.md"), "utf8");
    assert.match(run(["--version", "v2.3.0", "--check"], root), /检查通过/);
    assert.equal(readFileSync(join(root, "VERSION"), "utf8"), beforeVersion);
    assert.equal(readFileSync(join(root, "CHANGELOG.md"), "utf8"), beforeChangelog);
});

test("release check canonicalizes surrounding whitespace", () => {
    const root = fixture();
    assert.match(run(["--version", " v2.3.0 ", "--check"], root), /检查通过/);
});

test("release apply promotes Unreleased without changing older notes", () => {
    const root = fixture();
    assert.match(run(["--version", "v2.3.0", "--apply"], root), /已准备/);
    assert.equal(readFileSync(join(root, "VERSION"), "utf8"), "v2.3.0\n");
    assert.match(readFileSync(join(root, "backend", "wails.yaml"), "utf8"), /version: 2\.3\.0/);
    const changelog = readFileSync(join(root, "CHANGELOG.md"), "utf8");
    assert.match(changelog, /## Unreleased\n\n## v2\.3\.0\n\n- 修复示例问题/);
    assert.match(changelog, /## v2\.2\.5\n\n- 已发布/);
});

test("release apply refuses a dirty worktree unless explicitly allowed", () => {
    const root = fixture();
    writeFileSync(join(root, "dirty.txt"), "existing user work\n");
    assert.throws(() => run(["--version", "v2.3.0", "--apply"], root), /工作树不干净/);
});

test("allow-dirty never overwrites dirty release targets", () => {
    const root = fixture();
    writeFileSync(join(root, "backend", "wails.yaml"), "# changed by user\n");
    assert.throws(
        () => run(["--version", "v2.3.0", "--apply", "--allow-dirty"], root),
        (error) => error.message.includes("backend/wails.yaml"),
    );
});

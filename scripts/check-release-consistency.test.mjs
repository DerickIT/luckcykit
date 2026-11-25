import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { checkReleaseConsistency } from "./check-release-consistency.mjs";

function fixture() {
    const root = mkdtempSync(join(tmpdir(), "luckytv-consistency-"));
    writeFileSync(join(root, "VERSION"), "v2.2.5\n");
    writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n\n## Unreleased\n\n## v2.2.5\n\n- stable\n");
    writeFileSync(join(root, "backend-wails.yaml"), "project:\n  version: 2.2.5\n  name: test\n");
    return root;
}

function useFixture(root) {
    const wails = join(root, "backend", "wails.yaml");
    mkdirSync(join(root, "backend"));
    renameSync(join(root, "backend-wails.yaml"), wails);
}

test("release consistency validates the canonical application metadata", () => {
    const root = fixture();
    useFixture(root);
    assert.deepEqual(checkReleaseConsistency(root), { version: "v2.2.5", wailsVersion: "2.2.5", tag: undefined });
    assert.equal(checkReleaseConsistency(root, "v2.2.5").tag, "v2.2.5");
});

test("release consistency rejects drift and mismatched tags", () => {
    const root = fixture();
    useFixture(root);
    writeFileSync(join(root, "backend", "wails.yaml"), "project:\n  version: 2.2.4\n");
    assert.throws(() => checkReleaseConsistency(root), /project.version/);
    writeFileSync(join(root, "backend", "wails.yaml"), "project:\n  version: 2.2.5\n");
    assert.throws(() => checkReleaseConsistency(root, "v2.2.4"), /Git tag/);
});

import assert from "node:assert/strict";
import test from "node:test";
import { invalidCommitSubjects, isAllowedCommitSubject } from "./check-commit-subjects.mjs";

test("commit subject accepts the repository format", () => {
    assert.equal(isAllowedCommitSubject("fix(canvas): 画布 - 修复标题保存"), true);
    assert.equal(isAllowedCommitSubject("chore(deps): update Go modules"), true);
});

test("commit subject accepts merge and dependency automation exceptions", () => {
    assert.equal(isAllowedCommitSubject("Merge pull request #42 from dependabot/go_modules/example"), true);
    assert.equal(isAllowedCommitSubject("Bump actions/checkout from 4 to 5"), true);
    assert.equal(isAllowedCommitSubject("chore(deps-dev): bump a package"), true);
});

test("commit subject rejects vague or malformed subjects", () => {
    const invalid = invalidCommitSubjects([
        "fix bug",
        "fix(canvas): missing separator",
        "feat(): empty scope - add feature",
        "feat(canvas): - empty summary",
    ]);
    assert.deepEqual(invalid, [
        "fix bug",
        "fix(canvas): missing separator",
        "feat(): empty scope - add feature",
        "feat(canvas): - empty summary",
    ]);
});

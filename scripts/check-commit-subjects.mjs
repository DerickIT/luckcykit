#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const SUBJECT_PATTERN = /^(feat|fix|refactor|perf|docs|test|build|ci|chore|revert)\([^)\r\n]+\):\s+[^\r\n]+\s+-\s+\S/;
const MERGE_PATTERN = /^Merge\s+.+/;
const DEPENDENCY_PATTERN = /^(Bump\s+.+\s+from\s+.+\s+to\s+.+|chore\(deps(?:-[^)]+|\([^)]*\))?\):\s+.+)$/i;

export function isAllowedCommitSubject(subject) {
    const value = subject.trim();
    return SUBJECT_PATTERN.test(value) || MERGE_PATTERN.test(value) || DEPENDENCY_PATTERN.test(value);
}

export function invalidCommitSubjects(subjects) {
    return subjects.filter((subject) => !isAllowedCommitSubject(subject));
}

export function readCommitSubjects(repositoryRoot, from, to = "HEAD") {
    if (!from || !to) throw new Error("需要 --from 和 --to");
    const output = execFileSync("git", ["-C", repositoryRoot, "log", "--format=%s", `${from}..${to}`], { encoding: "utf8" });
    return output.split(/\r?\n/).map((subject) => subject.trim()).filter(Boolean);
}

export function checkCommitSubjects(repositoryRoot, from, to = "HEAD") {
    const subjects = readCommitSubjects(repositoryRoot, from, to);
    const invalid = invalidCommitSubjects(subjects);
    if (invalid.length > 0) {
        throw new Error(`提交主题不符合 <type>(<scope>): <业务模块> - <变更摘要>：\n${invalid.map((subject) => `- ${subject}`).join("\n")}`);
    }
    return subjects;
}

function parseArguments(argv) {
    let from = "";
    let to = "HEAD";
    for (let index = 0; index < argv.length; index += 1) {
        const argument = argv[index];
        if (argument === "--from") from = argv[++index] ?? "";
        else if (argument === "--to") to = argv[++index] ?? "";
        else if (argument === "--help" || argument === "-h") return null;
        else throw new Error(`未知参数 ${argument}`);
    }
    return { from, to };
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
    try {
        const argumentsValue = parseArguments(process.argv.slice(2));
        if (!argumentsValue) {
            process.stdout.write("用法: node scripts/check-commit-subjects.mjs --from <base> [--to <head>]\n");
        } else {
            const subjects = checkCommitSubjects(resolve("."), argumentsValue.from, argumentsValue.to);
            process.stdout.write(`commit subjects passed: ${subjects.length}\n`);
        }
    } catch (error) {
        process.stderr.write(`check-commit-subjects: ${error.message}\n`);
        process.exitCode = 1;
    }
}

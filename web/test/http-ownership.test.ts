import { expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";

function walkSourceFiles(dir: string): string[] {
    const files: string[] = [];
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const path = join(dir, entry.name);
        if (entry.isDirectory()) {
            files.push(...walkSourceFiles(path));
            continue;
        }
        if (/\.(ts|tsx)$/.test(entry.name)) files.push(path);
    }
    return files;
}

const webRoot = resolve(import.meta.dir, "..");
const srcRoot = resolve(webRoot, "src");

/** Windows 上 relative() 返回反斜杠；白名单与断言统一按 POSIX 分隔符比较。 */
function srcRel(file: string) {
    return relative(srcRoot, file).split(sep).join("/");
}

const axiosAllowed = new Set([
    "services/api/request.ts",
    // 独立 origin 的外部服务客户端：不复用本地 http 的 baseUrl、桌面 token 与信封解包。
    "services/api/external-client.ts",
    "services/api/channel-transport.ts",
    "services/api/image-response.ts",
    "services/diagnostics/client-diagnostics.ts",
]);

test("business API modules call http instead of axios or request(apiClient)", () => {
    const offenders = walkSourceFiles(resolve(srcRoot, "services"))
        .filter((file) => {
            const rel = srcRel(file);
            if (axiosAllowed.has(rel)) return false;
            const source = readFileSync(file, "utf8");
            return /from ["']axios["']/.test(source) || /request\s*(?:<[^>]+>)?\s*\(\s*apiClient\./.test(source) || /const api = apiClient/.test(source);
        })
        .map(srcRel);
    expect(offenders).toEqual([]);
});

test("axios.create stays in the shared request client", () => {
    const allowed: Record<string, true> = {
        "services/api/request.ts": true,
        "services/api/external-client.ts": true,
    };
    const offenders = walkSourceFiles(srcRoot)
        .filter((file) => {
            if (allowed[srcRel(file)]) return false;
            return readFileSync(file, "utf8").includes("axios.create(");
        })
        .map(srcRel);
    expect(offenders).toEqual([]);
});

test("copied flush Modal padding lives only in AppModal", () => {
    const banned = [
        'styles={{ container: { padding: 0, overflow: "hidden" }, body: { padding: 0 } }}',
        "styles={{ container: { padding: 0 }, body: { padding: 0 } }}",
    ];
    const offenders = walkSourceFiles(srcRoot)
        .filter((file) => {
            if (srcRel(file) === "components/ui/product/app-modal/app-modal.tsx") return false;
            const source = readFileSync(file, "utf8");
            return banned.some((snippet) => source.includes(snippet));
        })
        .map(srcRel);
    expect(offenders).toEqual([]);
});

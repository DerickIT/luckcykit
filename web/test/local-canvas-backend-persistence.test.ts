import { expect, test } from "bun:test";
import { isBrowserLocalRuntimeFor } from "@/lib/runtime-mode";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const repository = readFileSync(resolve(import.meta.dir, "../src/services/local-workspace-repository.ts"), "utf8");
const lifecycle = readFileSync(resolve(import.meta.dir, "../src/pages/canvas/use-canvas-project-lifecycle.ts"), "utf8");

test("runtime selects browser persistence only without a desktop or backend runtime", () => {
    expect(isBrowserLocalRuntimeFor({ desktopShell: false, embedded: false, backendConfigured: false, productionBuild: true, apiBaseURL: "/api" })).toBe(true);
    expect(isBrowserLocalRuntimeFor({ desktopShell: false, embedded: false, backendConfigured: false, productionBuild: false, apiBaseURL: "/api" })).toBe(false);
    expect(isBrowserLocalRuntimeFor({ desktopShell: false, embedded: false, backendConfigured: true, productionBuild: true, apiBaseURL: "/api" })).toBe(false);
    expect(isBrowserLocalRuntimeFor({ desktopShell: false, embedded: false, backendConfigured: false, productionBuild: true, apiBaseURL: "http://127.0.0.1:8080/api" })).toBe(false);
    expect(isBrowserLocalRuntimeFor({ desktopShell: true, embedded: false, backendConfigured: false, productionBuild: true, apiBaseURL: "/api" })).toBe(false);
    expect(isBrowserLocalRuntimeFor({ desktopShell: false, embedded: true, backendConfigured: false, productionBuild: true, apiBaseURL: "/api" })).toBe(false);
});

test("local canvas edits schedule and explicit saves await backend persistence", () => {
    expect(lifecycle).toContain("if (localMode) scheduleLocalCanvasBackendSync(projectId);");
    expect(lifecycle).toContain("if (localMode) await syncLocalCanvasProjectToBackend(projectId);");
    expect(repository).toContain("const backendSaveTails = new Map<string, Promise<void>>()");
    expect(repository).toContain("commitCanvasDocument");
});

test("canvas save summary never replaces the full node document", () => {
    expect(repository).toContain("type CanvasSaveSummary = Pick<CanvasProject");
    expect(repository).not.toContain("current.updatedAt === project.updatedAt ? saved");
    expect(repository).toContain("revision: saved.revision");
});

test("local canvas deletion removes the canonical backend record", () => {
    expect(repository).toContain("await http.delete(`/canvas-projects/${encodeURIComponent(id)}`)");
});

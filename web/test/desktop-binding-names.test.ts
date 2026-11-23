import { afterEach, describe, expect, test } from "bun:test";

import { getDesktopAppBinding, installDesktopAppGlobalBinding } from "@/services/desktop-runtime";

// 桌面壳是 package main 编出的可执行文件，Wails 注册的绑定名是 main.DesktopApp.<方法>。
// @wailsio/runtime 的 Call.ByName 最终 POST 到 /wails/runtime，这里拦住该请求读取方法名，
// 既验证真实调用路径，又避免 mock.module 污染同批次的其他测试。
type BoundCall = { method: string; args: unknown[] };
const boundCalls: BoundCall[] = [];

const originalWindow = globalThis.window;
const originalFetch = globalThis.fetch;

function jsonResponse(value: unknown) {
    return new Response(JSON.stringify(value), { status: 200, headers: { "content-type": "application/json" } });
}

type ShellWindow = {
    location: { protocol: string; origin: string; hostname: string; host: string };
    go?: { main?: { DesktopApp?: Record<string, (...args: unknown[]) => Promise<unknown>> } };
};

function setDesktopShellWindow(): ShellWindow {
    const shell: ShellWindow = {
        location: { protocol: "http:", origin: "http://wails.localhost:3000", hostname: "wails.localhost", host: "wails.localhost:3000" },
    };
    Object.assign(globalThis, { window: shell });
    return shell;
}

function recordBindingCalls() {
    globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
        const body = JSON.parse(String(init?.body ?? "{}")) as { args?: { methodName?: string; args?: unknown[] } };
        const methodName = body.args?.methodName;
        if (methodName) boundCalls.push({ method: methodName, args: body.args?.args ?? [] });
        return jsonResponse("ok");
    }) as typeof fetch;
}

afterEach(() => {
    boundCalls.length = 0;
    globalThis.window = originalWindow;
    globalThis.fetch = originalFetch;
});

describe("desktop shell binding names", () => {
    test("calls bound methods as main.DesktopApp.<method>", async () => {
        setDesktopShellWindow();
        recordBindingCalls();
        const binding = getDesktopAppBinding();
        expect(binding).toBeDefined();
        await binding!.StartupState!();
        expect(boundCalls).toEqual([{ method: "main.DesktopApp.StartupState", args: [] }]);
    });

    test("exposes the v2-style window.go proxy so capability probes still work", async () => {
        const shell = setDesktopShellWindow();
        recordBindingCalls();
        installDesktopAppGlobalBinding();
        const desktopApp = shell.go?.main?.DesktopApp;
        expect(desktopApp).toBeDefined();
        // desktop-media-save.ts 靠这个判断能不能把文件存到本地。
        expect(typeof desktopApp?.SaveOwnedMedia).toBe("function");
        await desktopApp!.SaveOwnedMedia("镜头 01.mp4", "resource-1");
        expect(boundCalls).toEqual([{ method: "main.DesktopApp.SaveOwnedMedia", args: ["镜头 01.mp4", "resource-1"] }]);
    });

    test("does not install the proxy outside the desktop shell", () => {
        const browserWindow: { location: unknown; go?: unknown } = { location: { protocol: "http:", origin: "http://localhost:3000", hostname: "localhost", host: "localhost:3000" } };
        Object.assign(globalThis, { window: browserWindow });
        installDesktopAppGlobalBinding();
        expect(browserWindow.go).toBeUndefined();
    });
});

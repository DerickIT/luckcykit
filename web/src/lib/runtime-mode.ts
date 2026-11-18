import { useUserStore } from "@/stores/use-user-store";
import { apiBaseURL } from "@/services/api/request";
import { isDesktopShellOrigin } from "@/lib/desktop-shell";
import { isEmbeddedRuntime } from "@/lib/embedded-runtime";

export type RuntimeSessionLike = {
    storageMode?: "local" | "remote";
    user?: { username?: string } | null;
};

/**
 * Pure runtime-mode decision used by persistence and upload boundaries.
 * Local-first is the default for the desktop build; hosted mode must opt out
 * explicitly with VITE_CANVAS_LOCAL_MODE=false.
 */
export function isLocalRuntimeModeForSession(session: RuntimeSessionLike, localFirstBuild: boolean) {
    return localFirstBuild || session.storageMode === "local" || session.user?.username === "local";
}

export function isLocalRuntimeMode() {
    return isLocalRuntimeModeForSession(
        useUserStore.getState(),
        import.meta.env.VITE_CANVAS_LOCAL_MODE !== "false",
    );
}

/** True inside the native Wails desktop shell or when targeting the loopback Go API. */
export function isNativeDesktopRuntime() {
    return isDesktopShellOrigin()
        || /^http:\/\/127\.0\.0\.1:\d+\/api$/u.test(String(apiBaseURL));
}

export function isBrowserLocalRuntimeFor(input: { desktopShell: boolean; embedded: boolean; backendConfigured: boolean; productionBuild: boolean; apiBaseURL: string }) {
    return !input.desktopShell
        && !input.embedded
        && !input.backendConfigured
        && input.productionBuild
        && input.apiBaseURL.startsWith("/");
}

/** Browser-local builds persist workspace data in IndexedDB and must not call same-origin /api. */
export function isBrowserLocalRuntime() {
    return isBrowserLocalRuntimeFor({
        desktopShell: isDesktopShellOrigin(),
        embedded: isEmbeddedRuntime(),
        backendConfigured: Boolean(import.meta.env.VITE_CANVAS_BACKEND_URL),
        productionBuild: import.meta.env.PROD,
        apiBaseURL: String(apiBaseURL),
    });
}

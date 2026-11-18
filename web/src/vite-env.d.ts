/// <reference types="vite/client" />

declare const __APP_VERSION__: string;
declare const __LUCKYTV_HEAVY_MEDIA_ENABLED__: boolean;

// Desktop shell binding shape. Wails v3 routing is handled by
// services/desktop-runtime.ts; this declaration keeps legacy call sites typed.
interface Window {
    go?: {
        main?: {
            DesktopApp?: Record<string, (...args: any[]) => Promise<any>>;
        };
    };
}

import { create } from "zustand";

import {
    defaultCloudApiBaseURL,
    getCloudAccount,
    getCloudLedger,
    getCloudModels,
    normalizeCloudApiBaseURL,
    requestCloudEmailCode,
    revokeCloudSession,
    verifyCloudEmailCode,
    type CloudAccount,
    type CloudLedgerEntry,
    type CloudModel,
} from "@/services/api/cloud-client";
import { getDesktopAppBinding } from "@/services/desktop-runtime";

type CloudSessionStore = {
    baseURL: string;
    email: string;
    token: string | null;
    expiresAt: string | null;
    account: CloudAccount | null;
    models: CloudModel[];
    ledger: CloudLedgerEntry[];
    hydrated: boolean;
    hydrating: boolean;
    loading: boolean;
    error: string | null;
    setBaseURL: (value: string) => void;
    setEmail: (value: string) => void;
    requestCode: () => Promise<void>;
    verifyCode: (code: string) => Promise<void>;
    hydrate: () => Promise<void>;
    refresh: () => Promise<void>;
    logout: () => Promise<void>;
};

type StoredCloudSession = { baseURL: string; email: string; token: string; expiresAt: string };

async function readStoredSession(): Promise<StoredCloudSession | null> {
    const binding = getDesktopAppBinding();
    if (!binding?.CloudSession) return null;
    const raw = await binding.CloudSession("read");
    if (!raw) return null;
    try {
        const parsed = JSON.parse(raw) as Partial<StoredCloudSession>;
        if (typeof parsed.baseURL !== "string" || typeof parsed.email !== "string" || typeof parsed.token !== "string" || typeof parsed.expiresAt !== "string") return null;
        if (Date.parse(parsed.expiresAt) <= Date.now()) return null;
        return { baseURL: normalizeCloudApiBaseURL(parsed.baseURL), email: parsed.email, token: parsed.token, expiresAt: parsed.expiresAt };
    } catch {
        return null;
    }
}

async function writeStoredSession(session: StoredCloudSession | null) {
    const binding = getDesktopAppBinding();
    if (!binding?.CloudSession) return;
    if (!session) {
        await binding.CloudSession("clear");
        return;
    }
    await binding.CloudSession("write", JSON.stringify(session));
}

export const useCloudSessionStore = create<CloudSessionStore>((set, get) => ({
    baseURL: defaultCloudApiBaseURL(),
    email: "",
    token: null,
    expiresAt: null,
    account: null,
    models: [],
    ledger: [],
    hydrated: false,
    hydrating: false,
    loading: false,
    error: null,
    setBaseURL: (value) => set({ baseURL: value, error: null }),
    setEmail: (value) => set({ email: value, error: null }),
    requestCode: async () => {
        const { baseURL, email } = get();
        set({ loading: true, error: null });
        try {
            await requestCloudEmailCode(normalizeCloudApiBaseURL(baseURL), email);
        } catch (error) {
            set({ error: error instanceof Error ? error.message : "验证码发送失败" });
            throw error;
        } finally {
            set({ loading: false });
        }
    },
    verifyCode: async (code) => {
        const { baseURL, email } = get();
        set({ loading: true, error: null });
        try {
            const result = await verifyCloudEmailCode(normalizeCloudApiBaseURL(baseURL), email, code);
            const session = { baseURL: normalizeCloudApiBaseURL(baseURL), email, token: result.token, expiresAt: result.expiresAt };
            try {
                await writeStoredSession(session);
            } catch {
                // Unsupported desktop platforms keep the token in memory for this run.
            }
            set({ ...session, hydrated: true });
            await get().refresh();
        } catch (error) {
            set({ error: error instanceof Error ? error.message : "验证码验证失败" });
            throw error;
        } finally {
            set({ loading: false });
        }
    },
    // Reads the locally stored session only. Network refresh is the page's job so
    // starting the app never blocks on an unconfigured cloud endpoint.
    hydrate: async () => {
        const { hydrated, hydrating } = get();
        if (hydrated || hydrating) return;
        set({ hydrating: true });
        try {
            const session = await readStoredSession();
            set({ hydrating: false, hydrated: true });
            if (session) set(session);
        } catch (error) {
            // Keep hydrated false so a later mount can retry the read.
            set({ hydrating: false, hydrated: false, error: error instanceof Error ? error.message : "云端会话恢复失败" });
        } finally {
            set({ hydrating: false });
        }
    },
    refresh: async () => {
        const { baseURL, token } = get();
        if (!token) return;
        set({ loading: true, error: null });
        try {
            const [account, ledger, modelResult] = await Promise.all([
                getCloudAccount(baseURL, token),
                getCloudLedger(baseURL, token),
                getCloudModels(baseURL, token),
            ]);
            set({ account, ledger: ledger.entries, models: modelResult.models, error: null });
        } catch (error) {
            // Errors surface through the store; callers (buttons) render them.
            set({ error: error instanceof Error ? error.message : "云端数据加载失败" });
        } finally {
            set({ loading: false });
        }
    },
    logout: async () => {
        const { baseURL, token } = get();
        set({ loading: true, error: null });
        let revokeError: string | null = null;
        if (token) {
            try {
                await revokeCloudSession(baseURL, token);
            } catch (error) {
                revokeError = error instanceof Error ? error.message : "云端退出失败";
            }
        }
        try {
            await writeStoredSession(null);
        } catch {
            // Clearing the in-memory session remains authoritative.
        }
        set({
            token: null,
            expiresAt: null,
            account: null,
            models: [],
            ledger: [],
            loading: false,
            error: revokeError ? `云端退出失败，本地会话已清除：${revokeError}` : null,
        });
    },
}));
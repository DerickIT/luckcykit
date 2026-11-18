import { App } from "antd";
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";

import { mergeManagedLuckyTVCatalog, shouldRefreshLuckyTVCatalog } from "@/lib/managed-luckytv-catalog";
import { localWorkspaceConfig } from "@/lib/user-session";
import { cancelLuckyTVConnection, disconnectLuckyTVConnection, getLuckyTVConnection, getLuckyTVConnectionEpoch, openLuckyTVWallet, startLuckyTVConnection, type LuckyTVConnectionSummary } from "@/services/api/luckytv-connection";
import { getLocalModelConfig } from "@/services/api/workspace";
import { normalizeConfigSnapshot, useConfigStore } from "@/stores/use-config-store";

const LuckyTVConnectionContext = createContext<ReturnType<typeof useConnectionState> | null>(null);

export function LuckyTVConnectionProvider({ children }: { children: ReactNode }) {
    const connection = useConnectionState();
    return <LuckyTVConnectionContext.Provider value={connection}>{children}</LuckyTVConnectionContext.Provider>;
}

export function useLuckyTVConnection() {
    const connection = useContext(LuckyTVConnectionContext);
    if (!connection) throw new Error("LuckyTVConnectionProvider is required");
    return connection;
}

function useConnectionState() {
    const { message } = App.useApp();
    const [connection, setConnection] = useState<LuckyTVConnectionSummary | null>(null);
    const [busy, setBusy] = useState(false);
    const [loading, setLoading] = useState(true);
    const [syncing, setSyncing] = useState(false);
    const [catalogSyncFailed, setCatalogSyncFailed] = useState(false);
    const [balanceRefreshFailed, setBalanceRefreshFailed] = useState(false);
    const currentConnection = useRef<LuckyTVConnectionSummary | null>(null);
    const appliedState = useRef<string | undefined>(undefined);
    const catalogSync = useRef<{ state: string; selection: string; adopt: boolean; pending: boolean } | null>(null);
    const actionPending = useRef(false);
    const mutationSeq = useRef(0);
    const scheduleNextPoll = useRef<((state?: string) => void) | null>(null);

    const applyConnection = async (summary: LuckyTVConnectionSummary, options?: { allowRetry?: boolean }) => {
        const previousState = currentConnection.current?.state;
        currentConnection.current = summary;
        setConnection(summary);
        scheduleNextPoll.current?.(summary.state);
        const retry = catalogSync.current?.state === summary.state ? catalogSync.current : null;
        if (retry?.pending) return;
        if (appliedState.current === summary.state && !(retry && options?.allowRetry)) return;
        appliedState.current = summary.state;
        if (!retry && !shouldRefreshLuckyTVCatalog(previousState, summary.state)) {
            catalogSync.current = null;
            setCatalogSyncFailed(false);
            return;
        }
        const intent = retry || { state: summary.state, selection: useConfigStore.getState().config.assistantModel, adopt: summary.state === "connected" && Boolean(previousState && previousState !== "connected"), pending: false };
        catalogSync.current = intent;
        intent.pending = true;
        setSyncing(true);
        try {
            const result = await getLocalModelConfig();
            if (catalogSync.current !== intent) return;
            const current = useConfigStore.getState().config;
            useConfigStore.getState().replaceConfig(localWorkspaceConfig(normalizeConfigSnapshot({
                config: mergeManagedLuckyTVCatalog(current, result.config, intent.adopt && current.assistantModel === intent.selection),
            }).config));
            catalogSync.current = null;
            setCatalogSyncFailed(false);
        } catch {
            if (catalogSync.current === intent) setCatalogSyncFailed(true);
        } finally {
            intent.pending = false;
            if (!catalogSync.current || catalogSync.current === intent) setSyncing(false);
        }
    };

    const refresh = async (options?: { allowRetry?: boolean }) => applyConnection(await getLuckyTVConnection(), options);

    useEffect(() => {
        let stopped = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        const arm = (state = currentConnection.current?.state) => {
            clearTimeout(timer);
            if (stopped) return;
            timer = setTimeout(poll, state === "pending" || state === "connecting" ? 2000 : 30000);
        };
        scheduleNextPoll.current = arm;
        const poll = async () => {
            if (!actionPending.current) {
                const epoch = getLuckyTVConnectionEpoch();
                const seq = mutationSeq.current;
                try {
                    const summary = await getLuckyTVConnection();
                    if (!stopped && !actionPending.current && epoch === getLuckyTVConnectionEpoch() && seq === mutationSeq.current) {
                        await applyConnection(summary);
                    }
                } catch { /* 后台状态读取失败时保留已确认的连接，手动操作显示错误。 */ }
            }
            if (!stopped) {
                setLoading(false);
                arm();
            }
        };
        void poll();
        return () => {
            stopped = true;
            scheduleNextPoll.current = null;
            clearTimeout(timer);
        };
    }, []);

    const runAction = async (action: () => Promise<LuckyTVConnectionSummary>, options?: { allowRetry?: boolean }) => {
        if (actionPending.current) return;
        actionPending.current = true;
        mutationSeq.current += 1;
        setBusy(true);
        const previous = currentConnection.current;
        try {
            await applyConnection(await action(), options);
        } catch (error) {
            try { await refresh(); } catch { setConnection(previous); }
            message.error(error instanceof Error ? error.message : "无法更新 LuckyTV 连接，请重试");
        } finally {
            actionPending.current = false;
            setLoading(false);
            setBusy(false);
        }
    };

    const connect = () => runAction(async () => {
        const state = currentConnection.current?.state;
        if (state === "expired" || state === "revoked" || state === "rejected") await disconnectLuckyTVConnection();
        setConnection({ state: "connecting", hasCredential: currentConnection.current?.hasCredential || false });
        return startLuckyTVConnection();
    });

    const retryCatalog = () => {
        const current = currentConnection.current;
        if (current?.catalogFailed || current?.state === "catalog_failed") return connect();
        return runAction(getLuckyTVConnection, { allowRetry: true });
    };

    const refreshBalance = async () => {
        if (actionPending.current) return;
        const epoch = getLuckyTVConnectionEpoch();
        const seq = mutationSeq.current;
        try {
            const summary = await getLuckyTVConnection(undefined, { refresh: true });
            if (!actionPending.current && epoch === getLuckyTVConnectionEpoch() && seq === mutationSeq.current) {
                mutationSeq.current += 1;
                setBalanceRefreshFailed(false);
                await applyConnection(summary);
            }
        } catch {
            if (epoch === getLuckyTVConnectionEpoch() && seq === mutationSeq.current) {
                setBalanceRefreshFailed(true);
            }
        }
    };

    const applySummary = (summary: LuckyTVConnectionSummary) => {
        mutationSeq.current += 1;
        setBalanceRefreshFailed(false);
        void applyConnection(summary);
    };

    const getMutationSeq = () => mutationSeq.current;

    const applyBalanceCnyIfCurrent = (expectedSeq: number, balanceCny: string) => {
        if (actionPending.current || expectedSeq !== mutationSeq.current) return false;
        const current = currentConnection.current;
        if (!current || !/^\d+(?:\.\d{2})?$/.test(balanceCny)) return false;
        const nextBalance = balanceCny === "0.00" ? "zero" : "available";
        mutationSeq.current += 1;
        setBalanceRefreshFailed(false);
        if (current.balanceCny === balanceCny && current.balance === nextBalance) return true;
        void applyConnection({ ...current, balance: nextBalance, balanceCny });
        return true;
    };

    return {
        connection,
        busy,
        loading,
        syncing,
        balanceRefreshFailed,
        catalogSyncFailed: catalogSyncFailed || Boolean(connection?.catalogFailed || connection?.state === "catalog_failed"),
        connect,
        cancel: () => runAction(cancelLuckyTVConnection),
        disconnect: () => runAction(disconnectLuckyTVConnection),
        refresh: () => runAction(getLuckyTVConnection, { allowRetry: true }),
        refreshBalance,
        getMutationSeq,
        applySummary,
        applyBalanceCnyIfCurrent,
        retryCatalog,
        openAccount: () => openLuckyTVWallet().catch((error) => message.error(error instanceof Error ? error.message : "无法打开 LuckyTV 账户")),
    };
}

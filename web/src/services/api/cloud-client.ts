import { createExternalClient, requestExternal, ExternalApiError } from "./external-client";

/** 独立云服务使用自己的 origin 与 Bearer 会话，不复用本地 `http` 客户端。 */
export { ExternalApiError as CloudApiError };

export type CloudAccount = {
    accountId: string;
    points: number;
    reservedPoints?: number;
    availablePoints?: number;
};

export type CloudLedgerEntry = {
    id: string;
    kind: string;
    points: number;
    createdAt: string;
};

export type CloudModel = {
    id: string;
    displayName: string;
    capabilities: string[];
    enabled: boolean;
    priceVersion: string;
    maxInputTokens: number;
    maxOutputTokens: number;
    inputPointsPerMillion: number;
    outputPointsPerMillion: number;
};

export type CloudLoginResult = {
    account: { id: string; createdAt: string };
    token: string;
    expiresAt: string;
};

export type CloudWeChatAuthorization = {
    authorizationURL: string;
    expiresAt: string;
};

export type CloudHandoff = {
    code: string;
    state: string;
};

export type CloudTask = {
    id: string;
    modelId: string;
    status: "queued" | "reserved" | "running" | "completed" | "failed" | "pending-reconciliation";
    reservedPoints: number;
    chargedPoints: number;
    responseText?: string;
    errorCode?: string;
};

export function normalizeCloudApiBaseURL(value: string) {
    const raw = value.trim().replace(/\/+$/u, "");
    if (!raw) throw new Error("请输入云服务 API 地址");
    let parsed: URL;
    try {
        parsed = new URL(raw);
    } catch {
        throw new Error("云服务 API 地址格式无效");
    }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") throw new Error("云服务 API 地址必须使用 HTTP 或 HTTPS");
    if (parsed.username || parsed.password || parsed.search || parsed.hash) throw new Error("云服务 API 地址不能包含凭据、查询参数或片段");
    return raw;
}

export function defaultCloudApiBaseURL() {
    return String(import.meta.env.VITE_LUCKYTV_CLOUD_API_URL || "http://127.0.0.1:8081/api/v1").replace(/\/+$/u, "");
}

export function createCloudClient(baseURL: string, token?: string) {
    return createExternalClient({ baseURL: normalizeCloudApiBaseURL(baseURL), token });
}

export function requestCloudEmailCode(baseURL: string, email: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL).post<{ status: string }>("/auth/email/code", { email }, { signal }));
}

export function verifyCloudEmailCode(baseURL: string, email: string, code: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL).post<CloudLoginResult>("/auth/email/verify", { email, code }, { signal }));
}

export function requestCloudWeChatAuthorization(baseURL: string, handoffURL: string, clientState: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL).get<CloudWeChatAuthorization>("/auth/wechat/start", { params: { handoffURL, clientState }, signal }));
}

export function exchangeCloudWeChatHandoff(baseURL: string, handoff: CloudHandoff, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL).post<CloudLoginResult>("/auth/wechat/handoff", handoff, { signal }));
}

export function getCloudAccount(baseURL: string, token: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL, token).get<CloudAccount>("/account", { signal }));
}

export function getCloudLedger(baseURL: string, token: string, signal?: AbortSignal) {
    return requestExternal(
        createCloudClient(baseURL, token).get<{ balance: number; reservedPoints: number; availablePoints: number; entries: CloudLedgerEntry[] }>("/ledger", { signal }),
    );
}

export function getCloudModels(baseURL: string, token: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL, token).get<{ models: CloudModel[] }>("/models", { signal }));
}

export function revokeCloudSession(baseURL: string, token: string, signal?: AbortSignal) {
    return requestExternal(createCloudClient(baseURL, token).post<void>("/sessions/revoke", undefined, { signal }));
}
import axios, { type AxiosInstance } from "axios";

/**
 * 外部服务客户端（独立 origin、独立 Bearer 凭据、独立响应信封）。
 *
 * 本地业务 API 必须走 `http`（`services/api/request.ts`）：它绑定本机 baseUrl、
 * 注入桌面 launch token 并解包 `{ code, data, msg }`。独立云服务不共享这些前提，
 * 因此所有 `axios.create` 集中在这里，由 `web/test/http-ownership.test.ts` 守住。
 */
export class ExternalApiError extends Error {
    readonly status?: number;
    readonly reason?: string;
    readonly cause?: unknown;

    constructor(message: string, options: { status?: number; reason?: string; cause?: unknown } = {}) {
        super(message);
        this.name = "ExternalApiError";
        this.status = options.status;
        this.reason = options.reason;
        this.cause = options.cause;
    }
}

export function createExternalClient(options: { baseURL: string; token?: string; timeout?: number }): AxiosInstance {
    return axios.create({
        baseURL: options.baseURL,
        timeout: options.timeout ?? 8_000,
        headers: options.token ? { Authorization: `Bearer ${options.token}` } : undefined,
    });
}

/** 把外部服务的 axios 失败统一成可读错误；调用方按 status/reason 判断类型。 */
export async function requestExternal<T>(request: Promise<{ data: T }>): Promise<T> {
    try {
        return (await request).data;
    } catch (error) {
        if (axios.isAxiosError(error)) {
            const body = error.response?.data as { error?: string; message?: string } | undefined;
            throw new ExternalApiError(body?.message || body?.error || "外部服务请求失败", {
                status: error.response?.status,
                reason: body?.error,
                cause: error,
            });
        }
        throw error;
    }
}
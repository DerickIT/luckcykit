import type { ModelCapabilityConfig } from "@/lib/model-capabilities";
import type { ModelProtocol } from "@/lib/model-protocols";
import { http } from "@/services/api/request";
import type { WorkspaceCapabilityContract } from "@/services/workspace-mode";
import { defaultConfig, type AiConfig } from "@/stores/use-config-store";
import type { FeatureAvailability, LocalUser, RuntimeLimits } from "@/stores/use-user-store";
import { isBrowserLocalRuntime } from "@/lib/runtime-mode";
import { getActiveUserScope } from "@/lib/user-scope";

export type ModelCapabilityDraftInput = {
    capability?: string;
    category?: string;
    type?: string;
    protocol?: ModelProtocol | string;
    interfaceType?: ModelProtocol | string;
    model: string;
    baseUrl?: string;
    channelId?: string;
    videoCapabilitiesVersion?: string;
    capabilityConfig?: ModelCapabilityConfig;
};

export function normalizeModelCapabilityDraft(draft: ModelCapabilityDraftInput) {
    if (isBrowserLocalRuntime()) {
        const capabilityConfig = draft.capabilityConfig || {};
        return Promise.resolve({ ...capabilityConfig, capabilityConfig } as ModelCapabilityConfig & { capabilityConfig?: ModelCapabilityConfig });
    }
    return http.post<ModelCapabilityConfig & { capabilityConfig?: ModelCapabilityConfig }>("/workspace/model-config/capability", draft);
}

export type LocalWorkspace = {
    id: string;
    name: string;
    owner: "local";
    storage: "sqlite" | "indexeddb";
};

export type WorkspaceBootstrapPayload = {
    contractVersion: number;
    profile: "local";
    capabilities: WorkspaceCapabilityContract["capabilities"];
    user: LocalUser;
    workspace: LocalWorkspace;
    storageMode: "local";
    runtimeLimits?: RuntimeLimits;
    features?: FeatureAvailability;
};

function browserLocalUser(): LocalUser {
    const scope = getActiveUserScope();
    return {
        id: scope,
        username: "local",
        displayName: "本地工作区",
        role: "user",
        status: "active",
        identityProvider: "local",
        createdAt: "",
        updatedAt: "",
    };
}

function browserLocalBootstrap(): WorkspaceBootstrapPayload {
    const scope = getActiveUserScope();
    return {
        contractVersion: 1,
        profile: "local",
        capabilities: { localAssets: true, providerCalls: true },
        user: browserLocalUser(),
        workspace: { id: scope, name: "本地浏览器工作区", owner: "local", storage: "indexeddb" },
        storageMode: "local",
        features: {
            shortDramaEnabled: true,
            taskCenterEnabled: true,
            customChannelsEnabled: true,
            frontendModelsEnabled: false,
            pluginCenterEnabled: true,
            systemPluginsVisibleToUsers: true,
        },
    };
}

export function getWorkspaceBootstrap() {
    return isBrowserLocalRuntime()
        ? Promise.resolve(browserLocalBootstrap())
        : http.get<WorkspaceBootstrapPayload>("/workspace/bootstrap");
}

export function getLocalModelConfig() {
    if (isBrowserLocalRuntime()) {
        return Promise.resolve({ config: defaultConfig, revision: 0, health: "default" as const, source: "builtin+local" as const });
    }
    return http.get<LocalModelConfigPayload>("/workspace/model-config");
}

export type LocalModelConfigPayload = {
	config: AiConfig;
	revision: number;
	health: "ready" | "default" | "migrated" | "recovered";
	source: "builtin+local";
};

export function saveLocalModelConfig(config: AiConfig, expectedRevision: number) {
	if (isBrowserLocalRuntime()) return Promise.resolve({ saved: true, revision: expectedRevision });
	return http.put<{ saved: boolean; revision: number }>("/workspace/model-config", { config, expectedRevision });
}

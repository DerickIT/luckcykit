import { canonicalChannelModel, filterModelsByCapability, modelOptionsFromChannels, type AiConfig, type ModelChannel } from "@/stores/use-config-store";

export function shouldRefreshLuckyTVCatalog(previous: string | undefined, next: string) {
    if (next === "connected" && previous !== "connected") return true;
    return next === "disconnected" && Boolean(previous) && previous !== "disconnected";
}

export function mergeManagedLuckyTVCatalog(current: AiConfig, server: AiConfig, adoptAuthorizedAssistant = false): AiConfig {
    const serverLucky = server.channels.find((channel) => channel.id === "luckytv");
    const serverIds = new Set(server.channels.map((channel) => channel.id));
    let found = false;
    // 固定服务由后端维护，重连后移除的旧官方服务也从本地目录移除。
    const channels = current.channels.filter((channel) => !channel.pinned || serverIds.has(channel.id)).map((channel) => {
        if (channel.id !== "luckytv") return channel;
        found = true;
        if (!serverLucky) {
            return { ...channel, models: [], modelProfiles: [], apiKey: "", secretKey: "", credentialRef: undefined, hasApiKey: false, hasSecretKey: false };
        }
        return {
            ...channel,
            models: [...(serverLucky.models || [])],
            modelProfiles: (serverLucky.modelProfiles || []).map((item) => ({ ...item })),
            apiKey: "",
            secretKey: "",
            credentialRef: serverLucky.credentialRef,
            legacyIds: serverLucky.legacyIds,
            hasApiKey: serverLucky.hasApiKey,
            hasSecretKey: serverLucky.hasSecretKey,
            baseUrl: serverLucky.baseUrl || channel.baseUrl,
        };
    });
    if (serverLucky && !found) {
        channels.unshift({
            ...serverLucky,
            apiKey: "",
            secretKey: "",
            models: [...(serverLucky.models || [])],
            modelProfiles: (serverLucky.modelProfiles || []).map((item) => ({ ...item })),
        });
    }
    return withChannels(adoptAuthorizedAssistant ? { ...current, assistantModel: server.assistantModel } : current, channels);
}

export function withChannels(config: AiConfig, channels: ModelChannel[]): AiConfig {
    const canonical = (value: string) => canonicalChannelModel(value, channels);
    config = { ...config, model: canonical(config.model), assistantModel: canonical(config.assistantModel), mediaReviewModel: canonical(config.mediaReviewModel), imageModel: canonical(config.imageModel), videoModel: canonical(config.videoModel), textModel: canonical(config.textModel), audioModel: canonical(config.audioModel) };
    const models = modelOptionsFromChannels(channels);
    const imageModels = filterModelsByCapability(models, "image", channels);
    const videoModels = filterModelsByCapability(models, "video", channels);
    const textModels = filterModelsByCapability(models, "text", channels);
    const audioModels = filterModelsByCapability(models, "audio", channels);
    return {
        ...config,
        channels,
        models,
        model: normalizeDefaultModel(config.model, models),
        assistantModel: config.channels.some((channel) => !channels.some((item) => item.id === channel.id) && config.assistantModel?.startsWith(`${channel.id}::`)) ? "" : config.assistantModel,
        mediaReviewModel: config.channels.some((channel) => !channels.some((item) => item.id === channel.id) && config.mediaReviewModel?.startsWith(`${channel.id}::`)) ? "" : config.mediaReviewModel,
        baseUrl: channels[0]?.baseUrl || config.baseUrl,
        apiKey: channels[0]?.apiKey || config.apiKey,
        apiFormat: channels[0]?.apiFormat || config.apiFormat,
        imageModels,
        videoModels,
        textModels,
        audioModels,
        imageModel: normalizeDefaultModel(config.imageModel, imageModels),
        videoModel: normalizeDefaultModel(config.videoModel, videoModels),
        textModel: normalizeDefaultModel(config.textModel, textModels),
        audioModel: normalizeDefaultModel(config.audioModel, audioModels),
    };
}

function normalizeDefaultModel(value: string, options: string[]) {
    return options.includes(value) ? value : options[0] || "";
}

import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test } from "bun:test";
import { CanvasConfigComposer } from "../src/components/canvas/canvas-config-composer";
import { CanvasNodePromptPanel } from "../src/components/canvas/canvas-node-prompt-panel";
import { createModelChannel, defaultConfig, normalizeConfigSnapshot } from "../src/stores/use-config-store";
import { CanvasNodeType } from "../src/types/canvas";

function fixtureConfig() {
    const channel = createModelChannel({
        id: "luckytv",
        name: "LuckyTV",
        scope: "system",
        pinned: true,
        credentialRef: "luckytv",
        baseUrl: "https://luckytv.app/v1",
        models: ["wan3.0-video", "gpt-image-1"],
    });
    return normalizeConfigSnapshot({
        config: {
            ...defaultConfig,
            channels: [channel],
            model: "luckytv::wan3.0-video",
            videoModel: "luckytv::wan3.0-video",
            imageModel: "luckytv::gpt-image-1",
        },
    }).config;
}

test("all canvas prompt footers omit the preset trigger", () => {
    const config = fixtureConfig();
    const panelHtml = renderToStaticMarkup(
        createElement(
            QueryClientProvider,
            { client: new QueryClient() },
            createElement(CanvasNodePromptPanel, {
                config,
                projectId: "proj-1",
                node: { id: "vid-1", type: CanvasNodeType.Video, title: "Video", position: { x: 0, y: 0 }, width: 320, height: 180, metadata: { prompt: "镜头" } },
                isRunning: false,
                onPromptChange: () => {},
                onConfigChange: () => {},
                onGenerate: () => {},
            }),
        ),
    );
    const panelFooter = panelHtml.slice(panelHtml.indexOf("canvas-node-composer-footer"));
    expect(panelHtml).toContain("canvas-preset-picker-trigger");
    expect(panelFooter).not.toContain("canvas-preset-picker-trigger");

    const composerHtml = renderToStaticMarkup(
        createElement(CanvasConfigComposer, {
            config,
            generationMode: "video",
            model: "luckytv::wan3.0-video",
            value: "镜头",
            inputs: [],
            onChange: () => {},
            onClose: () => {},
        }),
    );
    expect(composerHtml).not.toContain("canvas-preset-picker-trigger");
});

test("video prompt tools move camera control below the prompt", () => {
    const config = fixtureConfig();
    const panelHtml = renderToStaticMarkup(
        createElement(
            QueryClientProvider,
            { client: new QueryClient() },
            createElement(CanvasNodePromptPanel, {
                config,
                projectId: "proj-1",
                node: { id: "vid-1", type: CanvasNodeType.Video, title: "Video", position: { x: 0, y: 0 }, width: 320, height: 180, metadata: { prompt: "镜头" } },
                isRunning: false,
                onPromptChange: () => {},
                onConfigChange: () => {},
                onGenerate: () => {},
            }),
        ),
    );
    const beforeEditor = panelHtml.slice(0, panelHtml.indexOf("canvas-node-composer-editor"));
    const footer = panelHtml.slice(panelHtml.indexOf("canvas-node-composer-footer"));
    expect(beforeEditor).not.toContain(">参考<");
    expect(beforeEditor).not.toContain(">标记<");
    expect(beforeEditor).not.toContain(">特效<");
    expect(beforeEditor).not.toContain(">角色库<");
    expect(footer).toContain("运镜");
});

import { expect, test } from "bun:test";

import { dramasPath, projectChapterPath, projectPath, projectViewPath, projectWorkflowPath } from "../src/lib/project-routes";
import { workspaceNavEntriesFor } from "../src/lib/workspace-navigation";

test("short-drama routes keep a separate namespace from the canvas library", () => {
    expect(dramasPath()).toBe("/dramas");
    expect(projectPath("project 1")).toBe("/dramas/project%201");
    expect(projectViewPath("p1", "assets")).toBe("/dramas/p1/assets");
    expect(projectChapterPath("p1")).toBe("/dramas/p1/chapters");
    expect(projectChapterPath("p1", "c1")).toBe("/dramas/p1/chapters/c1");
    expect(projectWorkflowPath("p1")).toBe("/dramas/p1/workflow");
    expect(projectWorkflowPath("p1", "u1")).toBe("/dramas/p1/workflow/u1/video");
    expect(projectWorkflowPath("p1", "u1", "storyboard")).toBe("/dramas/p1/workflow/u1/storyboard");
});

test("navigation entries gate short-drama and plugin destinations per surface", () => {
    const enabled = workspaceNavEntriesFor("palette", { shortDramaEnabled: true, pluginCenterEnabled: true });
    expect(enabled.find((entry) => entry.id === "dramas")?.to).toBe("/dramas");
    expect(enabled.find((entry) => entry.id === "canvas")?.to).toBe("/project");
    expect(enabled.some((entry) => entry.id === "plugins")).toBe(true);

    const disabled = workspaceNavEntriesFor("palette", { shortDramaEnabled: false, pluginCenterEnabled: false });
    expect(disabled.some((entry) => entry.id === "dramas")).toBe(false);
    expect(disabled.some((entry) => entry.id === "plugins")).toBe(false);

    const sidebar = workspaceNavEntriesFor("sidebar", { shortDramaEnabled: true, pluginCenterEnabled: true });
    expect(sidebar.some((entry) => entry.id === "settings:channels")).toBe(true);
    expect(sidebar.some((entry) => entry.id === "agents")).toBe(true);
    expect(sidebar.some((entry) => entry.id === "settings")).toBe(false);
});
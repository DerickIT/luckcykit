import type { ComponentType } from "react";
import { Blocks, Clapperboard, HardDrive, Home, Images, LibraryBig, PanelsTopLeft, Plug, Plus, Settings, Settings2 } from "lucide-react";

import { dramasPath } from "@/lib/project-routes";

type NavIcon = ComponentType<{ className?: string; strokeWidth?: number }>;
type NavFeature = "shortDramaEnabled" | "pluginCenterEnabled";
type NavSurface = "sidebar" | "palette";

export type WorkspaceNavEntry = {
    id: string;
    title: string;
    to: string;
    icon: NavIcon;
    feature?: NavFeature;
    surfaces: readonly NavSurface[];
};

/**
 * Single source for workspace navigation. The sidebar and command palette both
 * filter this list, so a new destination or feature gate is declared once.
 */
export const workspaceNavEntries: readonly WorkspaceNavEntry[] = [
    { id: "new", title: "新建项目", to: "/canvas?mode=new", icon: Plus, surfaces: ["sidebar"] },
    { id: "home", title: "首页", to: "/", icon: Home, surfaces: ["sidebar", "palette"] },
    { id: "dramas", title: "短剧项目", to: dramasPath(), icon: Clapperboard, feature: "shortDramaEnabled", surfaces: ["sidebar", "palette"] },
    { id: "canvas", title: "画布库", to: "/project", icon: PanelsTopLeft, surfaces: ["sidebar", "palette"] },
    { id: "assets", title: "资产", to: "/assets", icon: Images, surfaces: ["sidebar", "palette"] },
    { id: "skills", title: "技能", to: "/skills", icon: LibraryBig, surfaces: ["sidebar", "palette"] },
    { id: "plugins", title: "插件", to: "/plugins", icon: Blocks, feature: "pluginCenterEnabled", surfaces: ["sidebar", "palette"] },
    { id: "settings:channels", title: "模型配置", to: "/settings?section=channels", icon: Settings2, surfaces: ["sidebar"] },
    { id: "settings:storage", title: "存储设置", to: "/settings?section=storage", icon: HardDrive, surfaces: ["sidebar"] },
    { id: "settings", title: "设置", to: "/settings", icon: Settings, surfaces: ["palette"] },
    { id: "agents", title: "LuckyTV MCP", to: "/agents", icon: Plug, surfaces: ["sidebar"] },
];

export function workspaceNavEntriesFor(surface: NavSurface, features: Record<NavFeature, boolean>) {
    return workspaceNavEntries.filter((entry) => entry.surfaces.includes(surface) && (!entry.feature || features[entry.feature]));
}
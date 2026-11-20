import type { ProjectSummary } from "@/services/api/projects";

export type ProjectActionTone = "default" | "attention" | "danger";

export type ProjectWorkbenchAction = {
    id: string;
    title: string;
    description: string;
    href: string;
    actionLabel: string;
    tone: ProjectActionTone;
};

export type ProjectStageCell = {
    label: string;
    state: "idle" | "active" | "attention" | "completed";
};

/** Chapter and shot order is a domain rule: read it from `position`, never insertion order. */
export function orderedByPosition<T extends { position: number }>(items: readonly T[]): T[] {
    return [...items].sort((left, right) => left.position - right.position);
}

export function projectSummaryCompletion(summary: ProjectSummary) {
    return summary.unitCount ? Math.round((summary.completedUnitCount / summary.unitCount) * 100) : 0;
}

export function projectSummaryStage(summary: ProjectSummary) {
    if (summary.project.status === "archived") return { label: "已归档", detail: "可在项目设置中恢复" };
    if (!summary.unitCount) return { label: "准备故事", detail: "还没有剧情章节" };
    if (summary.completedUnitCount === summary.unitCount) return { label: "章节已完成", detail: "可继续检查镜头与交付结果" };
    if (!summary.canvasCount) return { label: "组织章节", detail: "下一步可建立项目画布" };
    if (!summary.assetCount) return { label: "准备资产", detail: "补充角色、场景或画风" };
    return { label: "制作中", detail: `${summary.completedUnitCount}/${summary.unitCount} 章已完成` };
}
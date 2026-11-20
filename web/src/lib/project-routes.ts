/**
 * Short-drama project route namespace.
 *
 * The drama workbench lives under `/dramas`; `/project`, `/projects` and `/canvas`
 * remain canvas-library routes. Build project links here so a future route change
 * touches one file instead of every page.
 */
const DRAMAS_ROOT = "/dramas";

export function dramasPath() {
    return DRAMAS_ROOT;
}

export function projectPath(projectId: string) {
    return `${DRAMAS_ROOT}/${encodeURIComponent(projectId)}`;
}

export function projectViewPath(projectId: string, view: string) {
    return `${projectPath(projectId)}/${encodeURIComponent(view)}`;
}

export function projectChapterPath(projectId: string, chapterId?: string) {
    const root = `${projectPath(projectId)}/chapters`;
    return chapterId ? `${root}/${encodeURIComponent(chapterId)}` : root;
}

export function projectWorkflowPath(projectId: string, unitId?: string, stage = "video") {
    const root = `${projectPath(projectId)}/workflow`;
    return unitId ? `${root}/${encodeURIComponent(unitId)}/${encodeURIComponent(stage)}` : root;
}
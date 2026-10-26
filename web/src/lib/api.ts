import type {
  KnowledgeDoc, KnowledgeMeta, MainPromptMeta, RoleMeta,
  AppConfig, Approval, Asset, AuditEntry, Coverage, Dashboard, Evidence, Event, FindingView,
  Goal, MCPStatus, Message, ModeMeta, Project, ProjectDetail, ProjectEdge, ProjectFinding,
  ProjectMemory, ProjectNode, ProjectResponse, ProjectSummary, Run, RunDetail, RunSummary,
  SkillInfo, SnapshotView, Task, TaskDetail, AuthStatus,
} from "../types";

function arr<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeRunDetail(detail: RunDetail): RunDetail {
  return {
    ...detail,
    goals: arr(detail.goals),
    constraints: arr(detail.constraints),
    coverage: normalizeCoverage(detail.coverage),
  };
}

function normalizeCoverage(coverage: Coverage | null | undefined): Coverage {
  return {
    total: coverage?.total ?? 0,
    tested: coverage?.tested ?? 0,
    untested: arr(coverage?.untested),
    assets: arr(coverage?.assets),
  };
}

function normalizeSnapshot(view: SnapshotView): SnapshotView {
  const snapshot = view.snapshot;
  return {
    ...view,
    snapshot: {
      ...snapshot,
      goals: arr(snapshot.goals),
      constraints: arr(snapshot.constraints),
      nodes: arr(snapshot.nodes),
      edges: arr(snapshot.edges),
      anchors: arr(snapshot.anchors),
      assets: arr(snapshot.assets),
      evidence: arr(snapshot.evidence),
      flows: arr(snapshot.flows),
      retests: arr(snapshot.retests),
      events: arr(snapshot.events),
      tasks: arr(snapshot.tasks),
    },
    coverage: normalizeCoverage(view.coverage),
    review: { items: arr(view.review?.items) },
  };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const text = await res.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: text };
    }
  }
  if (!res.ok) {
    throw new Error(data?.error || `${res.status} ${res.statusText}`);
  }
  return data as T;
}

export const api = {
  health: () => request<{ ok: boolean; version: string; uptimeSeconds: number }>("/api/health"),
  modes: () => request<{ modes: ModeMeta[] }>("/api/modes").then((r) => arr(r.modes)),
  knowledge: (role: string) =>
    request<{ docs: KnowledgeMeta[] }>(`/api/agents/${encodeURIComponent(role)}/knowledge`).then((r) => arr(r.docs)),
  knowledgeDoc: (role: string, name: string) =>
    request<KnowledgeDoc>(`/api/agents/${encodeURIComponent(role)}/knowledge/${encodeURIComponent(name)}`),
  saveKnowledge: (role: string, payload: { name: string; description: string; body: string }) =>
    request<KnowledgeDoc>(`/api/agents/${encodeURIComponent(role)}/knowledge`, { method: "POST", body: JSON.stringify(payload) }),
  agentCatalog: () => request<{ roles: RoleMeta[]; modes: MainPromptMeta[] }>("/api/agents/catalog").then((r) => ({ roles: arr(r.roles), modes: arr(r.modes) })),
  config: () => request<AppConfig>("/api/config"),
  saveConfig: (cfg: AppConfig) =>
    request<AppConfig>("/api/config", { method: "PUT", body: JSON.stringify(cfg) }),
  skills: () => request<{ skills: SkillInfo[] }>("/api/skills").then((r) => arr(r.skills)),
  skill: (name: string) =>
    request<{ skill: SkillInfo }>(`/api/skills/${encodeURIComponent(name)}`).then((r) => r.skill),
  saveSkill: (body: { name: string; description: string; body: string }) =>
    request<{ skill: SkillInfo }>("/api/skills", { method: "POST", body: JSON.stringify(body) }).then((r) => r.skill),
  mcp: () => request<{ servers: MCPStatus[] }>("/api/mcp").then((r) => arr(r.servers)),
  reconnectMCP: () =>
    request<{ ok: boolean; servers: import("../types").MCPStatus[]; error?: string }>("/api/mcp/reconnect", { method: "POST" }),
  testMCP: (server: import("../types").MCPServerConfig) =>
    request<{ ok: boolean; status: MCPStatus; error?: string }>("/api/mcp/test", {
      method: "POST",
      body: JSON.stringify(server),
    }),

  runs: () => request<{ runs: RunSummary[] }>("/api/runs").then((r) => arr(r.runs)),
  run: (id: string) => request<RunDetail>(`/api/runs/${id}`).then(normalizeRunDetail),
  createRun: (body: Record<string, unknown>) =>
    request<{ run: Run; goal: Goal }>("/api/runs", { method: "POST", body: JSON.stringify(body) }),
  messages: (id: string) =>
    request<{ messages: Message[]; running: boolean }>(`/api/runs/${id}/messages`).then((r) => ({
      messages: arr(r.messages),
      running: r.running,
    })),
  send: (id: string, text: string) =>
    request<{ ok: boolean }>(`/api/runs/${id}/messages`, {
      method: "POST",
      body: JSON.stringify({ text }),
    }),
  stop: (id: string) => request<{ ok: boolean }>(`/api/runs/${id}/stop`, { method: "POST" }),
  resume: (id: string) => request<{ ok: boolean }>(`/api/runs/${id}/resume`, { method: "POST" }),
  snapshot: (id: string) => request<SnapshotView>(`/api/runs/${id}/snapshot`).then(normalizeSnapshot),
  findings: (id: string) =>
    request<{ findings: FindingView[] }>(`/api/runs/${id}/findings`).then((r) => arr(r.findings)),
  assets: (id: string) =>
    request<{ coverage: Coverage; assets: Asset[] }>(`/api/runs/${id}/assets`).then((r) => ({
      coverage: normalizeCoverage(r.coverage),
      assets: arr(r.assets),
    })),
  evidence: (id: string) =>
    request<{ evidence: Evidence[] }>(`/api/runs/${id}/evidence`).then((r) => arr(r.evidence)),
  report: (id: string) =>
    request<{ markdown: string }>(`/api/runs/${id}/report`).then((r) => r.markdown),
  events: (id: string) =>
    request<{ events: Event[] }>(`/api/runs/${id}/events`).then((r) => arr(r.events)),
  tasks: (id: string) =>
    request<{ tasks: Task[] }>(`/api/runs/${id}/tasks`).then((r) => arr(r.tasks)),
  approvals: (id: string) =>
    request<{ pending: Approval[]; recent: Approval[] }>(`/api/runs/${id}/approvals`).then((r) => ({
      pending: arr(r.pending),
      recent: arr(r.recent),
    })),
  decide: (id: string, approve: boolean) =>
    request<Approval>(`/api/approvals/${id}/decide`, {
      method: "POST",
      body: JSON.stringify({ approve }),
    }),
  addGoal: (id: string, text: string, acceptance: string) =>
    request<Goal>(`/api/runs/${id}/goals`, {
      method: "POST",
      body: JSON.stringify({ text, acceptance }),
    }),
  audit: (id: string) =>
    request<{ audit: AuditEntry[] }>(`/api/runs/${id}/audit`).then((r) => arr(r.audit)),

  dashboard: () => request<Dashboard>("/api/dashboard"),
  projects: () =>
    request<{ projects: ProjectSummary[] }>("/api/projects").then((r) => arr(r.projects)),
  createProject: (body: Record<string, unknown>) =>
    request<Project>("/api/projects", { method: "POST", body: JSON.stringify(body) }),
  project: (id: string) => request<ProjectResponse>(`/api/projects/${id}`),
  updateProject: (id: string, body: Record<string, unknown>) =>
    request<Project>(`/api/projects/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteProject: (id: string) =>
    request<{ deleted: string }>(`/api/projects/${id}`, { method: "DELETE" }),
  projectConversations: (id: string) =>
    request<{ conversations: Run[]; running: Record<string, boolean> }>(
      `/api/projects/${id}/conversations`,
    ),
  createProjectConversation: (id: string, body: Record<string, unknown>) =>
    request<{ run: Run; goal: Goal }>(`/api/projects/${id}/conversations`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  projectBoard: (id: string) =>
    request<{ nodes: ProjectNode[]; edges: ProjectEdge[]; memory: ProjectMemory[] }>(
      `/api/projects/${id}/board`,
    ),
  writeBoard: (id: string, body: Record<string, unknown>) =>
    request<{ node: ProjectNode; memory: ProjectMemory | null }>(`/api/projects/${id}/board`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  projectMemory: (id: string) =>
    request<{ memory: ProjectMemory[] }>(`/api/projects/${id}/memory`).then((r) => arr(r.memory)),
  writeMemory: (id: string, body: Record<string, unknown>) =>
    request<ProjectMemory>(`/api/projects/${id}/memory`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  allFindings: () =>
    request<{ findings: ProjectFinding[] }>("/api/findings").then((r) => arr(r.findings)),
  setFindingStatus: (id: string, state: string) =>
    request<{ id: string; state: string }>(`/api/findings/${id}/status`, {
      method: "POST",
      body: JSON.stringify({ state }),
    }),
  allAssets: () => request<{ assets: Asset[] }>("/api/assets").then((r) => arr(r.assets)),
  allTasks: () => request<{ tasks: Task[] }>("/api/tasks").then((r) => arr(r.tasks)),
  taskDetail: (id: string) => request<TaskDetail>(`/api/tasks/${id}`),
  cancelTask: (id: string) =>
    request<{ ok: boolean; id: string }>(`/api/tasks/${id}/cancel`, { method: "POST" }),
  allApprovals: (status = "pending") =>
    request<{ approvals: Approval[] }>(`/api/approvals?status=${encodeURIComponent(status)}`).then((r) =>
      arr(r.approvals),
    ),

  authStatus: () => request<AuthStatus>("/api/auth/status"),
  login: (password: string) =>
    request<AuthStatus>("/api/auth/login", { method: "POST", body: JSON.stringify({ password }) }),
  logout: () => request<{ ok: boolean }>("/api/auth/logout", { method: "POST" }),
  setPassword: (body: Record<string, unknown>) =>
    request<AuthStatus>("/api/auth/password", { method: "POST", body: JSON.stringify(body) }),
};

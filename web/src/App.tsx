import { ArrowLeft, Bell, Moon, PanelLeftOpen, PanelRightClose, PanelRightOpen, Search, Settings2, ShieldAlert, Sun } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import Chat from "./components/Chat";
import CommandPalette from "./components/CommandPalette";
import { NewRunDialog, SettingsDialog } from "./components/Dialogs";
import Inspector, { type InspectorTab } from "./components/Inspector";
import {
  AgentsPage, ApprovalsPage, AssetsPage, ConversationsPage, DashboardPage, EditProjectDialog,
  FindingsPage, LoginPage, MCPPage, NewProjectDialog, ProjectPage, ProjectsPage, ShellSidebar,
  SkillsPage, TasksPage, viewLabels, type View,
} from "./components/pages";
import { Empty, Spinner } from "./components/ui";
import { api } from "./lib/api";
import { statusLabels, statusTone } from "./lib/format";
import type {
  AppConfig, Approval, Asset, AuditEntry, Coverage, Dashboard, Event, Evidence, FindingView,
  AuthStatus, MCPStatus, MainPromptMeta, Message, ModeMeta, ProjectDetail, ProjectFinding, ProjectSummary,
  RoleMeta, RunDetail, RunSummary, SkillInfo, SnapshotView, Task,
} from "./types";

type Toast = { tone: "error" | "info"; text: string };

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export default function App() {
  const [modes, setModes] = useState<ModeMeta[]>([]);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [health, setHealth] = useState<{ version: string } | null>(null);
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [allAssets, setAllAssets] = useState<Asset[]>([]);
  const [allFindings, setAllFindings] = useState<ProjectFinding[]>([]);
  const [allTasks, setAllTasks] = useState<Task[]>([]);
  const [allApprovals, setAllApprovals] = useState<Approval[]>([]);
  const [theme, setTheme] = useState<"light" | "dark">(() => (localStorage.getItem("lingeye-theme") === "dark" ? "dark" : "light"));
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [notifyOpen, setNotifyOpen] = useState(false);
  const [newRunMode, setNewRunMode] = useState<string | undefined>(undefined);
  const [activeProjectId, setActiveProjectId] = useState<string | null>(null);
  const [projectDetail, setProjectDetail] = useState<ProjectDetail | null>(null);
  const [projectRunning, setProjectRunning] = useState<Record<string, boolean>>({});

  const [activeId, setActiveId] = useState<string | null>(null);
  const [detail, setDetail] = useState<RunDetail | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [snapView, setSnapView] = useState<SnapshotView | null>(null);
  const [findings, setFindings] = useState<FindingView[]>([]);
  const [coverage, setCoverage] = useState<Coverage | null>(null);
  const [evidence, setEvidence] = useState<Evidence[]>([]);
  const [report, setReport] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [streamingText, setStreamingText] = useState("");
  const [reasoningText, setReasoningText] = useState("");
  const [taskStreams, setTaskStreams] = useState<Record<string, string>>({});
  const [mcpStatus, setMcpStatus] = useState<MCPStatus[]>([]);
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [roles, setRoles] = useState<RoleMeta[]>([]);
  const [mainPrompts, setMainPrompts] = useState<MainPromptMeta[]>([]);

  const [view, setView] = useState<View>("dashboard");
  const [tab, setTab] = useState<InspectorTab>("graph");
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [sidebar, setSidebar] = useState(() => window.innerWidth >= 720);
  const [inspector, setInspector] = useState(() => window.innerWidth >= 1080);
  const [compactInspector, setCompactInspector] = useState(() => window.innerWidth < 1080);
  const [narrow, setNarrow] = useState(() => window.innerWidth < 720);
  const inspectorSqueezes = !narrow && window.innerWidth < 1600;
  const [showNew, setShowNew] = useState(false);
  const [newRunProjectId, setNewRunProjectId] = useState<string | undefined>(undefined);
  const [showNewProject, setShowNewProject] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showEditProject, setShowEditProject] = useState(false);
  const [busy, setBusy] = useState(false);
  const [toast, setToast] = useState<Toast | null>(null);
  const refreshTimer = useRef<number | undefined>(undefined);
  const requestSeq = useRef(0);

  const notify = useCallback((tone: Toast["tone"], text: string) => setToast({ tone, text }), []);

  const loadRuns = useCallback(async () => setRuns(await api.runs()), []);
  const loadProjects = useCallback(async () => setProjects(await api.projects()), []);
  const loadDashboard = useCallback(async () => setDashboard(await api.dashboard()), []);
  const loadAllAssets = useCallback(async () => setAllAssets(await api.allAssets()), []);
  const loadAllFindings = useCallback(async () => setAllFindings(await api.allFindings()), []);
  const loadAllTasks = useCallback(async () => setAllTasks(await api.allTasks()), []);
  const loadAllApprovals = useCallback(async () => setAllApprovals(await api.allApprovals()), []);

  const loadProject = useCallback(async (id: string) => {
    const response = await api.project(id);
    setProjectDetail(response.project);
    setProjectRunning(response.running);
  }, []);

  const loadAux = useCallback(async () => {
    try {
      const [mcp, skillList] = await Promise.all([api.mcp(), api.skills()]);
      setMcpStatus(mcp);
      setSkills(skillList);
    } catch {
      /* best effort */
    }
  }, []);

  const refresh = useCallback(
    async (id: string) => {
      const seq = ++requestSeq.current;
      try {
        const [d, m, s, f, a, ev, t, ap, au, e] = await Promise.all([
          api.run(id), api.messages(id), api.snapshot(id), api.findings(id),
          api.assets(id), api.evidence(id), api.tasks(id), api.approvals(id),
          api.audit(id), api.events(id),
        ]);
        if (seq !== requestSeq.current) return;
        setDetail(d);
        setMessages(m.messages);
        setSnapView(s);
        setFindings(f);
        setCoverage(a.coverage);
        setEvidence(ev);
        setTasks(t);
        setApprovals(ap.pending);
        setAudit(au);
        setEvents(e);
      } catch (error) {
        notify("error", messageOf(error));
      }
    },
    [notify],
  );

  useEffect(() => {
    void (async () => {
      try {
        const status = await api.authStatus();
        setAuth(status);
        if (status.enabled && !status.authenticated) return;
        const [modeList, cfg, projectList, dash, assets, findingList, taskList, skillList, mcp, h, approvalList, catalog] =
          await Promise.all([
            api.modes(), api.config(), api.projects(), api.dashboard(), api.allAssets(),
            api.allFindings(), api.allTasks(), api.skills(), api.mcp(), api.health(), api.allApprovals(),
            api.agentCatalog(),
          ]);
        setModes(modeList);
        setConfig(cfg);
        setProjects(projectList);
        setDashboard(dash);
        setAllAssets(assets);
        setAllFindings(findingList);
        setAllTasks(taskList);
        setSkills(skillList);
        setRoles(catalog.roles);
        setMainPrompts(catalog.modes);
        setMcpStatus(mcp);
        setHealth(h);
        setAllApprovals(approvalList);
        if (projectList[0]) setActiveProjectId(projectList[0].id);
        void loadRuns();
      } catch (error) {
        notify("error", messageOf(error));
      }
    })();
  }, [loadRuns, notify]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("lingeye-theme", theme);
  }, [theme]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
      }
      if (event.key === "Escape") {
        setPaletteOpen(false);
        setNotifyOpen(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    function onResize() {
      const isNarrow = window.innerWidth < 720;
      setNarrow(isNarrow);
      if (!isNarrow) setSidebar(true);
      const compact = window.innerWidth < 1080;
      setCompactInspector(compact);
      if (compact) setInspector(false);
    }
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(null), 4200);
    return () => window.clearTimeout(timer);
  }, [toast]);

  useEffect(() => {
    if (!activeId) return;
    setSelectedNode(null);
    setStreamingText("");
    setReasoningText("");
    void refresh(activeId);
  }, [activeId, refresh]);

  useEffect(() => {
    if (!activeId || tab !== "report") return;
    void api.report(activeId).then(setReport).catch((error) => notify("error", messageOf(error)));
  }, [activeId, tab, snapView?.snapshot.nodes.length, notify]);

  const schedule = useCallback(
    (id: string) => {
      window.clearTimeout(refreshTimer.current);
      refreshTimer.current = window.setTimeout(() => {
        void refresh(id);
        void loadRuns();
        if (activeProjectId) {
          void loadProject(activeProjectId).catch(() => undefined);
          void loadAllTasks().catch(() => undefined);
        }
      }, 450);
    },
    [refresh, loadRuns, loadProject, loadAllTasks, activeProjectId],
  );

  useEffect(() => {
    setStreamingText("");
    setReasoningText("");
  }, [activeId, view]);

  useEffect(() => {
    setTaskStreams({});
  }, [activeId]);

  useEffect(() => {
    if (view === "conversation" && inspector && inspectorSqueezes) {
      setSidebar(false);
    }
  }, [view, inspector, narrow, inspectorSqueezes]);

  useEffect(() => {
    if (!activeId) return;
    const source = new EventSource(`/api/runs/${activeId}/events/stream`);
    source.onmessage = (event) => {
      try {
        const frame = JSON.parse(event.data) as any;
        if (frame.type === "event" && frame.event) {
          const ev = frame.event;
          const taskId = typeof ev.taskId === "string" ? ev.taskId : "";
          if (ev.kind === "assistant_reasoning_delta") {
            if (!taskId) {
              setReasoningText((prev) => prev + (ev.detail ?? ""));
            }
          } else if (ev.kind === "assistant_delta") {
            if (taskId) {
              setTaskStreams((prev) => ({ ...prev, [taskId]: (prev[taskId] ?? "") + (ev.detail ?? "") }));
            } else {
              setStreamingText((prev) => prev + (ev.detail ?? ""));
            }
          } else if (ev.kind === "assistant") {
            if (taskId) {
              setTaskStreams((prev) => ({ ...prev, [taskId]: ev.detail ?? prev[taskId] ?? "" }));
            } else {
              setStreamingText("");
              setReasoningText("");
            }
          } else if (!taskId && (ev.kind === "tool_call" || ev.kind === "error" || ev.kind === "notice")) {
            setStreamingText("");
            setReasoningText("");
          }
          if (ev.kind !== "assistant_delta" && ev.kind !== "assistant_reasoning_delta") {
            setEvents((prev) => [
              ...prev.slice(-399),
              {
                id: `${Date.now()}-${Math.random().toString(16).slice(2)}`, runId: activeId, taskId,
                kind: ev.kind ?? "", tool: ev.tool ?? "", summary: ev.summary ?? "",
                detail: ev.detail ?? "", createdAt: new Date().toISOString(),
              },
            ]);
            if (taskId && ev.kind === "subagent" && String(ev.summary ?? "").startsWith("子代理结束")) {
              setTaskStreams((prev) => {
                const next = { ...prev };
                delete next[taskId];
                return next;
              });
            }
          }
        }
        if (frame.type === "approval" && frame.approval) {
          setApprovals((prev) => (prev.some((a) => a.id === frame.approval.id) ? prev : [...prev, frame.approval]));
        }
        if (frame.type === "approval.decided" && frame.approval) {
          setApprovals((prev) => prev.filter((a) => a.id !== frame.approval.id));
        }
        if (frame.type === "run.status" && frame.status) {
          setDetail((d) => (d ? { ...d, run: { ...d.run, status: frame.status } } : d));
        }
        schedule(activeId);
      } catch {
        /* ignore malformed frames */
      }
    };
    source.onerror = () => undefined;
    return () => source.close();
  }, [activeId, view, schedule]);

  useEffect(() => {
    if (!activeId || view !== "conversation") return;
    const active = detail?.running || detail?.run.status === "active";
    if (!active) return;
    const timer = window.setInterval(() => schedule(activeId), 3000);
    return () => window.clearInterval(timer);
  }, [activeId, view, detail?.running, detail?.run.status, schedule]);

  function toggleInspector() {
    const next = !inspector;
    if (next && inspectorSqueezes) setSidebar(false);
    setInspector(next);
  }

  function selectInspectorTab(next: InspectorTab) {
    if (inspectorSqueezes) setSidebar(false);
    setTab(next);
  }

  function openView(next: View) {
    setView(next);
    setNotifyOpen(false);
    if (next === "dashboard") {
      void loadDashboard();
      void loadProjects();
      void loadRuns();
      void loadAllApprovals();
    }
    if (next === "projects") void loadProjects();
    if (next === "conversations") void loadRuns();
    if (next === "assets") void loadAllAssets();
    if (next === "findings") void loadAllFindings();
    if (next === "tasks") void loadAllTasks();
    if (next === "approvals") void loadAllApprovals();
    if (next === "mcp" || next === "skills") void loadAux();
  }

  function openProject(id: string) {
    if (!id) {
      setView("projects");
      void loadProjects();
      return;
    }
    setActiveProjectId(id);
    setView("project");
    void loadProject(id).catch((error) => notify("error", messageOf(error)));
    void loadAllTasks();
  }

  function openConversation(id: string) {
    setActiveId(id);
    setView("conversation");
    void refresh(id);
  }

  function newConversation(projectId?: string, mode?: string) {
    setNewRunProjectId(projectId ?? activeProjectId ?? undefined);
    setNewRunMode(mode);
    setShowNew(true);
  }

  async function send(text: string) {
    if (!activeId) return;
    setStreamingText("");
    setReasoningText("");
    setBusy(true);
    try {
      await api.send(activeId, text);
      await refresh(activeId);
      await loadRuns();
      await loadDashboard();
    } catch (error) {
      notify("error", messageOf(error));
    } finally {
      setBusy(false);
    }
  }

  async function stop() {
    if (!activeId) return;
    const activeSubagents = tasks.filter((task) => task.status === "running" || task.status === "queued").length
      || allTasks.filter((task) => task.runId === activeId && (task.status === "running" || task.status === "queued")).length;
    if (activeSubagents > 0 && !window.confirm(`停止会同时取消 ${activeSubagents} 个正在运行/排队的子代理，确定停止？`)) {
      return;
    }
    try {
      await api.stop(activeId);
      await refresh(activeId);
      await loadRuns();
    } catch (error) {
      notify("error", messageOf(error));
    }
  }

  async function resume() {
    if (!activeId) return;
    try {
      await api.resume(activeId);
      await refresh(activeId);
    } catch (error) {
      notify("error", messageOf(error));
    }
  }

  async function decide(id: string, approve: boolean) {
    try {
      await api.decide(id, approve);
      setApprovals((prev) => prev.filter((a) => a.id !== id));
      setAllApprovals((prev) => prev.filter((a) => a.id !== id));
      await loadDashboard();
      if (activeId) await refresh(activeId);
    } catch (error) {
      notify("error", messageOf(error));
    }
  }

  async function setFindingStatus(id: string, state: string) {
    try {
      await api.setFindingStatus(id, state);
      setAllFindings((prev) => prev.map((finding) => (finding.id === id ? { ...finding, state } : finding)));
      setProjectDetail((prev) => (prev ? { ...prev, findings: prev.findings.map((finding) => (finding.id === id ? { ...finding, state } : finding)) } : prev));
      await loadDashboard();
      notify("info", "漏洞状态已更新");
    } catch (error) {
      notify("error", messageOf(error));
    }
  }

  async function cancelSubagent(id: string) {
    try {
      await api.cancelTask(id);
      await loadAllTasks();
      notify("info", "已取消子代理任务");
    } catch (error) {
      notify("error", messageOf(error));
    }
  }

  async function createRun(payload: Record<string, unknown>) {
    const created = await api.createRun(payload);
    setShowNew(false);
    setNewRunMode(undefined);
    setActiveId(created.run.id);
    setView("conversation");
    await Promise.all([loadRuns(), loadProjects(), loadDashboard(), loadAllApprovals()]);
    if (created.run.projectId) {
      setActiveProjectId(created.run.projectId);
      void loadProject(created.run.projectId);
    }
  }

  async function createProject(payload: Record<string, unknown>) {
    const project = await api.createProject(payload);
    setShowNewProject(false);
    await Promise.all([loadProjects(), loadDashboard()]);
    openProject(project.id);
  }

  async function updateProject(body: Record<string, unknown>) {
    if (!projectDetail) return;
    await api.updateProject(projectDetail.project.id, body);
    await Promise.all([loadProject(projectDetail.project.id), loadProjects(), loadDashboard()]);
    setShowEditProject(false);
    notify("info", "项目信息已更新");
  }

  async function deleteProject(id: string) {
    const target = projects.find((project) => project.id === id);
    await api.deleteProject(id);
    await Promise.all([
      loadProjects(), loadRuns(), loadDashboard(), loadAllApprovals(),
      loadAllTasks(), loadAllAssets(), loadAllFindings(),
    ]);
    if (activeProjectId === id) {
      setActiveProjectId(null);
      setProjectDetail(null);
      setView("projects");
    }
    if (detail?.run.projectId === id) {
      setActiveId(null);
      setDetail(null);
      setView("projects");
    }
    notify("info", `项目「${target?.name ?? id}」已删除`);
  }

  async function saveConfig(next: AppConfig) {
    const saved = await api.saveConfig(next);
    setConfig(saved);
    setShowSettings(false);
    await loadAux();
    notify("info", "设置已保存");
  }

  async function reconnectMCP() {
    try {
      await api.reconnectMCP();
      await loadAux();
      notify("info", "MCP 已重连");
    } catch (err) {
      notify("error", err instanceof Error ? err.message : String(err));
    }
  }

  async function saveSkill(input: { name: string; description: string; body: string }) {
    const saved = await api.saveSkill(input);
    await loadAux();
    notify("info", "技能已保存");
    return saved;
  }

  const activeProvider =
    config?.providers.find((p) => p.id === config.activeProvider) ?? config?.providers[0] ?? null;
  const runProvider = config?.providers.find((p) => p.id === detail?.run.providerId) ?? activeProvider;
  const providerLabel = detail?.run.model || runProvider?.model || "未配置模型";
  const projectName =
    projects.find((p) => p.id === detail?.run.projectId)?.name ?? "默认项目";

  if (auth?.enabled && !auth.authenticated) {
    return (
      <LoginPage
        onLogin={async (password) => {
          await api.login(password);
          window.location.reload();
        }}
      />
    );
  }

  return (
    <div className={`app ${sidebar ? "" : "sidebar-collapsed"} ${view === "conversation" && inspector ? "inspector-open" : "inspector-collapsed"}`}>
      {narrow && sidebar ? <button className="sidebar-backdrop" onClick={() => setSidebar(false)} aria-label="关闭侧栏" /> : null}
      {compactInspector && inspector && detail && view === "conversation" ? (
        <button className="inspector-backdrop" onClick={() => setInspector(false)} aria-label="关闭检查器" />
      ) : null}

      {sidebar ? (
        <ShellSidebar
          view={view}
          projects={projects}
          activeProjectId={activeProjectId}
          configured={Boolean(activeProvider?.model && activeProvider?.baseUrl)}
          version={health?.version}
          approvalCount={allApprovals.length}
          onNavigate={openView}
          onOpenProject={openProject}
          onNewProject={() => setShowNewProject(true)}
          onNewConversation={() => newConversation()}
          onSettings={() => setShowSettings(true)}
          onCollapse={() => setSidebar(false)}
        />
      ) : (
        <button className="icon-btn sidebar-open" onClick={() => setSidebar(true)} title="展开侧栏">
          <PanelLeftOpen size={17} />
        </button>
      )}

      <section className="main">
        <header className="globalbar">
          <div className="globalbar-left">
            <span className="global-crumb">lingeyebot</span>
            <span className="global-sep">/</span>
            <span className="global-page">{viewLabels[view]}</span>
            {view === "project" && projectDetail ? (
              <>
                <span className="global-sep">/</span>
                <span className="global-page">{projectDetail.project.name}</span>
              </>
            ) : null}
          </div>
          <button className="global-search" onClick={() => setPaletteOpen(true)}>
            <Search size={15} /><span>搜索项目、任务、资产、漏洞…</span><kbd>⌘K</kbd>
          </button>
          <div className="globalbar-right">
            <button className="icon-btn" onClick={() => setTheme(theme === "light" ? "dark" : "light")} title="切换主题">
              {theme === "light" ? <Moon size={17} /> : <Sun size={17} />}
            </button>
            <div className="notify-wrap">
              <button className="icon-btn notify-btn" onClick={() => setNotifyOpen((open) => !open)} title="待审批">
                <Bell size={17} />
                {allApprovals.length > 0 ? <span className="notify-badge">{allApprovals.length}</span> : null}
              </button>
              {notifyOpen ? (
                <div className="notify-pop">
                  <header>
                    <strong>待审批</strong>
                    <button className="link-btn" onClick={() => { setNotifyOpen(false); openView("approvals"); }}>查看全部</button>
                  </header>
                  {allApprovals.slice(0, 5).map((approval) => (
                    <div key={approval.id} className="notify-item">
                      <div>
                        <strong>{approval.tool}</strong>
                        <p>{approval.reason || "策略要求人工确认"}</p>
                        <code>{approval.input.slice(0, 120)}</code>
                      </div>
                      <button className="link-btn" onClick={() => { setNotifyOpen(false); openView("approvals"); }}>处理</button>
                    </div>
                  ))}
                  {allApprovals.length === 0 ? <p className="notify-empty">没有待审批操作。</p> : null}
                </div>
              ) : null}
            </div>
            <button className="icon-btn" onClick={() => setShowSettings(true)} title="设置"><Settings2 size={17} /></button>
            <div className="avatar">{(auth?.username ?? "L").slice(0, 1).toUpperCase()}</div>
          </div>
        </header>

        {view === "conversation" && detail ? (
          <>
            <header className="topbar">
              <div className="topbar-left">
                <button className="icon-btn" onClick={() => openProject(detail.run.projectId)} title="返回项目">
                  <ArrowLeft size={17} />
                </button>
                <div className="run-heading">
                  <div className="crumbs">
                    <span className={`mode-chip mode-${detail.run.mode}`}>{projectName}</span>
                  </div>
                  <h2>{detail.run.title || detail.run.objective}</h2>
                  <p>{detail.run.target} · {detail.run.objective}</p>
                </div>
              </div>
              <div className="topbar-right">
                <span className={`badge badge-${statusTone(detail.run.status)}`}>
                  {statusLabels[detail.run.status] ?? detail.run.status}
                </span>
                {detail.running ? <Spinner label="运行中" /> : null}
                <span className="provider-pill" title={providerLabel}>{providerLabel}</span>
                <button className="icon-btn" onClick={toggleInspector} title="检查器">
                  {inspector ? <PanelRightClose size={17} /> : <PanelRightOpen size={17} />}
                </button>
              </div>
            </header>
            <Chat
              detail={detail}
              mode={modes.find((m) => m.id === detail.run.mode)}
              messages={messages}
              approvals={approvals}
              events={events.filter((event) => !event.taskId)}
              streaming={streamingText}
              reasoning={reasoningText}
              busy={busy}
              modelConfigured={Boolean(runProvider?.baseUrl && runProvider?.model)}
              onOpenSettings={() => setShowSettings(true)}
              onSend={send}
              onStop={stop}
              onResume={resume}
              onDecide={decide}
            />
          </>
        ) : view === "conversation" ? (
          <div className="page-scroll">
            <Empty title="正在加载任务…" description="如果一直空白，返回项目重新打开这个任务。" />
          </div>
        ) : (
          <div className="page-scroll">
            {view === "dashboard" ? (
              <DashboardPage
                dashboard={dashboard}
                projects={projects}
                approvals={allApprovals}
                onOpenProject={openProject}
                onOpenConversation={openConversation}
                onOpenFindings={() => openView("findings")}
                onOpenApprovals={() => openView("approvals")}
                onNewProject={() => setShowNewProject(true)}
              />
            ) : null}
            {view === "conversations" ? (
              <ConversationsPage runs={runs} projects={projects} onOpen={openConversation} onNew={() => newConversation()} />
            ) : null}
            {view === "projects" ? (
              <ProjectsPage projects={projects} onOpen={openProject} onNew={() => setShowNewProject(true)} onDelete={deleteProject} />
            ) : null}
            {view === "project" ? (
              projectDetail ? (
                <ProjectPage
                  detail={projectDetail}
                  running={projectRunning}
                  tasks={allTasks}
                  modes={modes}
                  onOpenConversation={openConversation}
                  onNewConversation={(mode) => newConversation(projectDetail.project.id, mode)}
                  onEditProject={() => setShowEditProject(true)}
                  onStatus={setFindingStatus}
                />
              ) : (
                <Empty title="正在加载项目…" />
              )
            ) : null}
            {view === "assets" ? <AssetsPage assets={allAssets} /> : null}
            {view === "findings" ? (
              <FindingsPage findings={allFindings} onOpenConversation={openConversation} onStatus={setFindingStatus} />
            ) : null}
            {view === "tasks" ? (
              <TasksPage tasks={allTasks} runs={runs} projects={projects} roles={roles} mainPrompts={mainPrompts} taskStreams={taskStreams} onOpenConversation={openConversation} onCancel={cancelSubagent} />
            ) : null}
            {view === "approvals" ? (
              <ApprovalsPage approvals={allApprovals} runs={runs} projects={projects} onDecide={decide} onOpenConversation={openConversation} />
            ) : null}
            {view === "mcp" ? <MCPPage config={config} status={mcpStatus} onSave={saveConfig} onReconnect={reconnectMCP} /> : null}
            {view === "skills" ? <SkillsPage skills={skills} onSave={saveSkill} /> : null}
            {view === "agents" ? <AgentsPage modes={modes} onNewConversation={(mode) => newConversation(undefined, mode)} /> : null}
          </div>
        )}
      </section>

      {view === "conversation" && inspector && detail ? (
        <Inspector
          detail={detail}
          view={snapView}
          findings={findings}
          coverage={coverage}
          evidence={evidence}
          report={report}
          tasks={tasks}
          audit={audit}
          tab={tab}
          onTab={selectInspectorTab}
          selectedNode={selectedNode}
          onSelectNode={setSelectedNode}
          onClose={() => setInspector(false)}
          onOpenConversation={openConversation}
          onTaskCancel={cancelSubagent}
          taskStreams={taskStreams}
        />
      ) : null}

      {showNew ? (
        <NewRunDialog
          modes={modes}
          config={config}
          projects={projects}
          projectId={newRunProjectId}
          initialMode={newRunMode}
          onClose={() => { setShowNew(false); setNewRunMode(undefined); }}
          onCreate={createRun}
        />
      ) : null}

      {showNewProject ? (
        <NewProjectDialog onClose={() => setShowNewProject(false)} onCreate={createProject} />
      ) : null}

      {showEditProject && projectDetail ? (
        <EditProjectDialog
          project={projectDetail.project}
          onClose={() => setShowEditProject(false)}
          onSave={updateProject}
        />
      ) : null}

      {showSettings && config ? (
        <SettingsDialog
          config={config}
          mcpStatus={mcpStatus}
          skills={skills}
          auth={auth}
          onSetPassword={async (username, current, next) => {
            await api.setPassword({ username, currentPassword: current, newPassword: next });
            window.location.reload();
          }}
          onLogout={async () => {
            await api.logout();
            window.location.reload();
          }}
          onClose={() => setShowSettings(false)}
          onSave={saveConfig}
        />
      ) : null}

      {paletteOpen ? (
        <CommandPalette
          projects={projects}
          runs={runs}
          assets={allAssets}
          findings={allFindings}
          onClose={() => setPaletteOpen(false)}
          onChoose={(kind, id) => {
            setPaletteOpen(false);
            if (kind === "project" && id) openProject(id);
            if (kind === "conversation" && id) openConversation(id);
            if (kind === "view" && id) openView(id as View);
            if (kind === "asset") openView("assets");
            if (kind === "finding") openView("findings");
          }}
        />
      ) : null}

      {toast ? <div className={`toast ${toast.tone}`}>{toast.text}</div> : null}
    </div>
  );
}

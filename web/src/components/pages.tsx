import {
  Activity, AlertTriangle, ArrowRight, Bot, Boxes, Bug, Check, ChevronRight, Clock, Code2,
  Download, Eye, FileSearch, Flag, FolderKanban, HeartHandshake, LayoutDashboard, ListChecks,
  MessagesSquare, PanelLeftClose, Pencil, Plug, Play, Plus, RefreshCw, Search, Settings,
  ShieldAlert, ShieldCheck, Sparkles, Square, Swords, Terminal, Trash2, Users, X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import Graph from "./Graph";
import type {
  Asset, AppConfig, Approval, Dashboard, FindingPayload, KnowledgeDoc, KnowledgeMeta, MainPromptMeta,
  MCPServerConfig, MCPStatus, ModeMeta, Project, ProjectDetail, ProjectFinding, ProjectNode, ProjectSummary, RoleMeta,
  Run, RunSummary, SkillInfo, Snapshot, Task, TaskDetail,
} from "../types";
import { dateTime, eventKindLabel, modeLabels, severityTone, shortTime, statusLabels, statusTone, truncate } from "../lib/format";
import { api } from "../lib/api";
import { Badge, Markdown, Modal } from "./ui";

export type View =
  | "dashboard" | "conversations" | "projects" | "project" | "conversation"
  | "assets" | "findings" | "tasks" | "approvals" | "mcp" | "skills" | "agents";

export const viewLabels: Record<View, string> = {
  dashboard: "仪表盘", conversations: "任务", projects: "项目管理", project: "项目",
  conversation: "任务详情", assets: "资产管理", findings: "漏洞管理", tasks: "子代理",
  approvals: "人机协同", mcp: "MCP", skills: "技能", agents: "智能体",
};

const severityOrder = ["critical", "high", "medium", "low", "info"];
const severityLabels: Record<string, string> = {
  critical: "严重", high: "高危", medium: "中危", low: "低危", info: "信息",
};
const findingStatuses = [
  { value: "open", label: "待处理" },
  { value: "confirmed", label: "已确认" },
  { value: "fixed", label: "已修复" },
  { value: "ignored", label: "已忽略" },
  { value: "false_positive", label: "误报" },
];

function modeIcon(mode: string) {
  switch (mode) {
    case "daily": return HeartHandshake;
    case "bugbounty": return Bug;
    case "audit": return FileSearch;
    case "redteam": return Swords;
    case "ctf": return Flag;
    default: return Terminal;
  }
}

function severityCount(severity: Record<string, number>, keys: string[]) {
  return keys.reduce((sum, key) => sum + (severity[key] ?? 0), 0);
}

function percent(part: number, total: number) {
  if (total <= 0) return 0;
  return Math.round((part / total) * 100);
}

function csvCell(value: string | number) {
  let text = String(value ?? "");
  // Spreadsheet formula injection: prefix risky leading characters.
  if (/^[=+\-@\t\r]/.test(text)) text = "'" + text;
  return `"${text.replace(/"/g, '""')}"`;
}

function downloadCSV(filename: string, rows: (string | number)[][]) {
  const body = rows.map((row) => row.map(csvCell).join(",")).join("\r\n");
  const url = URL.createObjectURL(new Blob(["\ufeff" + body], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

interface SidebarProps {
  view: View;
  projects: ProjectSummary[];
  activeProjectId: string | null;
  configured: boolean;
  version?: string;
  approvalCount: number;
  onNavigate: (view: View) => void;
  onOpenProject: (id: string) => void;
  onNewProject: () => void;
  onNewConversation: () => void;
  onSettings: () => void;
  onCollapse: () => void;
}

const groups: { label: string; items: { view: View; label: string; icon: typeof LayoutDashboard }[] }[] = [
  {
    label: "工作台",
    items: [
      { view: "dashboard", label: "仪表盘", icon: LayoutDashboard },
      { view: "conversations", label: "任务", icon: MessagesSquare },
    ],
  },
  {
    label: "安全作业",
    items: [
      { view: "projects", label: "项目管理", icon: FolderKanban },
      { view: "assets", label: "资产管理", icon: Boxes },
      { view: "findings", label: "漏洞管理", icon: Bug },
      { view: "tasks", label: "子代理", icon: ListChecks },
      { view: "approvals", label: "人机协同", icon: ShieldAlert },
    ],
  },
  {
    label: "能力中心",
    items: [
      { view: "mcp", label: "MCP", icon: Plug },
      { view: "skills", label: "技能", icon: Sparkles },
      { view: "agents", label: "智能体", icon: Bot },
    ],
  },
];

export function ShellSidebar({
  view, projects, activeProjectId, configured, version, approvalCount,
  onNavigate, onOpenProject, onNewProject, onNewConversation, onSettings, onCollapse,
}: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark"><Eye size={18} /></div>
        <div className="brand-text"><h1>lingeyebot</h1><span>security agent</span></div>
        <button className="icon-btn" onClick={onCollapse} title="收起侧栏"><PanelLeftClose size={16} /></button>
      </div>

      <div className="project-switcher">
        <select
          value={activeProjectId ?? ""}
          onChange={(e) => (e.target.value ? onOpenProject(e.target.value) : onNavigate("projects"))}
        >
          <option value="">选择项目…</option>
          {projects.map((project) => (
            <option key={project.id} value={project.id}>{project.name}</option>
          ))}
        </select>
        <button className="icon-btn" onClick={onNewProject} title="新建项目"><Plus size={16} /></button>
      </div>

      <button className="primary new-run" onClick={onNewConversation}><Plus size={16} /> 新建任务</button>

      <nav className="nav-groups">
        {groups.map((group) => (
          <div key={group.label} className="nav-group">
            <span className="nav-label">{group.label}</span>
            {group.items.map((item) => {
              const Icon = item.icon;
              const active = view === item.view || (item.view === "projects" && view === "project");
              return (
                <button
                  key={item.view}
                  className={`nav-item ${active ? "active" : ""}`}
                  onClick={() => onNavigate(item.view)}
                >
                  <Icon size={16} /><span>{item.label}</span>
                  {item.view === "approvals" && approvalCount > 0 ? <em className="nav-count">{approvalCount}</em> : null}
                </button>
              );
            })}
          </div>
        ))}
        <div className="nav-group">
          <span className="nav-label">平台</span>
          <button className="nav-item" onClick={onSettings}><Settings size={16} /><span>系统设置</span></button>
        </div>
      </nav>

      <div className="sidebar-foot">
        <div className={`provider-state ${configured ? "ok" : "warn"}`}>
          <span className="dot" />{configured ? "LLM 已配置" : "未配置 LLM"}
        </div>
        {version ? <span className="version">v{version}</span> : null}
      </div>
    </aside>
  );
}

function Page({ title, subtitle, actions, children }: {
  title: string; subtitle?: string; actions?: ReactNode; children: ReactNode;
}) {
  return (
    <div className="page">
      <header className="page-head">
        <div><h1>{title}</h1>{subtitle ? <p>{subtitle}</p> : null}</div>
        {actions ? <div className="page-actions">{actions}</div> : null}
      </header>
      <div className="page-body">{children}</div>
    </div>
  );
}

function Kpi({ label, value, hint, tone, icon }: {
  label: string; value: ReactNode; hint: string; tone?: string; icon: ReactNode;
}) {
  return (
    <div className={`kpi-card ${tone ?? ""}`}>
      <div className="kpi-top"><span>{label}</span><i>{icon}</i></div>
      <strong>{value}</strong>
      <small>{hint}</small>
    </div>
  );
}

function SeverityDonut({ severity, total }: { severity: Record<string, number>; total: number }) {
  const radius = 54;
  const circumference = 2 * Math.PI * radius;
  let offset = 0;
  return (
    <div className="donut-wrap">
      <svg viewBox="0 0 140 140" className="donut" role="img" aria-label="漏洞严重程度分布">
        <circle cx="70" cy="70" r={radius} className="donut-track" />
        {severityOrder.map((key) => {
          const count = severity[key] ?? 0;
          const length = total > 0 ? (count / total) * circumference : 0;
          const dash = `${Math.max(0, length - 2)} ${circumference}`;
          const element = (
            <circle
              key={key}
              cx="70" cy="70" r={radius}
              className={`donut-segment sev-${key}`}
              strokeDasharray={dash}
              strokeDashoffset={-offset}
            />
          );
          offset += length;
          return element;
        })}
        <text x="70" y="66" textAnchor="middle" className="donut-total">{total}</text>
        <text x="70" y="84" textAnchor="middle" className="donut-label">漏洞</text>
      </svg>
      <div className="donut-legend">
        {severityOrder.map((key) => (
          <div key={key} className="donut-legend-row">
            <i className={`dot sev-${key}`} />
            <span>{severityLabels[key]}</span>
            <b>{severity[key] ?? 0}</b>
            <em>{percent(severity[key] ?? 0, total)}%</em>
          </div>
        ))}
      </div>
    </div>
  );
}

export function DashboardPage({
  dashboard, projects, approvals, onOpenProject, onOpenConversation, onOpenFindings,
  onOpenApprovals, onNewProject,
}: {
  dashboard: Dashboard | null; projects: ProjectSummary[]; approvals: Approval[];
  onOpenProject: (id: string) => void; onOpenConversation: (id: string) => void;
  onOpenFindings: () => void; onOpenApprovals: () => void; onNewProject: () => void;
}) {
  const [recentTab, setRecentTab] = useState<"findings" | "facts">("findings");
  const severity = dashboard?.severity ?? {};
  const statusCounts = dashboard?.statusCounts ?? {};
  const total = dashboard?.findings ?? 0;
  const openTotal = severityOrder.reduce((sum, key) => sum + (severity[key] ?? 0), 0);
  const critical = severity.critical ?? 0;
  const high = severity.high ?? 0;
  const fixed = statusCounts.fixed ?? 0;
  const fixRate = percent(fixed, total);
  const pending = dashboard?.pendingApprovals ?? approvals.length;
  const topTool = dashboard?.toolCounts?.[0];
  const maxTool = Math.max(1, ...(dashboard?.toolCounts ?? []).map((item) => item.count));

  return (
    <Page
      title="仪表盘"
      subtitle="项目、模式任务、漏洞与审批的运行态势"
      actions={<button className="primary" onClick={onNewProject}><Plus size={15} /> 新建项目</button>}
    >
      {(pending > 0 || critical + high > 0) ? (
        <div className="alert-strip">
          <div className="alert-icon"><AlertTriangle size={18} /></div>
          <div className="alert-copy">
            <strong>需要处理</strong>
            <span>
              {critical + high > 0 ? `${critical + high} 个未关闭严重/高危漏洞` : ""}
              {critical + high > 0 && pending > 0 ? " · " : ""}
              {pending > 0 ? `${pending} 个工具调用等待审批` : ""}
            </span>
          </div>
          <div className="alert-actions">
            {critical + high > 0 ? <button className="ghost-btn danger" onClick={onOpenFindings}>查看漏洞</button> : null}
            {pending > 0 ? <button className="ghost-btn" onClick={onOpenApprovals}>处理审批</button> : null}
          </div>
        </div>
      ) : null}

      <div className="kpi-grid">
        <Kpi label="进行中任务" value={dashboard?.running ?? 0} hint={`共 ${dashboard?.conversations ?? 0} 个模式任务`} icon={<Activity size={16} />} tone="blue" />
        <Kpi label="待处理漏洞" value={critical + high} hint={`共 ${total} 个漏洞 · 风险分 ${dashboard?.riskScore ?? 0}`} icon={<Bug size={16} />} tone="red" />
        <Kpi label="待审批" value={pending} hint="危险工具调用" icon={<ShieldAlert size={16} />} tone="amber" />
        <Kpi label="工具调用" value={dashboard?.toolCalls ?? 0} hint={topTool ? `最常用 ${topTool.tool}` : "暂无记录"} icon={<Terminal size={16} />} tone="violet" />
        <Kpi label="资产" value={dashboard?.assets ?? 0} hint={`${dashboard?.projects ?? 0} 个项目`} icon={<Boxes size={16} />} tone="green" />
      </div>

      <div className="dash-main">
        <section className="panel severity-panel">
          <header className="panel-head">
            <div><h3>漏洞严重程度</h3><p>按未关闭漏洞统计</p></div>
            <span className="panel-badge">{openTotal} 个未关闭</span>
          </header>
          <SeverityDonut severity={severity} total={openTotal} />
          <div className="fix-rate">
            <div className="fix-rate-head"><span>修复率</span><b>{fixRate}%</b></div>
            <div className="progress"><i style={{ width: `${fixRate}%` }} /></div>
            <div className="status-chips">
              {findingStatuses.map((status) => (
                <span key={status.value} className={`status-chip ${status.value}`}>
                  {status.label}<b>{statusCounts[status.value] ?? 0}</b>
                </span>
              ))}
            </div>
          </div>
        </section>

        <section className="panel">
          <header className="panel-head"><div><h3>工具执行排行</h3><p>按调用次数</p></div><span className="panel-badge">Top 8</span></header>
          <div className="tool-bars">
            {(dashboard?.toolCounts ?? []).length === 0 ? <p className="muted">还没有工具调用。</p> : null}
            {(dashboard?.toolCounts ?? []).map((item, index) => (
              <div key={`${item.tool}-${index}`} className="tool-bar-row">
                <span className="tool-rank">{String(index + 1).padStart(2, "0")}</span>
                <span className="tool-name" title={item.tool}>{item.tool}</span>
                <div className="tool-bar"><i style={{ width: `${(item.count / maxTool) * 100}%` }} /></div>
                <b>{item.count}</b>
              </div>
            ))}
          </div>
          <div className="cap-strip">
            <div><Plug size={15} /><span>MCP</span><b>{dashboard?.capabilities?.mcpOnline ?? 0}/{dashboard?.capabilities?.mcpTools ?? 0}</b></div>
            <div><Sparkles size={15} /><span>技能</span><b>{dashboard?.capabilities?.skills ?? 0}</b></div>
            <div><Bot size={15} /><span>角色</span><b>{dashboard?.capabilities?.agents ?? 0}</b></div>
            <div><LayersIcon /><span>模式</span><b>{dashboard?.capabilities?.modes ?? 0}</b></div>
          </div>
        </section>
      </div>

      <div className="dash-main">
        <section className="panel">
          <header className="panel-head">
            <div className="tabs-mini">
              <button className={recentTab === "findings" ? "active" : ""} onClick={() => setRecentTab("findings")}>最近漏洞</button>
              <button className={recentTab === "facts" ? "active" : ""} onClick={() => setRecentTab("facts")}>最近事实</button>
            </div>
            <button className="link-btn" onClick={onOpenFindings}>查看全部 <ArrowRight size={13} /></button>
          </header>
          <div className="mini-list">
            {recentTab === "findings" ? (
              (dashboard?.recentFindings ?? []).map((finding) => (
                <button key={finding.id} className="mini-row" onClick={() => onOpenConversation(finding.conversationId)}>
                  <Badge tone={severityTone[finding.payload?.severity ?? "info"] ?? "info"}>
                    {severityLabels[finding.payload?.severity ?? "info"] ?? finding.payload?.severity}
                  </Badge>
                  <span className="mini-main"><strong>{finding.payload?.title ?? "漏洞"}</strong><small>{finding.projectName} · {finding.conversationTitle}</small></span>
                  <span className="mini-meta">{shortTime(finding.createdAt)}</span>
                </button>
              ))
            ) : (
              (dashboard?.recentFacts ?? []).map((fact) => (
                <button key={fact.id} className="mini-row" onClick={() => onOpenConversation(fact.conversationId)}>
                  <span className="fact-basis">{fact.payload?.basis ?? "fact"}</span>
                  <span className="mini-main"><strong>{fact.payload?.text ?? fact.payload?.title ?? "事实"}</strong><small>{fact.projectName} · {fact.conversationTitle}</small></span>
                  <span className="mini-meta">{shortTime(fact.createdAt)}</span>
                </button>
              ))
            )}
            {(recentTab === "findings" ? dashboard?.recentFindings : dashboard?.recentFacts)?.length === 0 ? (
              <p className="muted">{recentTab === "findings" ? "还没有确认漏洞。" : "还没有事实记录。"}</p>
            ) : null}
          </div>
        </section>

        <section className="panel">
          <header className="panel-head"><div><h3>最近活动</h3><p>工具 / 代理事件</p></div><span className="panel-badge">实时</span></header>
          <div className="event-list">
            {(dashboard?.recentEvents ?? []).map((event) => (
              <div key={event.id} className="event-item">
                <span className={`event-dot ${event.kind}`} />
                <div><strong>{event.tool || eventKindLabel(event.kind)}</strong><p>{truncate(event.summary, 100)}</p></div>
                <span className="mini-meta">{shortTime(event.createdAt)}</span>
              </div>
            ))}
            {!dashboard?.recentEvents?.length ? <p className="muted">还没有活动。</p> : null}
          </div>
        </section>
      </div>

      <section className="panel">
        <header className="panel-head"><div><h3>项目</h3><p>项目是授权、资产与发现的容器</p></div><button className="link-btn" onClick={() => onOpenProject("")}>查看全部 <ArrowRight size={13} /></button></header>
        <div className="project-strip">
          {projects.slice(0, 6).map((project) => (
            <button key={project.id} className="project-mini" onClick={() => onOpenProject(project.id)}>
              <div className="project-mini-head"><FolderKanban size={16} /><strong>{project.name}</strong><ChevronRight size={14} /></div>
              <p>{project.description || project.scope || "未声明范围"}</p>
              <div className="project-mini-stats">
                <span>{project.conversations} 任务</span><span>{project.assets} 资产</span><span>{project.findings} 漏洞</span>
              </div>
            </button>
          ))}
          {projects.length === 0 ? <p className="muted">还没有项目，先创建一个。</p> : null}
        </div>
      </section>
    </Page>
  );
}

function LayersIcon() {
  return <Boxes size={15} />;
}

export function ConversationsPage({
  runs, projects, onOpen, onNew,
}: { runs: RunSummary[]; projects: ProjectSummary[]; onOpen: (id: string) => void; onNew: () => void }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [project, setProject] = useState("");
  const projectName = (id: string) => projects.find((p) => p.id === id)?.name ?? "默认项目";
  const filtered = runs.filter((run) => {
    const haystack = `${run.title} ${run.objective} ${run.target} ${projectName(run.projectId)} ${run.mode}`.toLowerCase();
    return (!query || haystack.includes(query.toLowerCase()))
      && (!status || run.status === status)
      && (!project || run.projectId === project);
  });
  return (
    <Page title="任务" subtitle="每个模式任务属于一个项目；项目内先选模式，再开始任务"
      actions={<button className="primary" onClick={onNew}><Plus size={15} /> 新建任务</button>}>
      <div className="filter-bar">
        <label className="search-box"><Search size={15} /><input placeholder="搜索任务、目标、项目…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <select value={project} onChange={(e) => setProject(e.target.value)}><option value="">全部项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">全部状态</option>
          {Array.from(new Set(runs.map((run) => run.status))).map((value) => <option key={value} value={value}>{statusLabels[value] ?? value}</option>)}
        </select>
        <span className="filter-count">{filtered.length} / {runs.length}</span>
      </div>
      <div className="card-list">
        {filtered.map((run) => {
          const Icon = modeIcon(run.mode);
          return (
            <button key={run.id} className="task-row" onClick={() => onOpen(run.id)}>
              <span className={`mode-avatar mode-${run.mode}`}><Icon size={16} /></span>
              <span className="conv-main">
                <strong>{run.title || run.objective}</strong>
                <small>{projectName(run.projectId)} · {run.target} · {modeLabels[run.mode] ?? run.mode}</small>
              </span>
              <span className="conv-stats">{run.findings} 漏洞 · {run.openGoals} 目标</span>
              <Badge tone={statusTone(run.status)}>{statusLabels[run.status] ?? run.status}</Badge>
              <ChevronRight size={14} />
            </button>
          );
        })}
        {filtered.length === 0 ? <EmptyHint title={runs.length === 0 ? "还没有任务" : "没有匹配的任务"} detail={runs.length === 0 ? "进入项目，选择一种模式开始。" : "调整搜索或筛选条件。"} /> : null}
      </div>
      {runs.length >= 100 ? <p className="muted small">仅显示最近 100 个任务。</p> : null}
    </Page>
  );
}

export function ProjectsPage({
  projects, onOpen, onNew, onDelete,
}: {
  projects: ProjectSummary[]; onOpen: (id: string) => void; onNew: () => void;
  onDelete: (id: string) => Promise<void>;
}) {
  const [query, setQuery] = useState("");
  const [deleting, setDeleting] = useState<ProjectSummary | null>(null);
  const columns = "minmax(0,1.6fr) 170px 70px 70px 70px 90px 120px 100px";
  const filtered = projects.filter((project) => {
    const haystack = `${project.name} ${project.description} ${project.scope} ${project.authorization}`.toLowerCase();
    return !query || haystack.includes(query.toLowerCase());
  });
  return (
    <Page title="项目管理" subtitle="项目承载授权范围、资产、发现与模式任务；进入项目后再选模式开任务"
      actions={<button className="primary" onClick={onNew}><Plus size={15} /> 新建项目</button>}>
      <div className="filter-bar">
        <label className="search-box"><Search size={15} /><input placeholder="搜索项目、范围、说明…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <span className="filter-count">{filtered.length} / {projects.length}</span>
      </div>
      <div className="data-table">
        <div className="data-head" style={{ gridTemplateColumns: columns }}>
          <span>项目</span><span>范围</span><span>任务</span><span>资产</span><span>漏洞</span><span>状态</span><span>最近活动</span><span>操作</span>
        </div>
        {filtered.map((project) => (
          <div key={project.id} className="data-row project-row" role="button" tabIndex={0} style={{ gridTemplateColumns: columns }}
            onClick={() => onOpen(project.id)}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onOpen(project.id); } }}>
            <span className="project-name-cell">
              <strong>{project.name}</strong>
              <small>{project.description || "还没有项目说明"}</small>
            </span>
            <span className="mono muted project-scope-cell">{project.scope || "未声明范围"}</span>
            <span>{project.conversations}</span>
            <span>{project.assets}</span>
            <span>{project.findings}</span>
            <Badge tone={statusTone(project.status)}>{project.status}</Badge>
            <span className="muted">{shortTime(project.lastActivity || project.updatedAt)}</span>
            <span className="row-actions">
              {project.nkey === "default" ? <span className="muted small">默认</span> : (
                <button className="link-btn danger" onClick={(e) => { e.stopPropagation(); setDeleting(project); }}><Trash2 size={12} /> 删除</button>
              )}
            </span>
          </div>
        ))}
        {filtered.length === 0 ? <p className="muted table-empty">{projects.length === 0 ? "还没有项目，先创建一个。" : "没有匹配的项目。"}</p> : null}
      </div>
      {deleting ? (
        <DeleteProjectDialog project={deleting} onClose={() => setDeleting(null)} onDelete={async () => {
          await onDelete(deleting.id);
          setDeleting(null);
        }} />
      ) : null}
    </Page>
  );
}

function DeleteProjectDialog({ project, onClose, onDelete }: {
  project: ProjectSummary; onClose: () => void; onDelete: () => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function remove() {
    if (name !== project.name) { setError("项目名不匹配"); return; }
    setBusy(true);
    setError("");
    try {
      await onDelete();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }
  return (
    <Modal title="删除项目" subtitle={project.name} onClose={onClose}>
      <p className="danger-note">此操作不可恢复。项目下的任务、消息、节点、证据、审批、子代理、共享黑板和项目记忆都会被删除；资产本身会保留。</p>
      <label className="field">
        <span>输入项目名 <strong>{project.name}</strong> 确认</span>
        <input value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder={project.name} />
      </label>
      {error ? <p className="error-text">{error}</p> : null}
      <footer className="modal-actions">
        <button className="ghost-btn" onClick={onClose}>取消</button>
        <button className="danger-btn" disabled={busy || name !== project.name} onClick={() => void remove()}>{busy ? "删除中…" : "永久删除"}</button>
      </footer>
    </Modal>
  );
}

const projectTabs = [
  { id: "tasks", label: "任务" },
  { id: "assets", label: "资产" },
  { id: "findings", label: "漏洞" },
  { id: "graph", label: "攻击链" },
  { id: "board", label: "共享黑板" },
] as const;

type ProjectTab = typeof projectTabs[number]["id"];

export function ProjectPage({
  detail, running, tasks, modes, onOpenConversation, onNewConversation, onEditProject, onStatus,
}: {
  detail: ProjectDetail; running: Record<string, boolean>; tasks: Task[]; modes: ModeMeta[];
  onOpenConversation: (id: string) => void; onNewConversation: (mode?: string) => void;
  onEditProject: () => void;
  onStatus: (id: string, state: string) => Promise<void>;
}) {
  const [tab, setTab] = useState<ProjectTab>("tasks");
  const project = detail.project;
  const critical = detail.findings.filter((finding) => finding.payload.severity === "critical").length;
  const high = detail.findings.filter((finding) => finding.payload.severity === "high").length;
  const coverage = percent(detail.coverage.tested, detail.coverage.total);
  const modeGroups = modes.map((mode) => ({
    mode,
    runs: detail.conversations.filter((run) => run.mode === mode.id),
  }));

  return (
    <Page title={project.name} subtitle={project.description || project.scope}
      actions={<><button className="ghost-btn" onClick={onEditProject}>项目设置</button><button className="primary" onClick={() => onNewConversation()}><Plus size={15} /> 新建任务</button></>}>
      <div className="project-meta-grid">
        <div><span>授权</span><strong>{project.authorization || "未声明"}</strong></div>
        <div><span>范围</span><strong>{project.scope || "未声明"}</strong></div>
        <div><span>覆盖</span><strong>{detail.coverage.tested}/{detail.coverage.total} · {coverage}%</strong></div>
        <div><span>漏洞</span><strong>{detail.findings.length} <em>严重 {critical} / 高危 {high}</em></strong></div>
        <div><span>任务</span><strong>{detail.conversations.length}</strong></div>
      </div>

      <div className="tab-row">
        {projectTabs.map((item) => (
          <button key={item.id} className={tab === item.id ? "active" : ""} onClick={() => setTab(item.id)}>{item.label}</button>
        ))}
      </div>

      {tab === "tasks" ? (
        <>
          <section className="mode-launch">
            <header className="section-head">
              <div><h2>选择模式开始</h2><p>同一项目共享授权、资产、发现与黑板；每个模式是一次独立任务。</p></div>
            </header>
            <div className="mode-start-grid">
              {modes.map((mode) => {
                const Icon = modeIcon(mode.id);
                const count = detail.conversations.filter((run) => run.mode === mode.id).length;
                return (
                  <button key={mode.id} className={`mode-start-card mode-${mode.id}`} onClick={() => onNewConversation(mode.id)}>
                    <span className="mode-start-icon"><Icon size={19} /></span>
                    <strong>{mode.name}</strong>
                    <p>{mode.tagline}</p>
                    <footer><span>{mode.roles.length} 角色 · {count} 任务</span><span className="mode-start-go">开始 <ArrowRight size={13} /></span></footer>
                  </button>
                );
              })}
            </div>
          </section>

          <section className="task-groups">
            <header className="section-head"><div><h2>项目任务</h2><p>按模式归位；运行中的任务会实时更新。</p></div><span className="section-count">{detail.conversations.length}</span></header>
            {modeGroups.filter((group) => group.runs.length > 0).map((group) => {
              const Icon = modeIcon(group.mode.id);
              return (
                <div key={group.mode.id} className="mode-task-group">
                  <header>
                    <span className={`mode-avatar mode-${group.mode.id}`}><Icon size={15} /></span>
                    <strong>{group.mode.name}</strong>
                    <span className="muted">{group.runs.length} 个任务</span>
                    <button className="link-btn" onClick={() => onNewConversation(group.mode.id)}>新建 <Plus size={12} /></button>
                  </header>
                  <div className="card-list">
                    {group.runs.map((run) => (
                      <button key={run.id} className="task-row" onClick={() => onOpenConversation(run.id)}>
                        <span className={`status-dot ${running[run.id] ? "good" : ""}`} />
                        <span className="conv-main"><strong>{run.title || run.objective}</strong><small>{run.target} · {shortTime(run.updatedAt)}</small></span>
                        <Badge tone={statusTone(run.status)}>{statusLabels[run.status] ?? run.status}</Badge>
                        <ChevronRight size={14} />
                      </button>
                    ))}
                  </div>
                </div>
              );
            })}
            {detail.conversations.length === 0 ? <EmptyHint title="还没有任务" detail="从上方选择一种模式开始。" /> : null}
          </section>
        </>
      ) : null}

      {tab === "assets" ? <AssetTable assets={detail.assets} /> : null}
      {tab === "findings" ? <FindingList findings={detail.findings} onStatus={onStatus} onOpenConversation={onOpenConversation} /> : null}
      {tab === "graph" ? <ProjectGraph detail={detail} /> : null}
      {tab === "board" ? <BoardView detail={detail} /> : null}
    </Page>
  );
}

function ProjectGraph({ detail }: { detail: ProjectDetail }) {
  const [selected, setSelected] = useState<string | null>(null);
  const snapshot: Snapshot = useMemo(() => ({
    run: (detail.conversations[0] ?? {}) as Run,
    goals: [], constraints: [],
    nodes: detail.board.map((node) => ({
      id: node.id, runId: node.conversationId, kind: node.kind, payload: node.payload,
      state: node.state, priority: node.priority, origin: node.origin, owner: node.owner,
      createdAt: node.createdAt, updatedAt: node.updatedAt,
    })),
    edges: detail.edges.map((edge) => ({
      id: edge.id, runId: "", srcId: edge.srcId, dstId: edge.dstId, rel: edge.rel, createdAt: edge.createdAt,
    })),
    anchors: [], assets: detail.assets, evidence: [], flows: [], retests: [], events: [], tasks: [],
  }), [detail]);
  const selectedNode = detail.board.find((node) => node.id === selected);
  if (!detail.board.length) {
    return <EmptyHint title="还没有攻击链节点" detail="让代理在任务中写入决策/事实/凭据，节点会自动出现在这里。" />;
  }
  const related = selectedNode
    ? detail.edges.filter((edge) => edge.srcId === selectedNode.id || edge.dstId === selectedNode.id)
    : [];
  return (
    <div className="project-graph">
      <Graph snapshot={snapshot} selectedId={selected} onSelect={setSelected} legendKinds={["decision", "fact", "credential", "note", "hint"]} />
      <p className="inspector-hint">点击节点查看完整内容与关联边；拖拽平移，滚轮缩放。</p>
      {selectedNode ? (
        <Modal
          title={String(selectedNode.payload?.title ?? selectedNode.kind)}
          subtitle={`${selectedNode.kind} · ${selectedNode.state}`}
          onClose={() => setSelected(null)}
          wide
        >
          <div className="board-detail-meta">
            <span>ID <b className="mono">{selectedNode.id}</b></span>
            <span>归属 <b>{selectedNode.owner || selectedNode.origin || "—"}</b></span>
            <span>置顶 <b>{selectedNode.pinned ? "是" : "否"}</b></span>
            <span>创建 <b>{dateTime(selectedNode.createdAt)}</b></span>
          </div>
          <pre className="task-output">{JSON.stringify(selectedNode.payload ?? {}, null, 2)}</pre>
          <h4>关联边</h4>
          {related.length ? (
            <div className="mini-list">
              {related.map((edge) => (
                <div key={edge.id} className="mini-row">
                  <Badge tone="muted">{edge.rel}</Badge>
                  <span className="mono">{edge.srcId === selectedNode.id ? "→" : "←"} {edge.srcId === selectedNode.id ? edge.dstId : edge.srcId}</span>
                </div>
              ))}
            </div>
          ) : <p className="muted small">没有关联边。</p>}
          <footer className="modal-actions"><button className="ghost-btn" onClick={() => setSelected(null)}>关闭</button></footer>
        </Modal>
      ) : null}
    </div>
  );
}

function BoardView({ detail }: { detail: ProjectDetail }) {
  const [selected, setSelected] = useState<ProjectNode | null>(null);
  return (
    <div className="board-layout">
      <section className="panel">
        <header className="panel-head">
          <div><h3>共享黑板</h3><p>由指挥官与子代理写入；人类通过对话下达指令</p></div>
          <span className="panel-badge">{detail.board.length}</span>
        </header>
        <p className="board-note">
          黑板不是人类表单：指挥官会把决策、事实、凭据与下一步写入这里，子代理按需读取，实现跨任务协调。点击卡片查看完整内容。
        </p>
        <div className="mini-list">
          {detail.board.map((node) => {
            const payload = node.payload ?? {};
            return (
              <button key={node.id} type="button" className="board-row" onClick={() => setSelected(node)}>
                <Badge tone={node.pinned ? "good" : "muted"}>{node.kind}</Badge>
                <div><strong>{payload.title ?? node.kind}</strong><p>{payload.detail ?? ""}</p></div>
                <span className="mini-meta">{node.owner || node.origin} · {dateTime(node.createdAt)}</span>
              </button>
            );
          })}
          {detail.board.length === 0 ? <p className="muted">还没有黑板节点。让代理在任务中写入事实、决策或凭据后会自动出现。</p> : null}
        </div>
      </section>
      <section className="panel">
        <header className="panel-head"><h3>项目记忆</h3><span className="panel-badge">{detail.memory.length}</span></header>
        <p className="board-note">由代理置顶的关键结论，自动注入后续对话与子代理上下文。</p>
        <div className="mini-list">
          {detail.memory.map((memory) => (
            <div key={memory.id} className="memory-row"><Badge tone="muted">{memory.kind}</Badge><strong>{memory.key}</strong><span>{memory.value}</span></div>
          ))}
          {detail.memory.length === 0 ? <p className="muted">还没有记忆。</p> : null}
        </div>
      </section>
      {selected ? (
        <Modal
          title={String(selected.payload?.title ?? selected.kind)}
          subtitle={`${selected.kind} · ${selected.state}`}
          onClose={() => setSelected(null)}
          wide
        >
          <div className="board-detail-meta">
            <span>ID <b className="mono">{selected.id}</b></span>
            <span>归属 <b>{selected.owner || selected.origin || "—"}</b></span>
            <span>可见性 <b>{selected.visibility || "—"}</b></span>
            <span>创建 <b>{dateTime(selected.createdAt)}</b></span>
          </div>
          <pre className="task-output">{JSON.stringify(selected.payload ?? {}, null, 2)}</pre>
          <footer className="modal-actions"><button className="ghost-btn" onClick={() => setSelected(null)}>关闭</button></footer>
        </Modal>
      ) : null}
    </div>
  );
}

export function AssetsPage({ assets }: { assets: Asset[] }) {
  const [query, setQuery] = useState("");
  const [type, setType] = useState("");
  const [selected, setSelected] = useState<Asset | null>(null);
  const types = Array.from(new Set(assets.map((asset) => asset.type))).sort();
  const filtered = assets.filter((asset) => {
    const haystack = `${asset.value} ${asset.company} ${asset.meta} ${asset.type}`.toLowerCase();
    return (!query || haystack.includes(query.toLowerCase())) && (!type || asset.type === type);
  });
  return (
    <Page title="资产管理" subtitle="跨项目资产真值库；覆盖与关联漏洞在项目任务内计算"
      actions={<button className="ghost-btn" onClick={() => downloadCSV("assets.csv", [["类型", "资产", "公司", "首次发现", "最近发现"], ...filtered.map((asset) => [asset.type, asset.value, asset.company, asset.firstSeenRun, asset.lastSeen])])}><Download size={14} /> 导出 CSV</button>}>
      <div className="stat-strip">
        <div><span>资产总数</span><b>{assets.length}</b></div>
        <div><span>类型</span><b>{types.length}</b></div>
        <div><span>公司/组织</span><b>{new Set(assets.map((asset) => asset.company).filter(Boolean)).size}</b></div>
        <div><span>筛选结果</span><b>{filtered.length}</b></div>
      </div>
      <div className="filter-bar">
        <label className="search-box"><Search size={15} /><input placeholder="搜索资产、公司、元数据…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <select value={type} onChange={(e) => setType(e.target.value)}>
          <option value="">全部类型</option>
          {types.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <span className="filter-count">{filtered.length} / {assets.length}</span>
      </div>
      <AssetTable assets={filtered} onSelect={setSelected} />
      {selected ? (
        <div className="drawer-backdrop" onClick={() => setSelected(null)}>
          <aside className="drawer" onClick={(e) => e.stopPropagation()}>
            <header><div><Badge tone="muted">{selected.type}</Badge><h2>{selected.value}</h2></div><button className="icon-btn" onClick={() => setSelected(null)}><X size={16} /></button></header>
            <dl className="detail-list">
              <div><dt>标识</dt><dd>{selected.identifier || "—"}</dd></div>
              <div><dt>公司/组织</dt><dd>{selected.company || "—"}</dd></div>
              <div><dt>首次发现</dt><dd>{selected.firstSeenRun || "—"}</dd></div>
              <div><dt>最近发现</dt><dd>{dateTime(selected.lastSeen)}</dd></div>
              <div><dt>元数据</dt><dd className="mono">{selected.meta || "—"}</dd></div>
            </dl>
          </aside>
        </div>
      ) : null}
    </Page>
  );
}

function AssetTable({ assets, onSelect }: { assets: Asset[]; onSelect?: (asset: Asset) => void }) {
  return (
    <div className="data-table">
      <div className="data-head" style={{ gridTemplateColumns: "90px minmax(0,1fr) 160px 140px" }}>
        <span>类型</span><span>资产</span><span>公司/组织</span><span>最近发现</span>
      </div>
      {assets.map((asset) => {
        const cells = (
          <>
            <span className="asset-type">{asset.type}</span>
            <span className="asset-value" title={asset.value}>{asset.value}</span>
            <span className="muted">{asset.company || "—"}</span>
            <span className="muted">{asset.lastSeen ? shortTime(asset.lastSeen) : "—"}</span>
          </>
        );
        return onSelect ? (
          <button key={asset.id} className="data-row" style={{ gridTemplateColumns: "90px minmax(0,1fr) 160px 140px" }} onClick={() => onSelect(asset)}>{cells}</button>
        ) : (
          <div key={asset.id} className="data-row static" style={{ gridTemplateColumns: "90px minmax(0,1fr) 160px 140px" }}>{cells}</div>
        );
      })}
      {assets.length === 0 ? <p className="muted table-empty">还没有资产；代理在任务中写入后会自动出现。</p> : null}
    </div>
  );
}

export function FindingsPage({
  findings, onOpenConversation, onStatus,
}: { findings: ProjectFinding[]; onOpenConversation: (id: string) => void; onStatus: (id: string, state: string) => Promise<void> }) {
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState("");
  const [state, setState] = useState("");
  const [project, setProject] = useState("");
  const projects = Array.from(new Map(findings.map((finding) => [finding.projectId, finding.projectName])).entries());
  const filtered = findings.filter((finding) => {
    const haystack = `${finding.payload.title} ${finding.payload.summary ?? ""} ${finding.projectName} ${finding.conversationTitle} ${finding.payload.vulnclass ?? ""}`.toLowerCase();
    return (!query || haystack.includes(query.toLowerCase()))
      && (!severity || finding.payload.severity === severity)
      && (!state || (finding.state || "open") === state)
      && (!project || finding.projectId === project);
  });
  const counts = Object.fromEntries(severityOrder.map((key) => [key, findings.filter((finding) => finding.payload.severity === key).length]));
  return (
    <Page title="漏洞管理" subtitle="跨项目、跨任务的确认漏洞；状态流转会写回证据图"
      actions={<button className="ghost-btn" onClick={() => downloadCSV("findings.csv", [["严重程度", "标题", "项目", "任务", "状态", "摘要"], ...filtered.map((finding) => [finding.payload.severity, finding.payload.title, finding.projectName, finding.conversationTitle, finding.state, finding.payload.summary ?? ""])])}><Download size={14} /> 导出 CSV</button>}>
      <div className="severity-strip">
        {severityOrder.map((key) => (
          <button key={key} className={`severity-stat sev-${key} ${severity === key ? "active" : ""}`} onClick={() => setSeverity(severity === key ? "" : key)}>
            <span>{severityLabels[key]}</span><b>{counts[key] ?? 0}</b><em>{percent(counts[key] ?? 0, findings.length)}%</em>
          </button>
        ))}
      </div>
      <div className="filter-bar">
        <label className="search-box"><Search size={15} /><input placeholder="搜索标题、摘要、项目、任务…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <select value={project} onChange={(e) => setProject(e.target.value)}><option value="">全部项目</option>{projects.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select>
        <select value={state} onChange={(e) => setState(e.target.value)}><option value="">全部状态</option>{findingStatuses.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select>
        <span className="filter-count">{filtered.length} / {findings.length}</span>
      </div>
      <FindingList findings={filtered} onStatus={onStatus} onOpenConversation={onOpenConversation} />
    </Page>
  );
}

function FindingList({
  findings, onStatus, onOpenConversation,
}: { findings: ProjectFinding[]; onStatus?: (id: string, state: string) => Promise<void>; onOpenConversation?: (id: string) => void }) {
  const terminalStates = new Set(["fixed", "ignored", "false_positive"]);
  async function changeStatus(finding: ProjectFinding, next: string) {
    const label = findingStatuses.find((item) => item.value === next)?.label ?? next;
    if (terminalStates.has(next) && !window.confirm(`将「${finding.payload.title}」标记为「${label}」？`)) return;
    await onStatus?.(finding.id, next);
  }
  if (!findings.length) return <EmptyHint title="没有匹配的漏洞" detail="调整筛选条件，或确认任务里已经提交了可复现漏洞。" />;
  return (
    <div className="panel-stack">
      {findings.map((finding) => {
        const payload = finding.payload;
        return (
          <article key={finding.id} className={`finding-card sev-${severityTone[payload.severity] ?? "info"}`}>
            <header>
              <div className="finding-title">
                <Badge tone={severityTone[payload.severity] ?? "info"}>{severityLabels[payload.severity] ?? payload.severity}</Badge>
                <strong>{payload.title}</strong>
                {payload.vulnclass ? <span className="chip">{payload.vulnclass}</span> : null}
              </div>
              <div className="finding-actions">
                {onStatus ? (
                  <select className="status-select" value={finding.state || "open"} onChange={(e) => void changeStatus(finding, e.target.value)}>
                    {findingStatuses.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                  </select>
                ) : null}
                {onOpenConversation ? <button className="link-btn" onClick={() => onOpenConversation(finding.conversationId)}>打开任务 <ArrowRight size={12} /></button> : null}
              </div>
            </header>
            <div className="finding-meta"><span>{finding.projectName}</span><span>{finding.conversationTitle}</span><span>{shortTime(finding.createdAt)}</span></div>
            {payload.summary ? <p>{payload.summary}</p> : null}
            {(payload.reproducibleSteps ?? []).length ? (
              <ol className="steps">{payload.reproducibleSteps.map((step, index) => <li key={index}>{step}</li>)}</ol>
            ) : null}
            <footer className="finding-foot">
              <span><ShieldCheck size={13} /> 证据 {payload.evidenceIds?.length ?? 0}</span>
              {payload.trustBoundary ? <span>信任边界：{payload.trustBoundary}</span> : null}
            </footer>
          </article>
        );
      })}
    </div>
  );
}

export function TasksPage({
  tasks, runs, projects, roles, mainPrompts, onOpenConversation, onCancel, taskStreams,
}: {
  tasks: Task[]; runs: RunSummary[]; projects: ProjectSummary[]; roles: RoleMeta[]; mainPrompts: MainPromptMeta[];
  onOpenConversation: (id: string) => void; onCancel: (id: string) => Promise<void>;
  taskStreams?: Record<string, string>;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [project, setProject] = useState("");
  const [selected, setSelected] = useState<Task | null>(null);
  const [roleDetail, setRoleDetail] = useState<RoleMeta | null>(null);
  const [mainDetail, setMainDetail] = useState<MainPromptMeta | null>(null);
  const runMap = new Map(runs.map((run) => [run.id, run]));
  const projectMap = new Map(projects.map((item) => [item.id, item.name]));
  const counts = { running: 0, queued: 0, done: 0, failed: 0, cancelled: 0 } as Record<string, number>;
  tasks.forEach((task) => { counts[task.status] = (counts[task.status] ?? 0) + 1; });
  const filtered = tasks.filter((task) => {
    const run = runMap.get(task.runId);
    const haystack = `${task.title} ${task.role} ${task.id} ${run?.title ?? ""} ${run?.target ?? ""}`.toLowerCase();
    return (!query || haystack.includes(query.toLowerCase()))
      && (!status || task.status === status)
      && (!project || run?.projectId === project);
  });
  return (
    <Page title="子代理" subtitle="任务里的角色化执行单元；运行中可取消">
      <div className="stat-strip">
        <div><span>运行中</span><b>{counts.running}</b></div>
        <div><span>排队</span><b>{counts.queued}</b></div>
        <div><span>已完成</span><b>{counts.done}</b></div>
        <div><span>失败/取消</span><b>{counts.failed + counts.cancelled}</b></div>
      </div>
      <div className="filter-bar">
        <label className="search-box"><Search size={15} /><input placeholder="搜索标题、角色、任务 ID…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <select value={project} onChange={(e) => setProject(e.target.value)}><option value="">全部项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">全部状态</option>
          <option value="running">运行中</option><option value="queued">排队</option>
          <option value="done">已完成</option><option value="failed">失败</option><option value="cancelled">已取消</option>
        </select>
        <span className="filter-count">{filtered.length} / {tasks.length}</span>
      </div>
      <div className="data-table">
        <div className="data-head" style={{ gridTemplateColumns: "120px minmax(0,1.4fr) minmax(0,1fr) 110px 100px 150px" }}>
          <span>角色</span><span>标题</span><span>所属任务</span><span>状态</span><span>耗时</span><span>操作</span>
        </div>
        {filtered.map((task) => {
          const run = runMap.get(task.runId);
          return (
            <div key={task.id} className="data-row" style={{ gridTemplateColumns: "120px minmax(0,1.4fr) minmax(0,1fr) 110px 100px 150px" }}>
              <span className="role-chip">{task.role}</span>
              <span className="task-title-cell"><strong>{task.title}</strong><small>{task.id}</small></span>
              <span className="task-run-cell"><strong>{run?.title ?? task.runId}</strong><small>{run ? projectMap.get(run.projectId) : "—"}</small></span>
              <Badge tone={statusTone(task.status)}>{statusLabels[task.status] ?? task.status}</Badge>
              <span className="muted">{taskDuration(task)}</span>
              <span className="row-actions">
                <button className="link-btn" onClick={() => setSelected(task)}><Eye size={12} /> 详情</button>
                {run ? <button className="link-btn" onClick={() => onOpenConversation(run.id)}>打开 <ArrowRight size={12} /></button> : null}
                {task.status === "running" || task.status === "queued" ? (
                  <button className="link-btn danger" onClick={() => void onCancel(task.id)}><Square size={12} /> 取消</button>
                ) : null}
              </span>
            </div>
          );
        })}
        {filtered.length === 0 ? <p className="muted table-empty">{tasks.length === 0 ? "还没有子代理任务；让指挥官在任务里派活后会出现在这里。" : "没有匹配的子代理任务。"}</p> : null}
      </div>
      <section className="role-catalog">
        <div className="catalog-head">
          <div>
            <h2>角色与方法论</h2>
            <p>内置子代理与主代理提示词；不需要先跑任务。</p>
          </div>
          <span className="chip">{roles.length} 个角色</span>
        </div>
        <div className="role-grid">
          {roles.map((role) => (
            <button key={role.code} className="role-card" onClick={() => setRoleDetail(role)}>
              <div className="role-card-top">
                <span className="mono muted">{role.code}</span>
                <span className="chip">{role.modes.length} 模式</span>
              </div>
              <strong>{role.name}</strong>
              <p>{role.description}</p>
              <div className="tool-chips">
                {(role.tools ?? []).slice(0, 6).map((tool) => <span key={tool} className="chip mono">{tool}</span>)}
                {(role.tools?.length ?? 0) > 6 ? <span className="chip">+{role.tools.length - 6}</span> : null}
              </div>
            </button>
          ))}
          {roles.length === 0 ? <p className="muted">角色目录加载中或当前服务未返回。</p> : null}
        </div>
        <div className="catalog-head main-head">
          <div>
            <h3>主代理方法论</h3>
            <p>每个模式的主代理职责、工具边界和角色分工。</p>
          </div>
        </div>
        <div className="main-prompt-grid">
          {mainPrompts.map((prompt) => (
            <button key={prompt.id} className="main-prompt-card" onClick={() => setMainDetail(prompt)}>
              <strong>{prompt.name}</strong>
              <p>{prompt.tagline}</p>
              <span className="muted small">{prompt.roles.length} 个角色 · {prompt.model || "跟随任务模型"}</span>
            </button>
          ))}
        </div>
      </section>
      {roleDetail ? (
        <Modal title={`${roleDetail.name} · ${roleDetail.code}`} subtitle={roleDetail.model || "跟随任务模型"} onClose={() => setRoleDetail(null)} wide>
          <div className="role-meta-grid">
            <div><span>参与模式</span><div className="tool-chips">{roleDetail.modes.map((name) => <span key={name} className="chip">{name}</span>)}</div></div>
            <div><span>可用工具</span><div className="tool-chips">{(roleDetail.tools ?? []).map((tool) => <span key={tool} className="chip mono">{tool}</span>)}</div></div>
          </div>
          <h4>系统提示词 / 方法论</h4>
          <pre className="role-prompt">{roleDetail.prompt}</pre>
          <KnowledgePanel role={roleDetail.code} />
          <footer className="modal-actions"><button className="ghost-btn" onClick={() => setRoleDetail(null)}>关闭</button></footer>
        </Modal>
      ) : null}
      {mainDetail ? (
        <Modal title={`${mainDetail.name} · 主代理`} subtitle={mainDetail.model || "跟随任务模型"} onClose={() => setMainDetail(null)} wide>
          <div className="role-meta-grid">
            <div><span>协作角色</span><div className="tool-chips">{mainDetail.roles.map((role) => <span key={role} className="chip">{role}</span>)}</div></div>
            <div><span>可用工具</span><div className="tool-chips">{(mainDetail.tools ?? []).map((tool) => <span key={tool} className="chip mono">{tool}</span>)}</div></div>
          </div>
          <h4>主代理提示词 / 方法论</h4>
          <pre className="role-prompt">{mainDetail.prompt}</pre>
          <KnowledgePanel role="main" />
          <footer className="modal-actions"><button className="ghost-btn" onClick={() => setMainDetail(null)}>关闭</button></footer>
        </Modal>
      ) : null}
      {selected ? (
        <TaskDetailModal task={selected} onClose={() => setSelected(null)}
          onOpenConversation={onOpenConversation} onCancel={onCancel} liveText={taskStreams?.[selected.id]} />
      ) : null}
    </Page>
  );
}

export function TaskDetailModal({
  task, onClose, onOpenConversation, onCancel, liveText,
}: {
  task: Task; onClose: () => void;
  onOpenConversation?: (runId: string) => void;
  onCancel?: (taskId: string) => Promise<void>;
  liveText?: string;
}) {
  const [detail, setDetail] = useState<TaskDetail | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    let timer: number | undefined;
    const load = async () => {
      try {
        const next = await api.taskDetail(task.id);
        if (!active) return;
        setDetail(next);
        setError("");
        if (next.task.status !== "running" && next.task.status !== "queued" && timer !== undefined) {
          window.clearInterval(timer);
          timer = undefined;
        }
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : String(err));
      }
    };
    timer = window.setInterval(load, 2500);
    void load();
    return () => {
      active = false;
      if (timer !== undefined) window.clearInterval(timer);
    };
  }, [task.id]);

  const current = detail?.task ?? task;
  const live = liveText || "";
  const run = detail?.run;
  const project = detail?.project;
  const events = detail?.events ?? [];
  const active = current.status === "running" || current.status === "queued";

  async function cancel() {
    if (!onCancel) return;
    await onCancel(current.id);
    try {
      setDetail(await api.taskDetail(task.id));
    } catch {
      /* the poll will refresh */
    }
  }

  return (
    <Modal title={`${current.role} · 子代理详情`} subtitle={current.title} onClose={onClose} wide>
      <div className="task-detail-grid">
        <div><span>状态</span><Badge tone={statusTone(current.status)}>{statusLabels[current.status] ?? current.status}</Badge></div>
        <div><span>任务 ID</span><small className="mono">{current.id}</small></div>
        <div><span>所属任务</span><strong>{run?.title ?? current.runId}</strong><small>{project?.name ?? "—"}</small></div>
        <div><span>预算</span><strong>{current.budget ? String(current.budget) : "不限制"}</strong></div>
        <div><span>创建</span><small>{dateTime(current.createdAt)}</small></div>
        <div><span>开始</span><small>{current.startedAt ? dateTime(current.startedAt) : "—"}</small></div>
        <div><span>结束</span><small>{current.finishedAt ? dateTime(current.finishedAt) : "—"}</small></div>
        <div><span>耗时</span><strong>{taskDuration(current)}</strong></div>
      </div>

      <section className="task-detail-section">
        <div className="task-section-head">
          <h4>正在做什么</h4>
          {active ? <span className="badge badge-blue">每 2.5 秒刷新</span> : null}
        </div>
        {error ? <p className="error-text">{error}</p> : null}
        {events.length === 0 ? <p className="muted small">暂无事件；子代理可能刚刚启动，或只写了最终汇总。</p> : (
          <div className="task-events">
            {events.slice(-40).map((event) => (
              <div key={event.id} className="task-event">
                <div className="task-event-head">
                  <span className="muted mono">{shortTime(event.createdAt)}</span>
                  <Badge tone={event.kind === "error" ? "bad" : event.kind === "tool_call" ? "blue" : "muted"}>
                    {eventKindLabel(event.kind)}
                  </Badge>
                  {event.tool ? <span className="mono task-event-tool">{event.tool}</span> : null}
                </div>
                {event.summary ? <p>{event.summary}</p> : null}
                {event.detail ? (
                  <details>
                    <summary>查看详情</summary>
                    <pre>{truncate(event.detail, 4000)}</pre>
                  </details>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </section>

      {live ? (
        <section className="task-detail-section">
          <h4>实时输出</h4>
          <pre className="task-output task-live">{live}</pre>
        </section>
      ) : null}

      <section className="task-detail-section">
        <h4>结果</h4>
        {current.result ? <pre className="task-output">{current.result}</pre> : <p className="muted small">还没有结果。</p>}
      </section>
      {current.error ? <section className="task-detail-section"><h4>错误</h4><pre className="task-output error-text">{current.error}</pre></section> : null}
      <section className="task-detail-section">
        <h4>Inbox（运行中插话）</h4>
        {current.inbox?.length ? current.inbox.map((message, index) => (
          <div key={`${message.at}-${index}`} className="inbox-row"><span>{dateTime(message.at)}</span><p>{message.text}</p></div>
        )) : <p className="muted small">没有插话消息。</p>}
      </section>
      <footer className="modal-actions">
        {onOpenConversation ? <button className="ghost-btn" onClick={() => { onOpenConversation(current.runId); onClose(); }}>打开所属任务 <ArrowRight size={12} /></button> : null}
        {active && onCancel ? <button className="ghost-btn danger" onClick={() => void cancel()}><Square size={12} /> 取消子代理</button> : null}
        <button className="ghost-btn" onClick={onClose}>关闭</button>
      </footer>
    </Modal>
  );
}

function KnowledgePanel({ role }: { role: string }) {
  const [docs, setDocs] = useState<KnowledgeMeta[]>([]);
  const [selected, setSelected] = useState("");
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function openDoc(target: string) {
    try {
      const doc: KnowledgeDoc = await api.knowledgeDoc(role, target);
      setSelected(doc.name);
      setName(doc.name);
      setDescription(doc.description);
      setBody(doc.body);
      setEditing(false);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  useEffect(() => {
    let active = true;
    setSelected("");
    setEditing(false);
    setError("");
    void api.knowledge(role).then(async (list) => {
      if (!active) return;
      setDocs(list);
      if (list[0]) {
        const doc = await api.knowledgeDoc(role, list[0].name);
        if (active) {
          setSelected(doc.name);
          setName(doc.name);
          setDescription(doc.description);
          setBody(doc.body);
          setEditing(false);
        }
      } else {
        setEditing(true);
        setName("");
        setDescription("");
        setBody("");
      }
    }).catch((err) => { if (active) setError(err instanceof Error ? err.message : String(err)); });
    return () => { active = false; };
  }, [role]);

  function newDoc() {
    setSelected("");
    setEditing(true);
    setName("");
    setDescription("");
    setBody("");
    setError("");
  }

  async function save() {
    if (!name.trim() || !description.trim()) {
      setError("名称与描述必填。");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const doc = await api.saveKnowledge(role, { name: name.trim(), description: description.trim(), body });
      setSelected(doc.name);
      setEditing(false);
      setDocs(await api.knowledge(role));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="knowledge-panel">
      <div className="knowledge-head">
        <div>
          <h4>角色知识库</h4>
          <p className="muted small">只存可复用的方法论；不要存日志、密钥、凭据或一次性命令输出。</p>
        </div>
        <button className="ghost-btn" onClick={newDoc}><Plus size={13} /> 新建</button>
      </div>
      <div className="knowledge-layout">
        <div className="knowledge-list">
          {docs.map((doc) => (
            <button key={doc.name} className={`knowledge-item ${selected === doc.name ? "active" : ""}`} onClick={() => void openDoc(doc.name)}>
              <strong>{doc.name}</strong>
              <small>{doc.description}</small>
            </button>
          ))}
          {docs.length === 0 ? <p className="muted small">还没有条目。</p> : null}
        </div>
        <div className="knowledge-editor">
          {selected || editing ? (
            <>
              <div className="knowledge-fields">
                <label className="field"><span>名称</span><input value={name} onChange={(e) => setName(e.target.value)} disabled={Boolean(selected) && !editing} placeholder="recon-method" /></label>
                <label className="field"><span>描述</span><input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="何时使用、解决什么问题" /></label>
              </div>
              <label className="field"><span>方法论正文</span><textarea rows={9} value={body} onChange={(e) => setBody(e.target.value)} disabled={Boolean(selected) && !editing} placeholder="步骤、判断标准、反例与证据要求…" /></label>
              <div className="knowledge-actions">
                {selected && !editing ? (
                  <button className="ghost-btn" onClick={() => setEditing(true)}><Pencil size={13} /> 编辑</button>
                ) : (
                  <button className="primary" disabled={busy} onClick={() => void save()}>{busy ? "保存中…" : "保存"}</button>
                )}
              </div>
            </>
          ) : <p className="muted small">选择左侧条目查看，或新建一条。</p>}
          {error ? <p className="error-text small">{error}</p> : null}
        </div>
      </div>
    </section>
  );
}

function taskDuration(task: Task) {
  const start = Date.parse(task.startedAt || task.createdAt);
  const end = task.finishedAt ? Date.parse(task.finishedAt) : Date.now();
  if (Number.isNaN(start) || Number.isNaN(end)) return "—";
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

export function ApprovalsPage({
  approvals, runs, projects, onDecide, onOpenConversation,
}: {
  approvals: Approval[]; runs: RunSummary[]; projects: ProjectSummary[];
  onDecide: (id: string, approve: boolean) => Promise<void>;
  onOpenConversation: (id: string) => void;
}) {
  const runMap = new Map(runs.map((run) => [run.id, run]));
  const projectMap = new Map(projects.map((project) => [project.id, project.name]));
  function approve(approval: Approval) {
    if (window.confirm(`批准后 ${approval.tool} 会立即执行，确定？`)) {
      void onDecide(approval.id, true);
    }
  }
  return (
    <Page title="人机协同" subtitle="默认关闭；需要时在「系统设置 → 运行」把审批切到 ask。开启后，危险工具调用会在执行前等待确认。">
      {approvals.length === 0 ? <EmptyHint title="当前没有待审批操作" detail="人机协同默认关闭（auto）。需要审批时在系统设置里切到 ask。" /> : (
        <div className="approval-list">
          {approvals.map((approval) => {
            const run = runMap.get(approval.runId);
            return (
              <article key={approval.id} className="approval-card">
                <header>
                  <div className="approval-head"><span className="approval-icon"><ShieldAlert size={17} /></span><div><strong>{approval.tool}</strong><small>{run?.title ?? approval.runId} {run ? `· ${projectMap.get(run.projectId) ?? ""}` : ""}</small></div></div>
                  <Badge tone="warn">待审批</Badge>
                </header>
                <p className="approval-reason">{approval.reason || "策略要求人工确认"}</p>
                <pre className="approval-input">{approval.input}</pre>
                <footer>
                  <span className="muted">{dateTime(approval.createdAt)}</span>
                  <span className="row-actions">
                    {run ? <button className="link-btn" onClick={() => onOpenConversation(run.id)}>打开任务</button> : null}
                    <button className="ghost-btn danger" onClick={() => void onDecide(approval.id, false)}><X size={13} /> 拒绝</button>
                    <button className="primary" onClick={() => approve(approval)}><Check size={13} /> 批准</button>
                  </span>
                </footer>
              </article>
            );
          })}
        </div>
      )}
    </Page>
  );
}

export function MCPPage({ config, status, onSave, onReconnect }: {
  config: AppConfig | null; status: MCPStatus[]; onSave: (config: AppConfig) => Promise<void>;
  onReconnect: () => Promise<void>;
}) {
  const [servers, setServers] = useState<MCPServerConfig[]>(() => config?.mcp ?? []);
  const [editing, setEditing] = useState<{ index: number; draft: MCPServerConfig } | null>(null);
  const [probe, setProbe] = useState<{ id: string; ok: boolean; text: string; tools: string[] } | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => { setServers(config?.mcp ?? []); }, [config?.mcp]);

  const statusMap = useMemo(() => new Map(status.map((server) => [server.id, server])), [status]);

  async function commit(next: MCPServerConfig[]): Promise<boolean> {
    if (!config) {
      setError("配置尚未加载，请稍后重试");
      return false;
    }
    const ids = next.map((server) => server.id.trim().toLowerCase());
    if (new Set(ids).size !== ids.length) {
      setError("MCP 服务器名称不能重复");
      return false;
    }
    setBusy(true);
    setError("");
    try {
      await onSave({ ...config, mcp: next });
      setServers(next);
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function probeServer(server: MCPServerConfig) {
    setProbe({ id: server.id, ok: false, text: "连接中…", tools: [] });
    try {
      const result = await api.testMCP(server);
      setProbe({
        id: server.id,
        ok: result.ok,
        text: result.ok ? `连接成功 · ${result.status.tools} 个工具` : (result.error || result.status.error || "连接失败"),
        tools: result.status.toolNames ?? [],
      });
    } catch (err) {
      setProbe({ id: server.id, ok: false, text: err instanceof Error ? err.message : String(err), tools: [] });
    }
  }

  function saveServer(server: MCPServerConfig) {
    const next = editing && editing.index >= 0
      ? servers.map((item, i) => (i === editing.index ? server : item))
      : [...servers, server];
    void commit(next).then((ok) => { if (ok) setEditing(null); });
  }

  return (
    <Page title="MCP" subtitle="stdio / Streamable HTTP 服务器；保存后重连，工具自动注册"
      actions={<>
        <button className="ghost-btn" disabled={busy} onClick={() => setEditing({ index: -1, draft: { id: `mcp-${Date.now().toString(36)}`, transport: "stdio", command: "npx", args: [], enabled: true } })}><Plus size={15} /> 添加服务器</button>
        <button className="ghost-btn" disabled={busy} onClick={() => void onReconnect()}><RefreshCw size={14} /> 重连</button>
        <button className="primary" disabled={busy} onClick={() => void commit(servers)}>{busy ? "重连中…" : <><RefreshCw size={14} /> 保存并重连</>}</button>
      </>}>
      {probe ? (
        <div className={`mcp-probe ${probe.ok ? "ok" : "bad"}`}>
          <strong>{probe.id}</strong>
          <span>{probe.text}</span>
          {probe.tools.length ? <div className="tool-chips">{probe.tools.map((name) => <span key={name} className="chip mono">{name}</span>)}</div> : null}
        </div>
      ) : null}
      {error ? <p className="error-text">{error}</p> : null}
      <div className="mcp-grid">
        {servers.map((server, index) => {
          const state = statusMap.get(server.id);
          const good = state?.state === "ready";
          return (
            <article key={`${server.id}-${index}`} className="mcp-card">
              <header>
                <span className={`test-dot ${good ? "good" : state?.state === "disabled" || !server.enabled ? "" : "bad"}`} />
                <strong>{server.id}</strong>
                <Badge tone={!server.enabled ? "muted" : good ? "good" : "bad"}>{server.enabled ? (state?.state ?? "未连接") : "已停用"}</Badge>
                <button className="link-btn" disabled={busy} onClick={() => void commit(servers.map((item, i) => i === index ? { ...item, enabled: !item.enabled } : item))}>
                  {server.enabled ? "停用" : "启用"}
                </button>
              </header>
              <p className="mcp-path">{server.transport === "stdio" ? `${server.command ?? ""} ${(server.args ?? []).join(" ")}` : server.url}</p>
              {state?.error ? <p className="error-text small">{state.error}</p> : null}
              {state?.toolNames?.length ? <div className="tool-chips">{state.toolNames.map((name) => <span key={name} className="chip mono">{name}</span>)}</div> : <p className="muted small">{server.enabled ? "尚无工具 / 等待重连" : "启用后重连以加载工具"}</p>}
              <footer className="mcp-actions">
                <button className="link-btn" onClick={() => void probeServer(server)}><Play size={12} /> 测试</button>
                <button className="link-btn" onClick={() => setEditing({ index, draft: server })}><Pencil size={12} /> 编辑</button>
                <button className="link-btn danger" disabled={busy} onClick={() => { if (window.confirm(`删除 MCP 服务器 ${server.id}？`)) void commit(servers.filter((_, i) => i !== index)); }}><Trash2 size={12} /> 删除</button>
              </footer>
            </article>
          );
        })}
        {servers.length === 0 ? <EmptyHint title="还没有 MCP 服务器" detail="添加 stdio 命令或 HTTP 地址，保存后会自动连接并列出工具。" /> : null}
      </div>
      <p className="muted small">stdio 适合本机命令；HTTP 适合远端长期服务。测试只验证草稿，不会改动当前连接。</p>
      {editing ? <MCPServerDialog initial={editing.draft} onClose={() => setEditing(null)} onSave={saveServer} /> : null}
    </Page>
  );
}

function MCPServerDialog({ initial, onClose, onSave }: {
  initial: MCPServerConfig; onClose: () => void; onSave: (server: MCPServerConfig) => void;
}) {
  const [draft, setDraft] = useState<MCPServerConfig>({ ...initial });
  const [argsText, setArgsText] = useState((initial.args ?? []).join("\n"));
  const [envText, setEnvText] = useState(kvLines(initial.env));
  const [headersText, setHeadersText] = useState(kvLines(initial.headers, ": "));
  const [probe, setProbe] = useState<{ ok: boolean; text: string; tools: string[] } | null>(null);
  const [error, setError] = useState("");

  function build(): MCPServerConfig {
    return {
      ...draft,
      id: draft.id.trim(),
      command: draft.command?.trim(),
      url: draft.url?.trim(),
      args: argsText.split("\n").map((line) => line.trim()).filter(Boolean),
      env: parseKV(envText, "="),
      headers: parseKV(headersText, ":"),
    };
  }

  async function test() {
    setError("");
    setProbe({ ok: false, text: "连接中…", tools: [] });
    try {
      const result = await api.testMCP(build());
      setProbe({ ok: result.ok, text: result.ok ? `连接成功 · ${result.status.tools} 个工具` : (result.error || result.status.error || "连接失败"), tools: result.status.toolNames ?? [] });
    } catch (err) {
      setProbe({ ok: false, text: err instanceof Error ? err.message : String(err), tools: [] });
    }
  }

  function save() {
    const server = build();
    if (!server.id) { setError("服务器名称必填"); return; }
    if (server.transport === "stdio" && !server.command) { setError("stdio 需要 command"); return; }
    if (server.transport === "http" && !server.url) { setError("HTTP 需要 URL"); return; }
    onSave(server);
  }

  return (
    <Modal title="MCP 服务器" subtitle="stdio 或 Streamable HTTP；密钥/Token 放在 env 或 headers" onClose={onClose} wide>
      <div className="form-grid">
        <label className="field"><span>名称 / ID</span><input value={draft.id} onChange={(e) => setDraft({ ...draft, id: e.target.value })} placeholder="filesystem" /></label>
        <label className="field"><span>传输</span><select value={draft.transport} onChange={(e) => setDraft({ ...draft, transport: e.target.value })}><option value="stdio">stdio</option><option value="http">Streamable HTTP</option></select></label>
        <label className="field field-span checkbox-field"><input type="checkbox" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} /> 启用并加载工具</label>
        {draft.transport === "stdio" ? <>
          <label className="field field-span"><span>命令</span><input value={draft.command ?? ""} onChange={(e) => setDraft({ ...draft, command: e.target.value })} placeholder="npx" /></label>
          <label className="field field-span"><span>参数（每行一个）</span><textarea rows={4} className="mono" value={argsText} onChange={(e) => setArgsText(e.target.value)} placeholder={"-y\n@modelcontextprotocol/server-filesystem\n/tmp"} /></label>
          <label className="field field-span"><span>环境变量（KEY=VALUE，每行一个）</span><textarea rows={3} className="mono" value={envText} onChange={(e) => setEnvText(e.target.value)} /></label>
        </> : <>
          <label className="field field-span"><span>URL</span><input value={draft.url ?? ""} onChange={(e) => setDraft({ ...draft, url: e.target.value })} placeholder="https://host/mcp" /></label>
          <label className="field field-span"><span>请求头（Key: Value，每行一个）</span><textarea rows={3} className="mono" value={headersText} onChange={(e) => setHeadersText(e.target.value)} placeholder={"Authorization: Bearer ..."} /></label>
        </>}
      </div>
      {probe ? <div className={`mcp-probe ${probe.ok ? "ok" : "bad"}`}><span>{probe.text}</span>{probe.tools.length ? <div className="tool-chips">{probe.tools.map((name) => <span key={name} className="chip mono">{name}</span>)}</div> : null}</div> : null}
      {error ? <p className="error-text">{error}</p> : null}
      <footer className="modal-actions">
        <button className="ghost-btn" onClick={() => void test()}>测试连接</button>
        <button className="ghost-btn" onClick={onClose}>取消</button>
        <button className="primary" onClick={save}>保存服务器</button>
      </footer>
    </Modal>
  );
}

function kvLines(obj?: Record<string, string>, sep = "=") {
  return Object.entries(obj ?? {}).map(([key, value]) => `${key}${sep}${value}`).join("\n");
}

function parseKV(text: string, sep: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const index = trimmed.indexOf(sep);
    if (index <= 0) continue;
    out[trimmed.slice(0, index).trim()] = trimmed.slice(index + sep.length).trim();
  }
  return out;
}

export function SkillsPage({ skills, onSave }: {
  skills: SkillInfo[];
  onSave: (input: { name: string; description: string; body: string }) => Promise<unknown>;
}) {
  const [editing, setEditing] = useState<{ name: string; description: string; body: string; isNew: boolean } | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function open(skill?: SkillInfo) {
    setError("");
    if (!skill) {
      setEditing({ name: "", description: "", body: "# 标题\n\n## 何时使用\n\n## 步骤\n", isNew: true });
      return;
    }
    setLoading(true);
    try {
      const full = await api.skill(skill.name);
      setEditing({ name: full.name, description: full.description, body: full.body ?? "", isNew: false });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!editing) return;
    if (!editing.name.trim()) { setError("技能名称必填"); return; }
    if (editing.isNew && skills.some((skill) => skill.name.toLowerCase() === editing.name.trim().toLowerCase())) {
      setError("同名技能已存在，请换一个名称");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onSave({ name: editing.name, description: editing.description, body: editing.body });
      setEditing(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Page title="技能" subtitle="SKILL.md 渐进加载：只注入名称与描述；需要时读取正文"
      actions={<button className="primary" onClick={() => void open()}><Plus size={15} /> 新建技能</button>}>
      {error && !editing ? <p className="error-text">{error}</p> : null}
      <div className="skill-grid">
        {skills.map((skill) => (
          <article key={skill.name} className="skill-card">
            <header><span className="mode-avatar mode-daily"><Sparkles size={15} /></span><strong>{skill.name}</strong></header>
            <p>{skill.description || "没有描述；Agent 只能看到名称。"}</p>
            <small className="mono">{skill.path}</small>
            <footer>
              <button className="link-btn" disabled={loading} onClick={() => void open(skill)}><Eye size={12} /> 查看 / 编辑</button>
            </footer>
          </article>
        ))}
        {skills.length === 0 ? <EmptyHint title="还没有技能" detail="新建一个 SKILL.md，或把技能目录放进 workspace/skills 后保存设置。" /> : null}
      </div>
      {editing ? (
        <Modal title={editing.isNew ? "新建技能" : `编辑技能 · ${editing.name}`} subtitle="写清触发场景、步骤、输出格式和禁止事项" onClose={() => setEditing(null)} wide>
          <div className="form-grid">
            <label className="field field-span"><span>名称</span><input value={editing.name} disabled={!editing.isNew} onChange={(e) => setEditing({ ...editing, name: e.target.value })} placeholder="ssrf-testing" /></label>
            <label className="field field-span"><span>描述（Agent 用它判断何时加载）</span><input value={editing.description} onChange={(e) => setEditing({ ...editing, description: e.target.value })} placeholder="SSRF 漏洞识别、验证与绕过流程" /></label>
            <label className="field field-span"><span>正文（Markdown）</span><textarea className="mono skill-editor" rows={18} value={editing.body} onChange={(e) => setEditing({ ...editing, body: e.target.value })} /></label>
          </div>
          {error ? <p className="error-text">{error}</p> : null}
          <footer className="modal-actions">
            <button className="ghost-btn" onClick={() => setEditing(null)}>取消</button>
            <button className="primary" disabled={busy} onClick={() => void save()}>{busy ? "保存中…" : "保存技能"}</button>
          </footer>
        </Modal>
      ) : null}
    </Page>
  );
}

export function AgentsPage({ modes, onNewConversation }: { modes: ModeMeta[]; onNewConversation: (mode?: string) => void }) {
  return (
    <Page title="智能体" subtitle="五种模式与内置角色编制"
      actions={<button className="primary" onClick={() => onNewConversation()}><Plus size={15} /> 新建任务</button>}>
      <div className="mode-start-grid">
        {modes.map((mode) => {
          const Icon = modeIcon(mode.id);
          return (
            <button key={mode.id} className={`mode-start-card mode-${mode.id}`} onClick={() => onNewConversation(mode.id)}>
              <span className="mode-start-icon"><Icon size={19} /></span>
              <strong>{mode.name}</strong>
              <p>{mode.tagline}</p>
              <div className="chips">{mode.roles.map((role) => <span key={role} className="chip">{role}</span>)}</div>
              <footer><span>{mode.toolCount ?? 0} 工具</span><span className="mode-start-go">开始 <ArrowRight size={13} /></span></footer>
            </button>
          );
        })}
      </div>
    </Page>
  );
}

function EmptyHint({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="empty-hint">
      <div className="empty-icon"><Eye size={20} /></div>
      <strong>{title}</strong>
      <p>{detail}</p>
    </div>
  );
}

export function EditProjectDialog({
  project, onClose, onSave,
}: {
  project: Project;
  onClose: () => void;
  onSave: (body: Record<string, unknown>) => Promise<void>;
}) {
  const [name, setName] = useState(project.name);
  const [description, setDescription] = useState(project.description);
  const [authorization, setAuthorization] = useState(project.authorization);
  const [scope, setScope] = useState(project.scope);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit() {
    if (!name.trim()) {
      setError("项目名必填");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onSave({ name, description, authorization, scope });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }
  return (
    <Modal title="项目设置" subtitle="项目级授权与范围会作为默认值带入新任务" onClose={onClose}>
      <div className="form-grid">
        <label className="field field-span"><span>项目名</span><input value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="field field-span"><span>说明</span><textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
        <label className="field"><span>授权</span><input value={authorization} onChange={(e) => setAuthorization(e.target.value)} placeholder="谁授权、书面范围、测试窗口" /></label>
        <label className="field"><span>范围</span><input value={scope} onChange={(e) => setScope(e.target.value)} placeholder="允许触碰的域名 / IP / 路径" /></label>
      </div>
      {error ? <p className="error-text">{error}</p> : null}
      <div className="modal-actions"><button className="ghost-btn" onClick={onClose}>取消</button><button className="primary" disabled={busy} onClick={() => void submit()}>{busy ? "保存中…" : "保存项目"}</button></div>
    </Modal>
  );
}

export function LoginPage({ onLogin }: { onLogin: (password: string) => Promise<void> }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await onLogin(password);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }
  return (
    <div className="login-page">
      <form className="login-card" onSubmit={(event) => void submit(event)}>
        <div className="brand-mark"><Eye size={20} /></div>
        <h1>lingeyebot</h1>
        <p>个人账户登录</p>
        <input type="password" autoFocus value={password} onChange={(event) => setPassword(event.target.value)} placeholder="密码" />
        {error ? <p className="error-text">{error}</p> : null}
        <button className="primary" disabled={busy || !password} type="submit">{busy ? "登录中…" : "登录"}</button>
      </form>
    </div>
  );
}

export function NewProjectDialog({ onClose, onCreate }: {
  onClose: () => void; onCreate: (body: Record<string, unknown>) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [authorization, setAuthorization] = useState("");
  const [scope, setScope] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit() {
    if (!name.trim()) { setError("项目名必填"); return; }
    setBusy(true);
    try {
      await onCreate({ name, description, authorization, scope });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }
  return (
    <Modal title="新建项目" subtitle="项目共享授权、范围、资产、发现与黑板" onClose={onClose}>
      <div className="form-grid">
        <label className="field field-span"><span>项目名</span><input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如 Acme SRC / 某客户红队" /></label>
        <label className="field field-span"><span>说明</span><textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
        <label className="field"><span>授权</span><input value={authorization} onChange={(e) => setAuthorization(e.target.value)} /></label>
        <label className="field"><span>范围</span><input value={scope} onChange={(e) => setScope(e.target.value)} /></label>
      </div>
      {error ? <p className="error-text">{error}</p> : null}
      <div className="modal-actions"><button className="ghost-btn" onClick={onClose}>取消</button><button className="primary" disabled={busy} onClick={() => void submit()}>创建项目</button></div>
    </Modal>
  );
}

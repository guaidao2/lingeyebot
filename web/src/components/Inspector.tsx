import {
  Activity, Bug, Boxes, Copy, Download, FileSearch, FileText, GitBranch, Users, X,
} from "lucide-react";
import { useState } from "react";
import type {
  AuditEntry, Coverage, Evidence, FindingView, RunDetail,
  SnapshotView, Task,
} from "../types";
import { nodeKindLabel, severityTone, shortTime, statusLabels, statusTone, truncate } from "../lib/format";
import Graph from "./Graph";
import { Badge, Collapsible, Empty, Markdown } from "./ui";
import { TaskDetailModal } from "./pages";

export type InspectorTab = "graph" | "assets" | "findings" | "evidence" | "report" | "tasks" | "status";

const tabs: { id: InspectorTab; label: string; icon: typeof GitBranch }[] = [
  { id: "graph", label: "攻击链", icon: GitBranch },
  { id: "assets", label: "资产", icon: Boxes },
  { id: "findings", label: "漏洞", icon: Bug },
  { id: "evidence", label: "证据", icon: FileSearch },
  { id: "report", label: "报告", icon: FileText },
  { id: "tasks", label: "子代理", icon: Users },
  { id: "status", label: "状态", icon: Activity },
];

interface Props {
  detail: RunDetail;
  view: SnapshotView | null;
  findings: FindingView[];
  coverage: Coverage | null;
  evidence: Evidence[];
  report: string;
  tasks: Task[];
  audit: AuditEntry[];
  tab: InspectorTab;
  onTab: (tab: InspectorTab) => void;
  selectedNode: string | null;
  onSelectNode: (id: string | null) => void;
  onClose: () => void;
  onOpenConversation?: (runId: string) => void;
  onTaskCancel?: (taskId: string) => Promise<void>;
  taskStreams?: Record<string, string>;
}

export default function Inspector({
  detail,
  view,
  findings,
  coverage,
  evidence,
  report,
  tasks,
  audit,
  tab,
  onTab,
  selectedNode,
  onSelectNode,
  onClose,
  onOpenConversation,
  onTaskCancel,
  taskStreams,
}: Props) {
  const snapshot = view?.snapshot;
  const review = view?.review.items ?? [];
  return (
    <aside className="inspector">
      <div className="inspector-tabs">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button key={id} className={`itab ${tab === id ? "active" : ""}`} onClick={() => onTab(id)}>
            <Icon size={15} />
            <span>{label}</span>
          </button>
        ))}
        <button className="icon-btn inspector-close" onClick={onClose} title="关闭检查器" aria-label="关闭检查器">
          <X size={16} />
        </button>
      </div>

      <div className="inspector-body">
        {tab === "graph" ? (
          snapshot ? (
            <>
              <Graph snapshot={snapshot} selectedId={selectedNode} onSelect={onSelectNode} />
              <p className="inspector-hint">点击节点高亮；点右上角全屏后，点卡片查看完整详情。拖拽平移，滚轮缩放。</p>
            </>
          ) : (
            <Empty title="暂无链路数据" description="任务开始后会自动生成探索图。" />
          )
        ) : null}

        {tab === "assets" ? (
          coverage ? <AssetsPanel coverage={coverage} /> : <Empty title="暂无资产" />
        ) : null}

        {tab === "findings" ? <FindingsPanel findings={findings} /> : null}

        {tab === "evidence" ? <EvidencePanel evidence={evidence} /> : null}

        {tab === "report" ? <ReportPanel report={report} /> : null}

        {tab === "tasks" ? <TasksPanel tasks={tasks} onOpenConversation={onOpenConversation} onCancel={onTaskCancel} taskStreams={taskStreams} /> : null}

        {tab === "status" ? (
          <StatusPanel detail={detail} view={view} audit={audit} />
        ) : null}
      </div>
    </aside>
  );
}

function AssetsPanel({ coverage }: { coverage: Coverage }) {
  const [filter, setFilter] = useState("");
  if (coverage.total === 0) {
    return <Empty icon={<Boxes size={22} />} title="还没有资产" description="代理在任务中写入资产后，这里会出现覆盖与关联信息。" />;
  }
  const rows = coverage.assets.filter((a) => {
    const q = filter.trim().toLowerCase();
    return !q || a.value.toLowerCase().includes(q) || a.type.toLowerCase().includes(q);
  });
  const percent = coverage.total ? Math.round((coverage.tested / coverage.total) * 100) : 0;
  return (
    <div className="panel-stack">
      <div className="coverage-card">
        <div className="coverage-head">
          <span>资产覆盖</span>
          <strong>{percent}%</strong>
        </div>
        <div className="progress">
          <i style={{ width: `${percent}%` }} />
        </div>
        <div className="coverage-meta">
          <span>已测 {coverage.tested}</span>
          <span>未测 {coverage.total - coverage.tested}</span>
          <span>总计 {coverage.total}</span>
        </div>
      </div>
      <input className="search-input" placeholder="搜索资产…" value={filter} onChange={(e) => setFilter(e.target.value)} />
      <div className="asset-table">
        {rows.map((asset) => (
          <div key={asset.id} className={`asset-row ${asset.tested ? "tested" : ""}`}>
            <span className={`test-dot ${asset.tested ? "on" : ""}`} />
            <span className="asset-type">{asset.type}</span>
            <span className="asset-value" title={asset.value}>{asset.value}</span>
            {asset.tested ? <Badge tone="good">已测</Badge> : <Badge tone="muted">未测</Badge>}
          </div>
        ))}
        {rows.length === 0 ? <p className="muted">没有匹配的资产。</p> : null}
      </div>
    </div>
  );
}

function FindingsPanel({ findings }: { findings: FindingView[] }) {
  if (!findings.length) {
    return <Empty icon={<Bug size={22} />} title="还没有确认漏洞" description="只有真实触发并带证据的问题才会出现在这里。" />;
  }
  return (
    <div className="panel-stack">
      {findings.map((item) => {
        const f = item.finding;
        const tone = severityTone[f.severity] ?? "info";
        return (
          <article key={item.id} className={`finding-card sev-${tone}`}>
            <header>
              <div className="finding-title">
                <Badge tone={tone}>{f.severity}</Badge>
                <strong>{f.title}</strong>
              </div>
              <span className="muted mono">{item.id}</span>
            </header>
            {f.vulnclass ? <p className="finding-class">{f.vulnclass}</p> : null}
            {f.summary ? <p>{f.summary}</p> : null}
            <Collapsible title="复现步骤" defaultOpen>
              <ol className="steps">
                {(f.reproducibleSteps ?? []).map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </Collapsible>
            {f.impact ? <p className="muted">影响：{f.impact}</p> : null}
            {f.trustBoundary ? <p className="muted">信任边界：{f.trustBoundary}</p> : null}
            {item.evidence.length ? (
              <Collapsible title={`证据 ${item.evidence.length}`}>
                {item.evidence.map((e) => (
                  <div key={e.id} className="evidence-mini">
                    <span className="mono">{e.kind}</span> {e.label}
                    {e.channel ? <span className="muted"> · 信道 {e.channel}</span> : null}
                  </div>
                ))}
              </Collapsible>
            ) : null}
          </article>
        );
      })}
    </div>
  );
}

function EvidencePanel({ evidence }: { evidence: Evidence[] }) {
  if (!evidence.length) {
    return <Empty icon={<FileSearch size={22} />} title="还没有证据" description="事实与漏洞都必须落在证据上。" />;
  }
  return (
    <div className="panel-stack">
      {evidence.map((e) => (
        <article key={e.id} className="evidence-card">
          <header>
            <span className="evidence-kind">{e.kind}</span>
            <strong>{e.label}</strong>
          </header>
          <div className="evidence-meta">
            {e.channel ? <span>信道 {e.channel}</span> : null}
            <span>{e.size} B</span>
            <span className="mono">{e.sha256.slice(0, 12)}</span>
            <span>{shortTime(e.createdAt)}</span>
          </div>
          {e.layers?.length ? (
            <div className="chips">
              {e.layers.map((layer, index) => (
                <span key={index} className="chip">{layer}</span>
              ))}
            </div>
          ) : null}
          {e.preview ? <pre className="code-block evidence-preview">{truncate(e.preview, 1200)}</pre> : null}
        </article>
      ))}
    </div>
  );
}

function ReportPanel({ report }: { report: string }) {
  function copy() {
    void navigator.clipboard.writeText(report).catch(() => undefined);
  }
  function download() {
    const blob = new Blob([report], { type: "text/markdown;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "lingeyebot-report.md";
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <div className="panel-stack">
      <div className="report-actions">
        <button className="ghost-btn" onClick={copy}><Copy size={14} /> 复制</button>
        <button className="ghost-btn" onClick={download}><Download size={14} /> 下载</button>
      </div>
      <div className="report-body">
        <Markdown content={report} />
      </div>
    </div>
  );
}

function TasksPanel({ tasks, onOpenConversation, onCancel, taskStreams }: {
  tasks: Task[]; onOpenConversation?: (runId: string) => void; onCancel?: (taskId: string) => Promise<void>;
  taskStreams?: Record<string, string>;
}) {
  const [selected, setSelected] = useState<Task | null>(null);
  if (!tasks.length) {
    return <Empty icon={<Users size={22} />} title="还没有子代理任务" description="主代理可以按角色派发情报、枚举、建模、前/后渗透与隐蔽任务。" />;
  }
  return (
    <div className="panel-stack">
      {tasks.map((task) => (
        <article key={task.id} className="task-card">
          <header>
            <span className={`role-chip role-${task.role}`}>{task.role}</span>
            <Badge tone={statusTone(task.status)}>{statusLabels[task.status] ?? task.status}</Badge>
            <button className="link-btn task-detail-btn" onClick={() => setSelected(task)}>详情</button>
          </header>
          <p className="task-title">{task.title}</p>
          <div className="task-meta">
            <span>{task.id}</span>
            <span>{shortTime(task.createdAt)}</span>
            {task.inbox.length ? <span>指令 {task.inbox.length}</span> : null}
          </div>
          {task.result ? <pre className="code-block task-result">{truncate(task.result, 1200)}</pre> : null}
          {task.error ? <p className="error-text">{task.error}</p> : null}
        </article>
      ))}
      {selected ? <TaskDetailModal task={selected} onClose={() => setSelected(null)} onOpenConversation={onOpenConversation} onCancel={onCancel} liveText={taskStreams?.[selected.id]} /> : null}
    </div>
  );
}

function StatusPanel({ detail, view, audit }: { detail: RunDetail; view: SnapshotView | null; audit: AuditEntry[] }) {
  const snapshot = view?.snapshot;
  const review = view?.review.items ?? [];
  return (
    <div className="panel-stack">
      <section className="status-section">
        <h4>目标</h4>
        {detail.run.goalMode === false ? <p className="muted">目标模式未启用；本任务只对话，不自动继续。</p> : null}
        {detail.goals.map((goal) => (
          <div key={goal.id} className="goal-row">
            <Badge tone={statusTone(goal.status)}>{statusLabels[goal.status] ?? goal.status}</Badge>
            <div>
              <p>{goal.text}</p>
              {goal.acceptance ? <small>验收：{goal.acceptance}</small> : null}
            </div>
          </div>
        ))}
        {detail.goals.length === 0 ? <p className="muted">没有目标。</p> : null}
      </section>

      <section className="status-section">
        <h4>约束</h4>
        {snapshot?.constraints.map((c) => (
          <div key={c.id} className="constraint-row">
            <Badge tone={c.kind === "deny" ? "bad" : "good"}>{c.kind}</Badge>
            <span>{c.text}</span>
          </div>
        ))}
        {(snapshot?.constraints.length ?? 0) === 0 ? <p className="muted">没有约束。</p> : null}
      </section>

      <section className="status-section">
        <h4>复核清单</h4>
        {review.map((item, index) => (
          <div key={`${item.kind}-${item.ref}-${index}`} className="review-row">
            <Badge tone={item.status === "satisfied" ? "good" : item.status === "blocked" ? "bad" : "warn"}>
              {item.status}
            </Badge>
            <span>{item.text}</span>
          </div>
        ))}
        {review.length === 0 ? <p className="muted">当前没有未完成项。</p> : null}
      </section>

      <section className="status-section">
        <h4>计数</h4>
        <div className="count-grid">
          {Object.entries(detail.counts).map(([kind, count]) => (
            <div key={kind} className="count-cell">
              <strong>{count}</strong>
              <span>{nodeKindLabel(kind)}</span>
            </div>
          ))}
          <div className="count-cell">
            <strong>{detail.coverage.tested}/{detail.coverage.total}</strong>
            <span>覆盖</span>
          </div>
        </div>
      </section>

      {snapshot?.flows.length ? (
        <section className="status-section">
          <h4>数据流 / 拼接点</h4>
          {snapshot.flows.map((flow) => (
            <div key={flow.id} className="flow-row">
              <span>{flow.source}</span>
              <span className="muted">→ {flow.transform || "（无变换）"} →</span>
              <strong>{flow.sink}</strong>
            </div>
          ))}
        </section>
      ) : null}

      <section className="status-section">
        <h4>审计</h4>
        <div className="audit-list">
          {audit.slice(0, 30).map((entry) => (
            <div key={entry.id} className="audit-row">
              <span className={`audit-action ${entry.action}`}>{entry.action}</span>
              <span className="audit-tool">{entry.tool}</span>
              <span className="audit-command" title={entry.command}>{truncate(entry.command || entry.reason, 70)}</span>
              <time>{shortTime(entry.createdAt)}</time>
            </div>
          ))}
          {audit.length === 0 ? <p className="muted">还没有工具调用审计。</p> : null}
        </div>
      </section>
    </div>
  );
}



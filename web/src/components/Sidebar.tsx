import { Eye, PanelLeftClose, Plus, Settings } from "lucide-react";
import type { RunSummary } from "../types";
import { modeLabels, statusTone, truncate } from "../lib/format";

interface Props {
  runs: RunSummary[];
  activeId: string | null;
  configured: boolean;
  version?: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  onSettings: () => void;
  onCollapse: () => void;
}

export default function Sidebar({
  runs,
  activeId,
  configured,
  version,
  onSelect,
  onNew,
  onSettings,
  onCollapse,
}: Props) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark">
          <Eye size={18} />
        </div>
        <div className="brand-text">
          <h1>lingeyebot</h1>
          <span>security agent</span>
        </div>
        <button className="icon-btn" onClick={onCollapse} title="收起侧栏">
          <PanelLeftClose size={16} />
        </button>
      </div>

      <button className="primary new-run" onClick={onNew}>
        <Plus size={16} /> 新建任务
      </button>

      <div className="sidebar-label">
        任务 <span>{runs.length}</span>
      </div>

      <div className="run-list">
        {runs.map((run) => (
          <button
            key={run.id}
            className={`run-item ${activeId === run.id ? "active" : ""}`}
            onClick={() => onSelect(run.id)}
          >
            <div className="run-top">
              <span className={`mode-dot mode-${run.mode}`} />
              <span className="run-target">{run.target || "未命名任务"}</span>
              <span className={`status-dot ${statusTone(run.status)}`} title={run.status} />
            </div>
            <div className="run-objective">{truncate(run.objective, 64)}</div>
            <div className="run-meta">
              <span className={`mode-chip mode-${run.mode}`}>{modeLabels[run.mode] ?? run.mode}</span>
              <span>{run.findings} 漏洞</span>
              <span>{run.openGoals} 待达成</span>
            </div>
          </button>
        ))}
        {runs.length === 0 ? <p className="sidebar-empty">还没有任务，点击上方新建。</p> : null}
      </div>

      <div className="sidebar-foot">
        <div className={`provider-state ${configured ? "ok" : "warn"}`}>
          <span className="dot" />
          {configured ? "LLM 已配置" : "未配置 LLM"}
        </div>
        <button className="ghost-btn" onClick={onSettings}>
          <Settings size={15} /> 设置
        </button>
        {version ? <span className="version">v{version}</span> : null}
      </div>
    </aside>
  );
}

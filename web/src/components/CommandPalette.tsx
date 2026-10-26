import { Boxes, Bug, CornerDownLeft, FolderKanban, LayoutDashboard, MessagesSquare, Search } from "lucide-react";
import { useMemo, useState } from "react";
import type { Asset, ProjectFinding, ProjectSummary, RunSummary } from "../types";
import type { View } from "./pages";

export type PaletteKind = "view" | "project" | "conversation" | "asset" | "finding";

interface PaletteItem {
  key: string;
  group: string;
  label: string;
  hint: string;
  kind: PaletteKind;
  id?: string;
  icon: typeof LayoutDashboard;
}

const pages: { id: View; label: string }[] = [
  { id: "dashboard", label: "仪表盘" },
  { id: "conversations", label: "任务" },
  { id: "projects", label: "项目管理" },
  { id: "assets", label: "资产管理" },
  { id: "findings", label: "漏洞管理" },
  { id: "tasks", label: "子代理" },
  { id: "approvals", label: "人机协同" },
  { id: "mcp", label: "MCP" },
  { id: "skills", label: "技能" },
  { id: "agents", label: "智能体" },
];

export default function CommandPalette({
  projects, runs, assets, findings, onClose, onChoose,
}: {
  projects: ProjectSummary[];
  runs: RunSummary[];
  assets: Asset[];
  findings: ProjectFinding[];
  onClose: () => void;
  onChoose: (kind: PaletteKind, id?: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);

  const items = useMemo<PaletteItem[]>(() => {
    const list: PaletteItem[] = pages.map((page) => ({
      key: `view:${page.id}`, group: "页面", label: page.label, hint: "跳转", kind: "view", id: page.id, icon: LayoutDashboard,
    }));
    projects.forEach((project) => list.push({
      key: `project:${project.id}`, group: "项目", label: project.name,
      hint: `${project.conversations} 任务 · ${project.findings} 漏洞`, kind: "project", id: project.id, icon: FolderKanban,
    }));
    runs.slice(0, 40).forEach((run) => list.push({
      key: `conversation:${run.id}`, group: "任务", label: run.title || run.objective,
      hint: `${run.target} · ${run.mode}`, kind: "conversation", id: run.id, icon: MessagesSquare,
    }));
    assets.slice(0, 40).forEach((asset) => list.push({
      key: `asset:${asset.id}`, group: "资产", label: asset.value,
      hint: `${asset.type}${asset.company ? " · " + asset.company : ""}`, kind: "asset", id: asset.id, icon: Boxes,
    }));
    findings.slice(0, 40).forEach((finding) => list.push({
      key: `finding:${finding.id}`, group: "漏洞", label: finding.payload?.title ?? "漏洞",
      hint: `${finding.payload?.severity ?? "info"} · ${finding.projectName}`, kind: "finding", id: finding.id, icon: Bug,
    }));
    return list;
  }, [projects, runs, assets, findings]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items.slice(0, 30);
    return items.filter((item) => `${item.label} ${item.hint} ${item.group}`.toLowerCase().includes(needle)).slice(0, 30);
  }, [items, query]);

  const grouped = useMemo(() => {
    const map = new Map<string, PaletteItem[]>();
    filtered.forEach((item) => {
      const list = map.get(item.group) ?? [];
      list.push(item);
      map.set(item.group, list);
    });
    return Array.from(map.entries());
  }, [filtered]);

  const flat = grouped.flatMap(([, list]) => list);
  const current = Math.min(active, Math.max(0, flat.length - 1));

  function choose(item: PaletteItem) {
    onChoose(item.kind, item.id);
    onClose();
  }

  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div className="palette" onClick={(event) => event.stopPropagation()}>
        <div className="palette-input">
          <Search size={17} />
          <input
            autoFocus
            value={query}
            placeholder="搜索项目、任务、资产、漏洞…"
            onChange={(event) => { setQuery(event.target.value); setActive(0); }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing) return;
              if (event.key === "Escape") onClose();
              if (event.key === "ArrowDown") { event.preventDefault(); setActive((value) => Math.min(value + 1, flat.length - 1)); }
              if (event.key === "ArrowUp") { event.preventDefault(); setActive((value) => Math.max(0, value - 1)); }
              if (event.key === "Enter" && flat[current]) { event.preventDefault(); choose(flat[current]); }
            }}
          />
          <kbd>esc</kbd>
        </div>
        <div className="palette-results">
          {grouped.map(([group, list]) => (
            <div key={group} className="palette-group">
              <span className="palette-group-label">{group}</span>
              {list.map((item) => {
                const index = flat.indexOf(item);
                return (
                  <button
                    key={item.key}
                    className={`palette-item ${index === current ? "active" : ""}`}
                    onMouseEnter={() => setActive(index)}
                    onClick={() => choose(item)}
                  >
                    <item.icon size={16} />
                    <span className="palette-main"><strong>{item.label}</strong><small>{item.hint}</small></span>
                    {index === current ? <CornerDownLeft size={13} /> : null}
                  </button>
                );
              })}
            </div>
          ))}
          {filtered.length === 0 ? <p className="palette-empty">没有匹配结果。</p> : null}
        </div>
        <div className="palette-foot"><span>↑↓ 选择</span><span>Enter 打开</span><span>Esc 关闭</span></div>
      </div>
    </div>
  );
}

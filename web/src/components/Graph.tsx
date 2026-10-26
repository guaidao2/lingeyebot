import { Maximize2, Minimize2, Minus, Plus, RotateCcw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { Snapshot } from "../types";
import { nodeKindLabel, truncate } from "../lib/format";

const KIND_COLORS: Record<string, string> = {
  goal: "#34d399",
  intent: "#60a5fa",
  fact: "#fbbf24",
  finding: "#fb7185",
  hint: "#a78bfa",
  attack_path: "#fb923c",
  asset: "#94a3b8",
  decision: "#60a5fa",
  credential: "#a78bfa",
  note: "#94a3b8",
  contact: "#94a3b8",
};

const DEFAULT_LEGEND = ["goal", "intent", "fact", "finding", "hint", "attack_path"];

const COLUMN_OF: Record<string, string> = {
  goal: "left",
  hint: "left",
  intent: "intent",
  fact: "fact",
  finding: "finding",
  attack_path: "path",
  asset: "asset",
  decision: "intent",
  credential: "left",
  note: "left",
  contact: "left",
};

const COLUMN_STEP = 360;

const REL_COLORS: Record<string, string> = {
  spawns: "#34d399",
  derived_from: "#818cf8",
  yields: "#fbbf24",
  proves: "#fb7185",
  covers: "#22d3ee",
  anchors: "#475569",
  leads_to: "#f97316",
  pivots_to: "#f97316",
  exploits: "#fb7185",
  constrains: "#818cf8",
  enables: "#f59e0b",
  relates: "#64748b",
};

interface VNode {
  id: string;
  kind: string;
  label: string;
  sub: string;
  x: number;
  y: number;
  w: number;
  h: number;
  state: string;
}

function layout(snapshot: Snapshot): VNode[] {
  const out: VNode[] = [];
  const counters: Record<string, number> = {};
  const present = new Set(snapshot.nodes.map((node) => node.kind));
  const columns: string[] = [];
  if (snapshot.goals.length > 0 || ["hint", "credential", "note", "contact"].some((kind) => present.has(kind))) {
    columns.push("left");
  }
  if (present.has("intent") || present.has("decision")) columns.push("intent");
  if (present.has("fact")) columns.push("fact");
  if (present.has("finding")) columns.push("finding");
  if (present.has("attack_path")) columns.push("path");
  if (!columns.length) columns.push("left");
  const columnX = new Map(columns.map((column, index) => [column, 40 + index * COLUMN_STEP]));
  const add = (kind: string, id: string, label: string, sub: string, state: string) => {
    const column = COLUMN_OF[kind] ?? "left";
    const index = counters[column] ?? 0;
    counters[column] = index + 1;
    out.push({
      id,
      kind,
      label: truncate(label, 150),
      sub,
      x: columnX.get(column) ?? 40,
      y: 80 + index * 160,
      w: 280,
      h: 92,
      state,
    });
  };

  for (const goal of snapshot.goals) {
    add("goal", goal.id, goal.text, `目标 · ${goal.status}`, goal.status);
  }
  const sorted = [...snapshot.nodes].sort((a, b) => {
    const order = ["decision", "hint", "intent", "fact", "credential", "note", "contact", "finding", "attack_path"];
    return order.indexOf(a.kind) - order.indexOf(b.kind);
  });
  for (const node of sorted) {
    const payload = (node.payload ?? {}) as Record<string, any>;
    switch (node.kind) {
      case "intent":
        add("intent", node.id, payload.title ?? "意图", `意图 · ${node.state}${node.priority ? ` · P${node.priority}` : ""}`, node.state);
        break;
      case "fact":
        add("fact", node.id, payload.detail ?? "事实", `事实 · ${payload.basis ?? "observed"}`, node.state);
        break;
      case "finding":
        add("finding", node.id, payload.title ?? "漏洞", `${payload.severity ?? "info"} · ${node.state}`, node.state);
        break;
      case "hint":
        add("hint", node.id, payload.text ?? "提示", `提示 · ${node.state}`, node.state);
        break;
      case "attack_path":
        add("attack_path", node.id, payload.title ?? "攻击路径", `路径 · ${payload.status ?? node.state}`, node.state);
        break;
      case "decision":
        add("decision", node.id, payload.title ?? "决策", `决策 · ${node.state}`, node.state);
        break;
      case "credential":
        add("credential", node.id, payload.title ?? "凭据", `凭据 · ${node.state}`, node.state);
        break;
      case "note":
        add("note", node.id, payload.title ?? "备注", `备注 · ${node.state}`, node.state);
        break;
      case "contact":
        add("contact", node.id, payload.title ?? "联系人", `联系人 · ${node.state}`, node.state);
        break;
      default:
        break;
    }
  }
  return out;
}

// Column positions are only a seed. The visible graph is force-directed:
// edges pull related cards together, cards repel each other, and a weak
// gravity keeps the whole graph from drifting. This avoids the rigid
// left-to-right column order and lets the graph expand with its topology.
function forceLayout(snapshot: Snapshot): VNode[] {
  const nodes = layout(snapshot);
  if (nodes.length <= 1) return nodes;
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const links = snapshot.edges
    .filter((edge) => byId.has(edge.srcId) && byId.has(edge.dstId))
    .map((edge) => [byId.get(edge.srcId)!, byId.get(edge.dstId)!] as const);
  const center = (node: VNode) => ({ x: node.x + node.w / 2, y: node.y + node.h / 2 });
  const radius = Math.max(320, nodes.length * 48);
  nodes.forEach((node, index) => {
    const angle = (Math.PI * 2 * index) / nodes.length;
    node.x = Math.cos(angle) * radius;
    node.y = Math.sin(angle) * radius;
  });

  const iterations = 220;
  for (let step = 0; step < iterations; step += 1) {
    const fx = new Map<string, number>();
    const fy = new Map<string, number>();
    const add = (id: string, x: number, y: number) => {
      fx.set(id, (fx.get(id) ?? 0) + x);
      fy.set(id, (fy.get(id) ?? 0) + y);
    };
    for (let i = 0; i < nodes.length; i += 1) {
      const a = nodes[i];
      const ac = center(a);
      for (let j = i + 1; j < nodes.length; j += 1) {
        const b = nodes[j];
        const bc = center(b);
        let dx = bc.x - ac.x;
        let dy = bc.y - ac.y;
        let dist = Math.hypot(dx, dy);
        if (dist < 1) {
          dx = (i + 1) * 0.7;
          dy = (j + 1) * 0.7;
          dist = Math.hypot(dx, dy);
        }
        const minDist = 380;
        const push = (minDist * minDist) / (dist * dist) * 0.9;
        add(a.id, (-dx / dist) * push, (-dy / dist) * push);
        add(b.id, (dx / dist) * push, (dy / dist) * push);
      }
    }
    for (const [a, b] of links) {
      const ac = center(a);
      const bc = center(b);
      const dx = bc.x - ac.x;
      const dy = bc.y - ac.y;
      const dist = Math.hypot(dx, dy) || 1;
      const target = 460;
      const pull = (dist - target) * 0.006;
      add(a.id, (dx / dist) * pull, (dy / dist) * pull);
      add(b.id, (-dx / dist) * pull, (-dy / dist) * pull);
    }
    const avgX = nodes.reduce((sum, node) => sum + center(node).x, 0) / nodes.length;
    const avgY = nodes.reduce((sum, node) => sum + center(node).y, 0) / nodes.length;
    for (const node of nodes) {
      const c = center(node);
      add(node.id, (avgX - c.x) * 0.0015, (avgY - c.y) * 0.0015);
    }
    for (const node of nodes) {
      node.x += Math.max(-8, Math.min(8, fx.get(node.id) ?? 0));
      node.y += Math.max(-8, Math.min(8, fy.get(node.id) ?? 0));
    }
  }
  const minX = Math.min(...nodes.map((node) => node.x));
  const minY = Math.min(...nodes.map((node) => node.y));
  for (const node of nodes) {
    node.x = node.x - minX + 40;
    node.y = node.y - minY + 40;
  }
  return nodes;
}

export default function Graph({
  snapshot,
  selectedId,
  onSelect,
  legendKinds,
}: {
  snapshot: Snapshot;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  legendKinds?: string[];
}) {
  const [view, setView] = useState({ x: 0, y: 0, k: 1 });
  const [maximized, setMaximized] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; vx: number; vy: number } | null>(null);
  const dragMoved = useRef(false);
  const fittedSignature = useRef("");
  const resizeTimer = useRef<number | undefined>(undefined);
  const fitRef = useRef<() => void>(() => {});
  const userAdjusted = useRef(false);

  const nodes = useMemo(() => forceLayout(snapshot), [snapshot]);
  const signature = useMemo(() => nodes.map((n) => n.id).join(","), [nodes]);
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const edges = useMemo(
    () => snapshot.edges.filter((e) => byId.has(e.srcId) && byId.has(e.dstId)),
    [snapshot.edges, byId],
  );
  const anchorEdges = useMemo(
    () => snapshot.anchors.filter((a) => byId.has(a.nodeId) && byId.has(a.assetId)),
    [snapshot.anchors, byId],
  );
  const anchorOffsets = useMemo(() => {
    const outgoing = new Map<string, string[]>();
    const incoming = new Map<string, string[]>();
    for (const anchor of anchorEdges) {
      const key = `${anchor.nodeId}-${anchor.assetId}`;
      const out = outgoing.get(anchor.nodeId) ?? [];
      out.push(key);
      outgoing.set(anchor.nodeId, out);
      const inc = incoming.get(anchor.assetId) ?? [];
      inc.push(key);
      incoming.set(anchor.assetId, inc);
    }
    const ratio = (key: string, list: string[]) => {
      const index = list.indexOf(key);
      return list.length <= 1 ? 0.5 : (index + 1) / (list.length + 1);
    };
    const result = new Map<string, { from: number; to: number }>();
    for (const anchor of anchorEdges) {
      const key = `${anchor.nodeId}-${anchor.assetId}`;
      result.set(key, {
        from: ratio(key, outgoing.get(anchor.nodeId) ?? []),
        to: ratio(key, incoming.get(anchor.assetId) ?? []),
      });
    }
    return result;
  }, [anchorEdges]);
  const linked = edges.length > 0 || anchorEdges.length > 0;
  const edgeOffsets = useMemo(() => {
    const outgoing = new Map<string, string[]>();
    const incoming = new Map<string, string[]>();
    for (const edge of edges) {
      const out = outgoing.get(edge.srcId) ?? [];
      out.push(edge.id);
      outgoing.set(edge.srcId, out);
      const inc = incoming.get(edge.dstId) ?? [];
      inc.push(edge.id);
      incoming.set(edge.dstId, inc);
    }
    const ratio = (id: string, list: string[]) => {
      const index = list.indexOf(id);
      return list.length <= 1 ? 0.5 : (index + 1) / (list.length + 1);
    };
    const result = new Map<string, { from: number; to: number }>();
    for (const edge of edges) {
      result.set(edge.id, {
        from: ratio(edge.id, outgoing.get(edge.srcId) ?? []),
        to: ratio(edge.id, incoming.get(edge.dstId) ?? []),
      });
    }
    return result;
  }, [edges]);
  const selectedNode = useMemo(
    () => snapshot.nodes.find((node) => node.id === selectedId) ?? null,
    [snapshot.nodes, selectedId],
  );
  const selectedGoal = useMemo(
    () => snapshot.goals.find((goal) => goal.id === selectedId) ?? null,
    [snapshot.goals, selectedId],
  );
  const selectedAsset = useMemo(
    () => snapshot.assets.find((asset) => asset.id === selectedId) ?? null,
    [snapshot.assets, selectedId],
  );
  const selectedCard = useMemo(
    () => nodes.find((node) => node.id === selectedId) ?? null,
    [nodes, selectedId],
  );
  const selectedEdges = useMemo(
    () => selectedId
      ? snapshot.edges.filter((edge) => edge.srcId === selectedId || edge.dstId === selectedId)
      : [],
    [snapshot.edges, selectedId],
  );
  const selectedPayload = selectedNode?.payload
    ?? (selectedGoal ? { text: selectedGoal.text, acceptance: selectedGoal.acceptance, status: selectedGoal.status } : null)
    ?? (selectedAsset ? { type: selectedAsset.type, value: selectedAsset.value, company: selectedAsset.company, meta: selectedAsset.meta } : null);

  useEffect(() => {
    if (nodes.length === 0 || fittedSignature.current === signature) return;
    fittedSignature.current = signature;
    userAdjusted.current = false;
    const timer = window.setTimeout(() => fitRef.current(), 30);
    return () => window.clearTimeout(timer);
  }, [signature]);

  useEffect(() => {
    if (!maximized || nodes.length === 0) return;
    userAdjusted.current = false;
    const timer = window.setTimeout(() => fitRef.current(), 30);
    return () => window.clearTimeout(timer);
  }, [maximized, signature]);

  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const observer = new ResizeObserver(() => {
      if (userAdjusted.current) return;
      window.clearTimeout(resizeTimer.current);
      resizeTimer.current = window.setTimeout(() => fitRef.current(), 80);
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
      window.clearTimeout(resizeTimer.current);
    };
  }, [signature, maximized]);

  useEffect(() => {
    if (!maximized) {
      userAdjusted.current = false;
      const timer = window.setTimeout(() => fitRef.current(), 30);
      return () => window.clearTimeout(timer);
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMaximized(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [maximized]);

  function zoom(factor: number, centerX?: number, centerY?: number) {
    userAdjusted.current = true;
    const rect = container.current?.getBoundingClientRect();
    const mx = centerX ?? (rect ? rect.width / 2 : 400);
    const my = centerY ?? (rect ? rect.height / 2 : 300);
    setView((v) => {
      const k = Math.min(2.2, Math.max(0.3, v.k * factor));
      const scale = k / v.k;
      return { k, x: mx - (mx - v.x) * scale, y: my - (my - v.y) * scale };
    });
  }

  function fit() {
    if (!nodes.length) return;
    userAdjusted.current = false;
    const rect = container.current?.getBoundingClientRect();
    const width = rect?.width ?? 900;
    const height = rect?.height ?? 500;
    if (width < 40 || height < 40) return;
    const minX = Math.min(...nodes.map((n) => n.x));
    const maxX = Math.max(...nodes.map((n) => n.x + n.w));
    const minY = Math.min(...nodes.map((n) => n.y));
    const maxY = Math.max(...nodes.map((n) => n.y + n.h));
    const contentW = Math.max(1, maxX - minX);
    const contentH = Math.max(1, maxY - minY);
    const k = Math.min(1.2, Math.max(0.6, Math.min((width - 48) / contentW, (height - 48) / contentH)));
    setView({
      k,
      x: (width - contentW * k) / 2 - minX * k,
      y: (height - contentH * k) / 2 - minY * k,
    });
  }
  fitRef.current = fit;

  return (
    <div className={`graph-canvas ${maximized ? "maximized" : ""}`} ref={container}>
      <svg
        className="graph-svg"
        onWheel={(e) => {
          e.preventDefault();
          const rect = container.current?.getBoundingClientRect();
          zoom(e.deltaY < 0 ? 1.08 : 0.93, e.clientX - (rect?.left ?? 0), e.clientY - (rect?.top ?? 0));
        }}
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          if ((e.target as Element).closest(".gnode")) return;
          dragMoved.current = false;
          drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y };
          (e.currentTarget as SVGElement).setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          if (!drag.current) return;
          if (Math.abs(e.clientX - drag.current.x) > 3 || Math.abs(e.clientY - drag.current.y) > 3) {
            dragMoved.current = true;
            userAdjusted.current = true;
          }
          const start = drag.current;
          setView((v) => ({ ...v, x: start.vx + e.clientX - start.x, y: start.vy + e.clientY - start.y }));
        }}
        onPointerUp={() => {
          drag.current = null;
        }}
        onClick={(e) => {
          if (!dragMoved.current && e.target === e.currentTarget) onSelect(null);
          dragMoved.current = false;
        }}
      >
        <defs>
          {Object.entries(REL_COLORS).map(([rel, color]) => (
            <marker
              key={rel}
              id={`arrow-${rel}`}
              markerWidth="8"
              markerHeight="8"
              refX="8"
              refY="4"
              markerUnits="userSpaceOnUse"
              orient="auto"
            >
              <path d="M0,0 L8,4 L0,8 z" fill={color} />
            </marker>
          ))}
        </defs>
        <g transform={`translate(${view.x},${view.y}) scale(${view.k})`}>
          {anchorEdges.map((anchor) => {
            const from = byId.get(anchor.nodeId);
            const to = byId.get(anchor.assetId);
            if (!from || !to) return null;
            const offset = anchorOffsets.get(`${anchor.nodeId}-${anchor.assetId}`) ?? { from: 0.5, to: 0.5 };
            const forward = to.x >= from.x;
            const gap = 12;
            const x1 = forward ? from.x + from.w : from.x;
            const x2 = forward ? to.x - gap : to.x + to.w + gap;
            const y1 = from.y + 12 + (from.h - 24) * offset.from;
            const y2 = to.y + 12 + (to.h - 24) * offset.to;
            const mid = (x1 + x2) / 2;
            return (
              <path
                key={`${anchor.nodeId}-${anchor.assetId}`}
                d={`M${x1},${y1} C${mid},${y1} ${mid},${y2} ${x2},${y2}`}
                className="graph-edge anchor"
                markerEnd="url(#arrow-covers)"
                vectorEffect="non-scaling-stroke"
              />
            );
          })}
          {edges.map((edge) => {
            const from = byId.get(edge.srcId)!;
            const to = byId.get(edge.dstId)!;
            const offset = edgeOffsets.get(edge.id) ?? { from: 0.5, to: 0.5 };
            const forward = to.x >= from.x;
            const gap = 10;
            const verticalGap = 6;
            const portY = (node: typeof from, ratio: number) =>
              node.y + 12 + (node.h - 24) * ratio;
            let d: string;
            if (Math.abs(to.x - from.x) < 1) {
              const upward = to.y < from.y;
              const x1 = from.x + from.w / 2;
              const x2 = to.x + to.w / 2;
              const y1 = upward ? from.y : from.y + from.h;
              const y2 = upward ? to.y + to.h + verticalGap : to.y - verticalGap;
              const bend = upward ? -24 : 24;
              d = `M${x1},${y1} C${x1},${y1 + bend} ${x2},${y2 - bend} ${x2},${y2}`;
            } else {
              const x1 = forward ? from.x + from.w : from.x;
              const x2 = forward ? to.x - gap : to.x + to.w + gap;
              const y1 = portY(from, offset.from);
              const y2 = portY(to, offset.to);
              const dx = Math.max(60, Math.abs(x2 - x1) / 2);
              d = forward
                ? `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`
                : `M${x1},${y1} C${x1 - dx},${y1} ${x2 + dx},${y2} ${x2},${y2}`;
            }
            const color = REL_COLORS[edge.rel] ?? "#64748b";
            return (
              <path
                key={edge.id}
                d={d}
                className="graph-edge"
                stroke={color}
                strokeWidth={1.6}
                markerEnd={`url(#arrow-${edge.rel in REL_COLORS ? edge.rel : "spawns"})`}
                vectorEffect="non-scaling-stroke"
              />
            );
          })}
          {nodes.map((node) => {
            const color = KIND_COLORS[node.kind] ?? "#94a3b8";
            return (
              <foreignObject key={node.id} x={node.x} y={node.y} width={node.w} height={node.h}>
                <div
                  className={`gnode kind-${node.kind} ${selectedId === node.id ? "selected" : ""} state-${node.state}`}
                  style={{ borderColor: selectedId === node.id ? color : undefined }}
                  onClick={(e) => {
                    e.stopPropagation();
                    onSelect(node.id);
                  }}
                >
                  <span className="gnode-kind" style={{ color }}>
                    {nodeKindLabel(node.kind)}
                  </span>
                  <span className="gnode-label">{node.label}</span>
                  <span className="gnode-sub">{node.sub}</span>
                </div>
              </foreignObject>
            );
          })}
        </g>
      </svg>

      <div className="graph-controls">
        <button className="icon-btn" onClick={() => zoom(1.15)} title="放大">
          <Plus size={15} />
        </button>
        <button className="icon-btn" onClick={() => zoom(0.87)} title="缩小">
          <Minus size={15} />
        </button>
        <button className="icon-btn" onClick={fit} title="适应画布">
          <RotateCcw size={15} />
        </button>
        <button
          className="icon-btn"
          onClick={() => setMaximized((value) => !value)}
          title={maximized ? "退出全屏" : "全屏查看"}
        >
          {maximized ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
      </div>

      <div className="graph-legend">
        {(legendKinds ?? DEFAULT_LEGEND).map((kind) => (
          <span key={kind} className="legend-item">
            <i style={{ background: KIND_COLORS[kind] }} />
            {nodeKindLabel(kind)}
          </span>
        ))}
      </div>

      {selectedCard ? (
        <div className="graph-detail-overlay">
          <header>
            <span className="gnode-kind">{nodeKindLabel(selectedCard.kind)}</span>
            <button className="icon-btn" onClick={() => onSelect(null)} aria-label="关闭节点详情">×</button>
          </header>
          <GraphDetail payload={selectedPayload} kind={selectedCard.kind} sub={selectedCard.sub} />
          <h4>关联边</h4>
          {selectedEdges.length ? selectedEdges.map((edge) => (
            <div key={edge.id} className="graph-detail-edge">
              <span>{edge.rel}</span>
              <span className="mono">
                {edge.srcId === selectedId ? "→" : "←"} {edge.srcId === selectedId ? edge.dstId : edge.srcId}
              </span>
            </div>
          )) : <p className="muted small">没有关联边。</p>}
        </div>
      ) : null}

      {nodes.length > 0 && !linked ? (
        <div className="graph-hint">
          暂无关系边：节点按阶段排列；让主代理用 attack_path_add 记录链路。
        </div>
      ) : null}

      {nodes.length === 0 ? (
        <div className="graph-empty">
          <p>还没有探索记录。</p>
          <p className="muted">发送第一条指令后，目标、意图、事实与漏洞会出现在这里。</p>
        </div>
      ) : null}
    </div>
  );
}

const DETAIL_LABELS: Record<string, string> = {
  severity: "严重度",
  vulnclass: "漏洞类型",
  basis: "依据",
  confidence: "置信度",
  status: "状态",
  company: "公司/组织",
  type: "类型",
  meta: "元数据",
  acceptance: "验收标准",
  goalId: "目标",
  evidenceIds: "证据",
  reproducibleSteps: "复现步骤",
};

function scalarText(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (Array.isArray(value)) return value.map((item) => scalarText(item)).filter(Boolean).join("、");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function scalarField(value: unknown): boolean {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean";
}

function GraphDetail({ payload, kind, sub }: { payload: unknown; kind: string; sub: string }) {
  const record = payload && typeof payload === "object" && !Array.isArray(payload)
    ? payload as Record<string, unknown>
    : {};
  const title = scalarText(record.title) || scalarText(record.text) || scalarText(record.value)
    || scalarText(record.detail) || kind;
  const summary = scalarText(record.summary) || scalarText(record.description) || scalarText(record.detail)
    || scalarText(record.text);
  const steps = Array.isArray(record.reproducibleSteps ?? record.steps)
    ? (record.reproducibleSteps ?? record.steps) as unknown[]
    : [];
  const evidence = Array.isArray(record.evidenceIds ?? record.evidence_ids)
    ? (record.evidenceIds ?? record.evidence_ids) as unknown[]
    : [];
  const skip = new Set(["title", "text", "value", "detail", "summary", "description", "reproducibleSteps", "steps", "evidenceIds", "evidence_ids"]);
  const rows = Object.entries(record)
    .filter(([key, value]) => !skip.has(key) && scalarField(value))
    .slice(0, 12);

  return (
    <div className="graph-detail-fields">
      <h3>{title}</h3>
      <p className="graph-detail-sub">{sub}</p>
      {summary && summary !== title ? <p className="graph-detail-summary">{summary}</p> : null}
      {rows.length ? (
        <dl>
          {rows.map(([key, value]) => (
            <div key={key}>
              <dt>{DETAIL_LABELS[key] ?? key}</dt>
              <dd>{scalarText(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {steps.length ? (
        <>
          <h4>复现步骤</h4>
          <ol className="detail-steps">{steps.map((step, index) => <li key={index}>{scalarText(step)}</li>)}</ol>
        </>
      ) : null}
      {evidence.length ? (
        <>
          <h4>证据</h4>
          <div className="chips">{evidence.map((id, index) => <span key={index} className="chip mono">{scalarText(id)}</span>)}</div>
        </>
      ) : null}
      <details className="graph-raw">
        <summary>原始数据</summary>
        <pre>{JSON.stringify(payload ?? {}, null, 2)}</pre>
      </details>
    </div>
  );
}

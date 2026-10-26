export function shortTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

export function dateTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export function truncate(value: string, n = 120): string {
  const text = (value ?? "").trim();
  if (text.length <= n) return text;
  return text.slice(0, n) + "…";
}

export function pretty(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export const modeLabels: Record<string, string> = {
  daily: "日常",
  bugbounty: "BugBounty",
  audit: "代码审计",
  redteam: "红队",
  ctf: "CTF",
};

export const statusLabels: Record<string, string> = {
  active: "进行中",
  paused: "已暂停",
  achieved: "已达成",
  stalled: "待决定",
  blocked: "被阻断",
  queued: "排队中",
  running: "执行中",
  done: "完成",
  failed: "失败",
  cancelled: "已取消",
  exhausted: "预算耗尽",
  open: "待处理",
  met: "已达成",
  abandoned: "已放弃",
  confirmed: "已确认",
  pending: "待确认",
  approved: "已批准",
  denied: "已拒绝",
  reproduced: "仍可复现",
  fixed: "已修复",
  inconclusive: "无法确认",
};

export const severityTone: Record<string, string> = {
  critical: "critical",
  high: "high",
  medium: "medium",
  low: "low",
  info: "info",
};

export function statusTone(status: string): string {
  switch (status) {
    case "active":
    case "running":
    case "done":
    case "met":
    case "fixed":
    case "approved":
      return "good";
    case "paused":
    case "stalled":
    case "queued":
    case "open":
    case "pending":
      return "warn";
    case "blocked":
    case "failed":
    case "cancelled":
    case "denied":
      return "bad";
    default:
      return "muted";
  }
}

export function nodeKindLabel(kind: string): string {
  const map: Record<string, string> = {
    goal: "目标",
    intent: "意图",
    fact: "事实",
    finding: "漏洞",
    hint: "提示",
    attack_path: "攻击路径",
    digest: "压缩",
    asset: "资产",
    decision: "决策",
    credential: "凭据",
    note: "备注",
    contact: "联系人",
  };
  return map[kind] ?? kind;
}

export function eventKindLabel(kind: string): string {
  const map: Record<string, string> = {
    tool_call: "工具调用",
    tool_result: "工具结果",
    assistant: "模型回复",
    assistant_delta: "流式输出",
    error: "错误",
    notice: "提示",
    compact: "上下文压缩",
    finding_status: "漏洞状态",
    auto: "自动继续",
    user: "用户指令",
    goal: "目标",
  };
  return map[kind] ?? kind;
}

import { AlertTriangle, Brain, Eye, Play, Send, Square, Terminal, Wrench } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import type { Approval, Event, Message, ModeMeta, RunDetail, ToolCall } from "../types";
import { pretty, shortTime, truncate } from "../lib/format";
import { Badge, Collapsible, Markdown } from "./ui";

interface Props {
  detail: RunDetail;
  mode?: ModeMeta;
  messages: Message[];
  approvals: Approval[];
  events: Event[];
  streaming?: string;
  busy: boolean;
  modelConfigured: boolean;
  onOpenSettings: () => void;
  onSend: (text: string) => Promise<void>;
  onStop: () => void;
  onResume: () => void;
  onDecide: (id: string, approve: boolean) => void;
}

const suggestions: Record<string, string[]> = {
  daily: ["检查这段配置的风险", "帮我审一下这个脚本", "给我一个可落地的加固清单"],
  bugbounty: ["在 scope 内做资产枚举并列出攻击面", "验证这个接口的越权", "把已确认漏洞整理成可提交报告"],
  audit: ["审计这个仓库的认证与授权逻辑", "追踪所有命令执行 sink 的调用链", "列出最可能被利用的 3 个问题"],
  redteam: ["按 ATT&CK 规划一次授权红队演练", "分析当前立足点的内网横向路径", "给出低暴露的下一步"],
  ctf: ["先识别题型并给出解题路线", "从附件里找 flag 格式线索", "把当前假设按信息增益排序"],
};

export default function Chat({
  detail,
  mode,
  messages,
  approvals,
  events,
  streaming,
  busy,
  modelConfigured,
  onOpenSettings,
  onSend,
  onStop,
  onResume,
  onDecide,
}: Props) {
  const [text, setText] = useState("");
  const scroller = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const runIdRef = useRef(detail.run.id);
  const running = detail.running;
  const achieved = detail.run.status === "achieved";
  const goalMode = detail.run.goalMode !== false;
  const paused = detail.run.status === "paused" || detail.run.status === "stalled"
    || (detail.run.status === "blocked" && modelConfigured);
  const lastEvent = events.length ? events[events.length - 1] : null;
  const lastError = [...events].reverse().find((event) => event.kind === "error");
  const showError = Boolean(lastError) && !running && (paused || detail.run.status === "blocked");

  function approve(approval: Approval) {
    if (window.confirm(`批准后 ${approval.tool} 会立即执行，确定？`)) {
      onDecide(approval.id, true);
    }
  }

  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    if (runIdRef.current !== detail.run.id) {
      runIdRef.current = detail.run.id;
      stickToBottom.current = true;
    }
    if (stickToBottom.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [detail.run.id, messages, approvals, events.length, streaming]);

  async function submit() {
    const value = text.trim();
    if (!value || busy) return;
    setText("");
    stickToBottom.current = true;
    await onSend(value);
  }

  function handleTranscriptScroll() {
    const el = scroller.current;
    if (!el) return;
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
  }

  return (
    <div className="chat">
      <div className="transcript" ref={scroller} onScroll={handleTranscriptScroll}>
        {!modelConfigured ? (
          <div className="chat-banner warn">
            <AlertTriangle size={18} />
            <div>
              <strong>尚未配置 LLM 端点</strong>
              <p>填写 base URL、API Key 和模型名后，任务才能开始运行。</p>
            </div>
            <button className="primary" onClick={onOpenSettings}>去设置</button>
          </div>
        ) : null}
        {showError && lastError ? (
          <div className="chat-banner error">
            <AlertTriangle size={18} />
            <div><strong>最近一次运行失败</strong><p>{lastError.summary || lastError.detail || "请检查模型端点后重试。"}</p></div>
          </div>
        ) : null}
        {messages.length === 0 ? (
          <div className="welcome">
            <div className="welcome-mark">
              <Eye size={28} />
            </div>
            <h2>{mode?.name ?? "开始一次任务"}</h2>
            <p>{mode?.tagline ?? "描述目标、授权与范围，lingeyebot 会持续推进直到目标达成。"}</p>
            <div className="suggestions">
              {(suggestions[detail.run.mode] ?? suggestions.daily).map((s) => (
                <button key={s} className="suggestion" onClick={() => setText(s)}>
                  {s}
                </button>
              ))}
            </div>
            <div className="welcome-note">
              当前授权：{detail.run.authorization || "未声明"} · 范围：{detail.run.scope || "未声明"}
            </div>
          </div>
        ) : null}

        {messages.length > 0 ? <ConversationTurns messages={messages} /> : null}

        {streaming ? (
          <div className="msg assistant">
            <div className="avatar"><Eye size={15} /></div>
            <div className="msg-body">
              <div className="msg-head"><span>lingeyebot</span><span className="muted">生成中…</span></div>
              <Markdown content={streaming} />
            </div>
          </div>
        ) : null}

        {approvals.map((approval) => (
          <div className="approval" key={approval.id}>
            <div className="approval-head">
              <AlertTriangle size={16} />
              <span>需要确认：{approval.tool}</span>
            </div>
            <p className="approval-reason">{approval.reason}</p>
            <pre className="code-block">{pretty(approval.input)}</pre>
            <div className="approval-actions">
              <button className="ghost-btn danger" onClick={() => onDecide(approval.id, false)}>
                拒绝
              </button>
              <button className="primary" onClick={() => approve(approval)}>
                批准执行
              </button>
            </div>
          </div>
        ))}
      </div>

      {running && lastEvent ? (
        <div className="live-strip">
          <span className="pulse" />
          <span className="live-tool">{lastEvent.tool || lastEvent.kind}</span>
          <span className="live-text">{truncate(lastEvent.summary, 140)}</span>
        </div>
      ) : null}

      <div className="composer">
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return;
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void submit();
            }
          }}
          placeholder={
            !modelConfigured
              ? "尚未配置 LLM：请先在设置里填写端点与模型"
              : running
                ? "运行中：输入会打断当前回合并以新指令继续…"
                : achieved
                  ? "目标已达成；可新增目标后继续"
                  : goalMode
                    ? "描述目标、授权与下一步（Enter 发送，Shift+Enter 换行）"
                    : "目标模式未启用：描述下一步（Enter 发送）"
          }
          rows={3}
        />
        <div className="composer-actions">
          <span className="composer-hint">
            {!modelConfigured
              ? "配置模型后才能发送"
              : running
                ? "可随时插话打断"
                : achieved
                  ? "目标已达成，任务已停止"
                  : paused
                    ? "任务已暂停，可继续"
                    : goalMode
                      ? "目标未达成时会持续自动推进"
                      : "目标模式未启用，不会自动继续"}
          </span>
          <div className="composer-buttons">
            {running ? (
              <button className="ghost-btn danger" onClick={onStop} title="停止当前任务">
                <Square size={15} /> 停止
              </button>
            ) : paused && modelConfigured ? (
              <button className="ghost-btn" onClick={onResume} title="恢复任务">
                <Play size={15} /> 继续
              </button>
            ) : null}
            <button className="primary" disabled={!text.trim() || busy || !modelConfigured} onClick={() => void submit()}>
              <Send size={15} /> {running ? "打断并发送" : "发送"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function ConversationTurns({ messages }: { messages: Message[] }) {
  const nodes: ReactNode[] = [];
  for (let i = 0; i < messages.length;) {
    const message = messages[i];
    if (message.role === "assistant") {
      const tools: Message[] = [];
      let next = i + 1;
      while (next < messages.length && messages[next].role === "tool") {
        tools.push(messages[next]);
        next++;
      }
      nodes.push(<AssistantTurn key={message.id} assistant={message} tools={tools} />);
      i = next;
      continue;
    }
    if (message.role === "tool") {
      nodes.push(<ToolResultRow key={message.id} message={message} />);
      i++;
      continue;
    }
    nodes.push(<MessageRow key={message.id} message={message} />);
    i++;
  }
  return <>{nodes}</>;
}

function AssistantTurn({ assistant, tools }: { assistant: Message; tools: Message[] }) {
  const calls = assistant.toolCalls ?? [];
  const results = new Map(tools.map((tool) => [tool.toolCallId, tool]));
  const loose = tools.filter((tool) => !calls.some((call) => call.id === tool.toolCallId));
  return (
    <div className="msg assistant">
      <div className="avatar"><Eye size={15} /></div>
      <div className="msg-body">
        <div className="msg-head">
          <span>lingeyebot</span>
          <time>{shortTime(assistant.createdAt)}</time>
        </div>
        {assistant.reasoning ? (
          <Collapsible
            title={<span className="thought-title"><Brain size={14} /> 思考</span>}
            meta={<span>{assistant.reasoning.length} 字符</span>}
          >
            <div className="reasoning-text"><Markdown content={assistant.reasoning} /></div>
          </Collapsible>
        ) : null}
        {assistant.content ? <Markdown content={assistant.content} /> : null}
        {calls.length ? (
          <Collapsible
            title={<span className="tool-title"><Wrench size={14} /> 工具调用 {calls.length}</span>}
            meta={<span>{tools.length} 个结果</span>}
          >
            <div className="tool-group">
              {calls.map((call) => <ToolStep key={call.id} call={call} result={results.get(call.id)} />)}
              {loose.map((tool) => <ToolResultRow key={tool.id} message={tool} />)}
            </div>
          </Collapsible>
        ) : loose.length ? (
          <div className="tool-group">{loose.map((tool) => <ToolResultRow key={tool.id} message={tool} />)}</div>
        ) : null}
      </div>
    </div>
  );
}

function ToolStep({ call, result }: { call: ToolCall; result?: Message }) {
  const args = pretty(call.arguments);
  const failed = result ? /^tool error|^tool call denied|^cancelled/i.test(result.content) : false;
  return (
    <div className={`tool-step ${failed ? "failed" : ""}`}>
      <header>
        <span className="tool-name"><Terminal size={13} /> {call.name}</span>
        {result ? <Badge tone={failed ? "bad" : "good"}>{failed ? "失败" : "完成"}</Badge> : <Badge tone="muted">等待结果</Badge>}
      </header>
      <pre className="tool-args">{truncate(args, 4000)}</pre>
      {result ? (
        <Collapsible title="结果" meta={<span>{result.content.length} 字符</span>}>
          <pre className="tool-output">{truncate(result.content, 12000)}</pre>
        </Collapsible>
      ) : null}
    </div>
  );
}

function ToolResultRow({ message }: { message: Message }) {
  const failed = /^tool error|^tool call denied|^cancelled/i.test(message.content);
  return (
    <div className={`tool-result-inline ${failed ? "failed" : ""}`}>
      <Collapsible
        title={<span className="tool-title"><Terminal size={14} /> {message.name || "tool"}</span>}
        meta={<span>{message.content.length} 字符</span>}
      >
        <pre className="code-block">{message.content}</pre>
      </Collapsible>
    </div>
  );
}

function MessageRow({ message }: { message: Message }) {
  if (message.role === "tool") {
    const failed = /^tool error|^tool call denied|^cancelled/i.test(message.content);
    return (
      <div className="msg tool">
        <div className={`tool-result ${failed ? "failed" : ""}`}>
          <Collapsible
            title={
              <span className="tool-title">
                <Terminal size={14} /> {message.name || "tool"}
              </span>
            }
            meta={<span>{message.content.length} 字符</span>}
          >
            <pre className="code-block">{message.content}</pre>
          </Collapsible>
        </div>
      </div>
    );
  }
  if (message.role === "assistant") {
    return (
      <div className="msg assistant">
        <div className="avatar">
          <Eye size={15} />
        </div>
        <div className="msg-body">
          <div className="msg-head">
            <span>lingeyebot</span>
            <time>{shortTime(message.createdAt)}</time>
          </div>
          {message.content ? <Markdown content={message.content} /> : null}
          {(message.toolCalls ?? []).map((call) => (
            <ToolCallCard key={call.id} call={call} />
          ))}
        </div>
      </div>
    );
  }
  return (
    <div className="msg user">
      <div className="msg-body">
        <div className="msg-head">
          <span>你</span>
          <time>{shortTime(message.createdAt)}</time>
        </div>
        <Markdown content={message.content} />
      </div>
    </div>
  );
}

function ToolCallCard({ call }: { call: ToolCall }) {
  const args = pretty(call.arguments);
  return (
    <div className="tool-call">
      <Collapsible
        title={
          <span className="tool-title">
            <Wrench size={14} /> 调用 {call.name}
          </span>
        }
        meta={<span>{truncate(args.replace(/\s+/g, " "), 80)}</span>}
      >
        <pre className="code-block">{args}</pre>
      </Collapsible>
    </div>
  );
}

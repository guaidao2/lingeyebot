import { Check, Plus, Server, Sparkles, Trash2, Wrench } from "lucide-react";
import { useState } from "react";
import type { AppConfig, AuthStatus, MCPStatus, ModeMeta, ProjectSummary, ProviderConfig, SkillInfo } from "../types";
import { modeLabels } from "../lib/format";
import { Badge, Modal } from "./ui";

export function NewRunDialog({
  modes,
  config,
  projects,
  projectId,
  initialMode,
  onClose,
  onCreate,
}: {
  modes: ModeMeta[];
  config: AppConfig | null;
  projects: ProjectSummary[];
  projectId?: string;
  initialMode?: string;
  onClose: () => void;
  onCreate: (payload: Record<string, unknown>) => Promise<void>;
}) {
  const initialProject = projects.find((item) => item.id === (projectId ?? projects[0]?.id)) ?? null;
  const [mode, setMode] = useState(initialMode ?? modes[0]?.id ?? "daily");
  const [selectedProjectId, setSelectedProjectId] = useState(initialProject?.id ?? "");
  const [target, setTarget] = useState("");
  const [objective, setObjective] = useState("");
  const [goalMode, setGoalMode] = useState(false);
  const [authorization, setAuthorization] = useState(initialProject?.authorization ?? "");
  const [scope, setScope] = useState(initialProject?.scope ?? "");
  const [deny, setDeny] = useState("");
  const [providerId, setProviderId] = useState(config?.activeProvider ?? config?.providers[0]?.id ?? "");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const selected = modes.find((m) => m.id === mode);
  const selectedProject = projects.find((item) => item.id === selectedProjectId) ?? null;

  function chooseProject(id: string) {
    setSelectedProjectId(id);
    const project = projects.find((item) => item.id === id);
    if (project) {
      setAuthorization(project.authorization ?? "");
      setScope(project.scope ?? "");
    }
  }

  async function submit() {
    if (!selectedProjectId) {
      setError("请先选择所属项目。");
      return;
    }
    if (!target.trim()) {
      setError("目标必填。");
      return;
    }
    if (goalMode && !objective.trim()) {
      setError("启用目标模式时，目的 / 验收必填。");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onCreate({
        projectId: selectedProjectId,
        mode,
        target: target.trim(),
        objective: goalMode ? objective.trim() : "",
        goalMode,
        authorization: authorization.trim(),
        scope: scope.trim(),
        providerId,
        model: model.trim(),
        constraints: deny
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean)
          .map((text) => ({ kind: "deny", text })),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <Modal
      title="新建任务"
      subtitle="选择项目与模式；目标模式可勾选。未启用时只对话，不会自动继续。"
      onClose={onClose}
      wide
    >
      <div className="dialog-project">
        <label className="field">
          <span>所属项目</span>
          <select value={selectedProjectId} onChange={(e) => chooseProject(e.target.value)}>
            {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
          </select>
        </label>
        <p className="muted small">
          {selectedProject ? `范围：${selectedProject.scope || "未声明"} · 授权：${selectedProject.authorization || "未声明"}` : "请先创建一个项目"}
        </p>
      </div>
      <span className="dialog-label">本轮模式</span>
      <div className="mode-grid">
        {modes.map((item) => (
          <button
            key={item.id}
            className={`mode-card ${mode === item.id ? "active" : ""}`}
            onClick={() => setMode(item.id)}
          >
            <div className={`mode-card-icon mode-${item.id}`}>{modeLabels[item.id]?.slice(0, 2) ?? "?"}</div>
            <div>
              <strong>{item.name}</strong>
              <p>{item.tagline}</p>
            </div>
            {mode === item.id ? <Check size={16} className="mode-check" /> : null}
          </button>
        ))}
      </div>

      {selected ? (
        <div className="role-strip">
          <span>内置子代理：</span>
          {selected.roles.map((role) => (
            <span key={role} className="chip">{role}</span>
          ))}
        </div>
      ) : null}

      <div className="form-grid">
        <label className={`goal-toggle field-span ${goalMode ? "on" : ""}`}>
          <input type="checkbox" checked={goalMode} onChange={(e) => setGoalMode(e.target.checked)} />
          <span className="goal-toggle-copy">
            <strong>启用目标模式</strong>
            <small>完成即停；未达成会自动继续，直到你叫停</small>
          </span>
        </label>
        <label className="field">
          <span>目标</span>
          <input
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="域名、IP、仓库路径、靶机地址或题目"
          />
        </label>
        <label className="field field-span">
          <span>目的 / 验收（目标模式）</span>
          <textarea
            value={objective}
            onChange={(e) => setObjective(e.target.value)}
            rows={3}
            disabled={!goalMode}
            placeholder={goalMode ? "要交付或证明什么。量化条件写清楚，例如：拿到 flag、覆盖率 80%、取得立足点。" : "目标模式未启用；这里不会创建验收目标。"}
          />
        </label>
        <label className="field">
          <span>授权</span>
          <input
            value={authorization}
            onChange={(e) => setAuthorization(e.target.value)}
            placeholder="谁授权、书面范围、测试窗口"
          />
        </label>
        <label className="field">
          <span>范围</span>
          <input
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            placeholder="允许触碰的域名 / IP / 路径"
          />
        </label>
        <label className="field">
          <span>模型端点</span>
          <select value={providerId} onChange={(e) => setProviderId(e.target.value)}>
            <option value="">（使用默认端点）</option>
            {(config?.providers ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.kind} · {p.model || "未命名模型"} · {p.baseUrl}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>模型覆盖（可选）</span>
          <input value={model} onChange={(e) => setModel(e.target.value)} placeholder="留空则用端点默认模型" />
        </label>
        <label className="field field-span">
          <span>禁止项（每行一条，会进入每一次提示与审批门）</span>
          <textarea
            value={deny}
            onChange={(e) => setDeny(e.target.value)}
            rows={2}
            placeholder="例如：不要触碰生产数据库&#10;不要对第三方发起请求"
          />
        </label>
      </div>

      {!config?.providers.length ? (
        <p className="form-warning">还没有配置 LLM 端点。创建任务后请先在「设置 → 模型」里填写 base URL、API Key 和模型名。</p>
      ) : null}
      {error ? <p className="error-text">{error}</p> : null}

      <div className="modal-actions">
        <button className="ghost-btn" onClick={onClose}>取消</button>
        <button className="primary" disabled={busy} onClick={() => void submit()}>
          {busy ? "创建中…" : "创建任务"}
        </button>
      </div>
    </Modal>
  );
}

export function SettingsDialog({
  config,
  mcpStatus,
  skills,
  auth,
  onSetPassword,
  onLogout,
  onClose,
  onSave,
}: {
  config: AppConfig;
  mcpStatus: MCPStatus[];
  skills: SkillInfo[];
  auth?: AuthStatus | null;
  onSetPassword?: (username: string, current: string, next: string) => Promise<void>;
  onLogout?: () => Promise<void>;
  onClose: () => void;
  onSave: (config: AppConfig) => Promise<void>;
}) {
  const [tab, setTab] = useState<"models" | "runtime" | "mcp" | "skills" | "account">("models");
  const [draft, setDraft] = useState<AppConfig>(() => JSON.parse(JSON.stringify(config)) as AppConfig);
  const [mcpJson, setMcpJson] = useState(() => JSON.stringify(config.mcp ?? [], null, 2));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [accountUser, setAccountUser] = useState(auth?.username ?? "bot");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");

  async function saveAccount() {
    if (!onSetPassword) return;
    if (newPassword.length < 8) {
      setError("新密码至少 8 位");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onSetPassword(accountUser, currentPassword, newPassword);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  function updateProvider(index: number, patch: Partial<ProviderConfig>) {
    setDraft((current) => {
      const providers = current.providers.map((p, i) => (i === index ? { ...p, ...patch } : p));
      let activeProvider = current.activeProvider;
      if (patch.id !== undefined && current.providers[index]?.id === activeProvider) {
        activeProvider = patch.id;
      }
      return { ...current, providers, activeProvider };
    });
  }

  function addProvider() {
    setDraft((current) => ({
      ...current,
      providers: [
        ...current.providers,
        {
          id: `p${Date.now().toString(36)}`,
          kind: "openai",
          baseUrl: "https://api.openai.com/v1",
          apiKey: "",
          model: "",
          maxTokens: 32768,
          maxTokensField: "",
          contextWindow: 0,
          reasoningMode: "auto",
          reasoningEffort: "",
          reasoningProfile: "auto",
        },
      ],
    }));
  }

  function applyPreset(index: number, preset: string) {
    if (!preset) return;
    const common = {
      maxTokens: 32768,
      maxTokensField: "",
      contextWindow: 0,
      reasoningMode: "auto",
      reasoningEffort: "",
    };
    if (preset === "deepseek") {
      updateProvider(index, {
        ...common, kind: "openai", baseUrl: "https://api.deepseek.com/v1",
        model: "deepseek-chat", reasoningProfile: "deepseek_compat",
      });
    } else if (preset === "openai") {
      updateProvider(index, {
        ...common, kind: "openai", baseUrl: "https://api.openai.com/v1",
        model: "gpt-4.1", reasoningProfile: "openai_compat",
      });
    } else if (preset === "anthropic") {
      updateProvider(index, {
        ...common, kind: "anthropic", baseUrl: "https://api.anthropic.com",
        model: "claude-sonnet-4-5", reasoningProfile: "auto",
      });
    }
  }

  function removeProvider(index: number) {
    setDraft((current) => {
      const providers = current.providers.filter((_, i) => i !== index);
      return {
        ...current,
        providers,
        activeProvider: providers.some((p) => p.id === current.activeProvider)
          ? current.activeProvider
          : providers[0]?.id ?? "",
      };
    });
  }

  function addSkillRoot() {
    setDraft((current) => ({ ...current, skills: [...(current.skills ?? []), ""] }));
  }

  function updateSkillRoot(index: number, value: string) {
    setDraft((current) => ({
      ...current,
      skills: (current.skills ?? []).map((root, i) => (i === index ? value : root)),
    }));
  }

  function removeSkillRoot(index: number) {
    setDraft((current) => ({
      ...current,
      skills: (current.skills ?? []).filter((_, i) => i !== index),
    }));
  }

  async function save() {
    let mcp = draft.mcp;
    try {
      mcp = JSON.parse(mcpJson) as AppConfig["mcp"];
    } catch {
      setError("MCP 配置不是合法 JSON。");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onSave({ ...draft, mcp });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="设置" subtitle="端点、运行限制、MCP 与技能。保存后立即生效。" onClose={onClose} wide>
      <div className="settings-tabs">
        <button className={tab === "models" ? "active" : ""} onClick={() => setTab("models")}>模型端点</button>
        <button className={tab === "runtime" ? "active" : ""} onClick={() => setTab("runtime")}>运行</button>
        <button className={tab === "mcp" ? "active" : ""} onClick={() => setTab("mcp")}>MCP</button>
        <button className={tab === "skills" ? "active" : ""} onClick={() => setTab("skills")}>技能</button>
        <button className={tab === "account" ? "active" : ""} onClick={() => setTab("account")}>账户</button>
      </div>

      {tab === "models" ? (
        <div className="settings-stack">
          {draft.providers.map((provider, index) => (
            <article key={index} className="provider-card">
              <header>
                <label className="radio">
                  <input
                    type="radio"
                    name="active-provider"
                    checked={draft.activeProvider === provider.id}
                    onChange={() => setDraft({ ...draft, activeProvider: provider.id })}
                  />
                  默认
                </label>
                <input
                  className="provider-id"
                  value={provider.id}
                  onChange={(e) => updateProvider(index, { id: e.target.value })}
                  placeholder="端点名称"
                  aria-label="端点名称"
                  title="端点名称（仅用于本机识别）"
                />
                <select value={provider.kind} onChange={(e) => updateProvider(index, { kind: e.target.value })}>
                  <option value="openai">OpenAI 兼容</option>
                  <option value="anthropic">Anthropic</option>
                </select>
                <button className="icon-btn danger" onClick={() => removeProvider(index)} title="删除端点">
                  <Trash2 size={15} />
                </button>
              </header>
              <div className="form-grid">
                <label className="field field-span">
                  <span>快速预设</span>
                  <select value="" onChange={(e) => applyPreset(index, e.target.value)}>
                    <option value="">选择预填充（仍可手动修改）…</option>
                    <option value="deepseek">DeepSeek · deepseek-chat · thinking 适配</option>
                    <option value="openai">OpenAI · gpt-4.1 · reasoning_effort</option>
                    <option value="anthropic">Anthropic · claude-sonnet-4-5</option>
                  </select>
                </label>
                <label className="field field-span">
                  <span>Base URL</span>
                  <input
                    value={provider.baseUrl}
                    onChange={(e) => updateProvider(index, { baseUrl: e.target.value })}
                    placeholder="https://api.openai.com/v1 或 https://api.anthropic.com"
                  />
                </label>
                <label className="field">
                  <span>模型</span>
                  <input
                    value={provider.model}
                    onChange={(e) => updateProvider(index, { model: e.target.value })}
                    placeholder="gpt-4o / claude-sonnet / 兼容模型名"
                  />
                </label>
                <label className="field">
                  <span>API Key</span>
                  <input
                    type="password"
                    value={provider.apiKey}
                    onChange={(e) => updateProvider(index, { apiKey: e.target.value })}
                    placeholder="sk-..."
                  />
                </label>
                <label className="field">
                  <span>单次输出上限（max_tokens）</span>
                  <input
                    type="number"
                    value={provider.maxTokens || 32768}
                    min={1}
                    onChange={(e) => updateProvider(index, { maxTokens: Number(e.target.value) })}
                  />
                  <small className="muted">只限制一次回复的输出，不是上下文窗口；渗透/推理建议 32768。</small>
                </label>
                <label className="field">
                  <span>通道上下文窗口</span>
                  <input
                    type="number"
                    value={provider.contextWindow ?? 0}
                    min={0}
                    onChange={(e) => updateProvider(index, { contextWindow: Number(e.target.value) })}
                    placeholder="0 = 跟随运行设置（1,000,000）"
                  />
                  <small className="muted">该通道输入+输出总预算；达到 80% 压缩，0 表示跟随运行设置。</small>
                </label>
                {provider.kind === "openai" ? (
                  <label className="field">
                    <span>输出上限字段（OpenAI）</span>
                    <select
                      value={provider.maxTokensField ?? ""}
                      onChange={(e) => updateProvider(index, { maxTokensField: e.target.value })}
                    >
                      <option value="">自动（推理模型用 max_completion_tokens）</option>
                      <option value="max_tokens">max_tokens（多数兼容端点）</option>
                      <option value="max_completion_tokens">max_completion_tokens（o 系列 / GPT-5）</option>
                    </select>
                  </label>
                ) : (
                  <p className="muted small">Anthropic 固定使用 max_tokens 字段。</p>
                )}
                {provider.kind === "openai" ? (
                  <>
                    <label className="field">
                      <span>推理模式</span>
                      <select
                        value={provider.reasoningMode ?? "auto"}
                        onChange={(e) => updateProvider(index, { reasoningMode: e.target.value })}
                      >
                        <option value="auto">自动（跟随模型/请求）</option>
                        <option value="on">开启</option>
                        <option value="off">关闭</option>
                      </select>
                    </label>
                    <label className="field">
                      <span>推理强度</span>
                      <select
                        value={provider.reasoningEffort ?? ""}
                        onChange={(e) => updateProvider(index, { reasoningEffort: e.target.value })}
                      >
                        <option value="">模型默认</option>
                        <option value="none">none</option>
                        <option value="low">low</option>
                        <option value="medium">medium</option>
                        <option value="high">high</option>
                        <option value="max">max</option>
                        <option value="xhigh">xhigh</option>
                      </select>
                    </label>
                    <label className="field">
                      <span>推理适配</span>
                      <select
                        value={provider.reasoningProfile ?? "auto"}
                        onChange={(e) => updateProvider(index, { reasoningProfile: e.target.value })}
                      >
                        <option value="auto">自动（DeepSeek 模型用 thinking）</option>
                        <option value="openai_compat">OpenAI 兼容（reasoning_effort）</option>
                        <option value="deepseek_compat">DeepSeek 兼容（thinking）</option>
                      </select>
                    </label>
                  </>
                ) : (
                  <p className="muted small">Anthropic 的推理由模型自身设置控制；这里只管理通道上下文与输出上限。</p>
                )}
              </div>
            </article>
          ))}
          <button className="ghost-btn" onClick={addProvider}><Plus size={15} /> 添加端点</button>
          <p className="muted small">密钥只保存在本机配置文件中；界面显示为掩码。留空或保持掩码会保留原值；要更换请直接输入新 key。</p>
        </div>
      ) : null}

      {tab === "runtime" ? (
        <div className="form-grid settings-stack">
          <label className="field">
            <span>人机协同（审批）</span>
            <select
              value={draft.approval.mode}
              onChange={(e) => setDraft({ ...draft, approval: { ...draft.approval, mode: e.target.value } })}
            >
              <option value="auto">关闭（默认）：自动放行，仍受 deny 约束</option>
              <option value="ask">开启：高风险动作需人工确认</option>
              <option value="deny">严格：拒绝需要确认的动作</option>
            </select>
          </label>
          <label className="field">
            <span>单轮工具迭代上限（0 = 不限制）</span>
            <input
              type="number"
              value={draft.runtime.maxIterations}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, maxIterations: Number(e.target.value) } })}
            />
          </label>
          <label className="field">
            <span>自动继续上限（轮，0 = 不限制）</span>
            <input
              type="number"
              value={draft.runtime.maxAutoTurns}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, maxAutoTurns: Number(e.target.value) } })}
            />
          </label>
          <label className="field">
            <span>自动继续间隔（秒）</span>
            <input
              type="number"
              value={draft.runtime.autoContinueWaitSeconds}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, autoContinueWaitSeconds: Number(e.target.value) } })}
            />
          </label>
          <label className="field">
            <span>上下文消息数</span>
            <input
              type="number"
              value={draft.runtime.contextMessages}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, contextMessages: Number(e.target.value) } })}
            />
          </label>
          <label className="field">
            <span>上下文窗口（模型总容量）</span>
            <input
              type="number"
              value={draft.runtime.contextWindow || 1000000}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, contextWindow: Number(e.target.value) } })}
            />
            <small className="muted">input + output 的总容量；达到 80% 时触发压缩，不是 max_tokens。</small>
          </label>
          <label className="field">
            <span>工具结果截断字符</span>
            <input
              type="number"
              value={draft.runtime.toolResultChars}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, toolResultChars: Number(e.target.value) } })}
            />
          </label>
          <label className="field checkbox">
            <input
              type="checkbox"
              checked={draft.runtime.autoContinue}
              onChange={(e) => setDraft({ ...draft, runtime: { ...draft.runtime, autoContinue: e.target.checked } })}
            />
            <span>目标未达成时自动继续（关闭后每轮结束等待你输入）</span>
          </label>
        </div>
      ) : null}

      {tab === "mcp" ? (
        <div className="settings-stack">
          <div className="mcp-status">
            {mcpStatus.map((server) => (
              <div key={server.id} className="mcp-row">
                <Server size={15} />
                <span className="mono">{server.id}</span>
                <span className="muted">{server.transport}</span>
                <Badge tone={server.state === "ready" ? "good" : server.state === "disabled" ? "muted" : "bad"}>
                  {server.state}
                </Badge>
                <span className="muted">{server.tools} tools</span>
                {server.error ? <span className="error-text">{server.error}</span> : null}
              </div>
            ))}
            {mcpStatus.length === 0 ? <p className="muted">还没有配置 MCP 服务器。</p> : null}
          </div>
          <label className="field">
            <span>MCP 配置（JSON，保存后重连）</span>
            <textarea rows={12} className="mono" value={mcpJson} onChange={(e) => setMcpJson(e.target.value)} />
          </label>
          <p className="muted small">
            示例：<code>{`[{"id":"fs","transport":"stdio","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"],"enabled":true}]`}</code>
          </p>
        </div>
      ) : null}

      {tab === "skills" ? (
        <div className="settings-stack">
          <div className="settings-subhead">
            <div>
              <strong>技能根目录</strong>
              <small className="muted">每个目录递归扫描 SKILL.md；保存设置后重建索引，可配置多个。</small>
            </div>
            <button className="ghost-btn" onClick={addSkillRoot}><Plus size={15} /> 添加目录</button>
          </div>
          <div className="skill-roots">
            {(draft.skills ?? []).map((root, index) => (
              <div className="skill-root-row" key={index}>
                <input
                  className="mono"
                  value={root}
                  onChange={(e) => updateSkillRoot(index, e.target.value)}
                  placeholder="/root/.lingeyebot/workspace/skills"
                />
                <button className="icon-btn danger" onClick={() => removeSkillRoot(index)} title="移除目录">
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
            {(draft.skills ?? []).length === 0 ? <p className="muted small">未配置时保存会回退到 workspace/skills。</p> : null}
          </div>
          <div className="skill-list">
            {skills.map((skill) => (
              <div key={skill.name} className="skill-row">
                <Sparkles size={15} />
                <div>
                  <strong>{skill.name}</strong>
                  <p>{skill.description}</p>
                  <span className="mono muted">{skill.path}</span>
                </div>
              </div>
            ))}
            {skills.length === 0 ? (
              <p className="muted">还没有技能。把 SKILL.md 放进 workspace 的 skills 目录，保存设置后会重新扫描。</p>
            ) : null}
          </div>
          <p className="muted small">
            <Wrench size={13} /> 技能是渐进加载的：只把名称与描述注入提示，模型需要时再读取全文。
          </p>
        </div>
      ) : null}

      {tab === "account" ? (
        <div className="settings-stack">
          <div className="form-grid">
            <label className="field"><span>用户名</span><input value={accountUser} onChange={(e) => setAccountUser(e.target.value)} /></label>
            <label className="field"><span>当前密码</span><input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} /></label>
            <label className="field field-span"><span>新密码（至少 8 位）</span><input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} /></label>
          </div>
          <p className="muted small">
            {auth?.enabled ? "登录已启用；保存后会清除当前会话，需要重新登录。" : "当前未启用登录；设置密码后启用个人账户。"}
          </p>
          <div className="modal-actions" style={{ marginTop: 0, border: 0, paddingTop: 0 }}>
            {auth?.enabled && onLogout ? (
              <button className="ghost-btn danger" onClick={() => void onLogout()}>退出登录</button>
            ) : null}
            <button className="primary" disabled={busy} onClick={() => void saveAccount()}>保存账户</button>
          </div>
        </div>
      ) : null}

      {error ? <p className="error-text">{error}</p> : null}

      {tab !== "account" ? (
        <div className="modal-actions">
          <button className="ghost-btn" onClick={onClose}>取消</button>
          <button className="primary" disabled={busy} onClick={() => void save()}>
            {busy ? "保存中…" : "保存设置"}
          </button>
        </div>
      ) : null}
    </Modal>
  );
}

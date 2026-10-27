# lingeyebot

本地优先、项目驱动的授权安全测试智能体。Go 单二进制 + SQLite + 内嵌 React Web UI，面向日常安全协作、BugBounty、代码审计、红队与 CTF。

**作者：guaidao2 & coolmoon**

> **状态：稳定版**
>
> **仅用于已获得明确授权、且在你拥有测试权限的范围内。** 使用者自行承担合规责任。
>
> 模型输出与工具调用必须由专业人员复核；本工具不保证发现全部问题。

详细文档：[使用手册](docs/USAGE.md) · [设计哲学](docs/DESIGN.md)

---

## 目录

- [为什么是 lingeyebot](#为什么是-lingeyebot)
- [核心功能](#核心功能)
- [快速开始](#快速开始)
- [使用流程](#使用流程)
- [模型与 Token 配置](#模型与-token-配置)
- [模式与内置角色](#模式与内置角色)
- [MCP 与 Skills](#mcp-与-skills)
- [安全边界](#安全边界)
- [数据、备份与删除](#数据备份与删除)
- [架构](#架构)
- [开发与测试](#开发与测试)
- [部署](#部署)
- [已知限制](#已知限制)
- [许可](#许可)

---

## 为什么是 lingeyebot

安全测试不是“发一句 prompt 等结果”，而是长期、可追溯、需要证据的工程活动。lingeyebot 围绕四件事设计：

- **项目优先（Project-first）**：授权、范围、资产、发现、共享黑板和项目记忆都属于项目，而不是一次对话。
- **证据优先（Evidence-first）**：事实必须区分 `observed / inferred / assumed / negative`；`observed` 绑定证据；漏洞必须有可复现步骤。
- **目标模式可选（Goal mode）**：新建任务时可勾选“启用目标模式”；开启后达成即停，未达成会持续推进，直到你叫停。
- **人机协同可选（Human-in-the-loop）**：默认自动执行，需要时在设置里切换审批模式；破坏性命令始终不会被静默执行。

它不试图替代 Metasploit、Cobalt Strike、Burp 或 nuclei。它负责把侦察、建模、验证、证据、报告和子代理协作组织成一个可审计的工作台。

---

## 核心功能

### 项目工作台

- 项目管理：新建、编辑、搜索、删除（需输入项目名确认）。
- 项目 → 选择模式 → 创建任务；任务按模式分组。
- 项目共享：授权/范围、资产、发现、攻击链、共享黑板、项目记忆。
- 共享黑板和项目记忆由指挥官与子代理写入；人类通过对话下达指令，不需要手填。
- 删除项目会事务级联清理任务、消息、节点、证据、审批、子代理、黑板和记忆；资产本身保留。
- 内置默认项目受保护，不能删除。

### 任务与子代理

- 每个任务记录授权、范围、禁止项和模型通道；目标/验收条件在启用目标模式时填写。
- 运行中可随时插话；新消息会打断当前回合并带着新指令继续。
- 子代理是基础能力：情报收集、主动枚举、脆弱点建模、前渗透、后渗透、隐蔽、worker。
- 子代理支持追加指令（inbox）、取消、可选迭代预算（默认不限制）和部分结果回注。
- 子代理详情可查看状态、所属任务、耗时、可选预算、结果、错误和 inbox。

### 目标模式与自动继续（可选）

- 新建任务时勾选“启用目标模式”才会创建目标并自动继续；默认关闭。
- 启用后，目标必须填写验收条件；全部达成后任务结束。
- 未达成时自动继续，先看当前态势和信念缺口，再选下一步。
- 默认不限制自动继续轮数或迭代次数；只有显式设置上限才会限制。
- 连续多轮没有新事实/证据时，只有在设置了自动继续上限的情况下才会标记 `stalled`；默认由用户手动停止。
- 未启用目标模式时按普通对话执行，不自动继续。
- 停止、恢复、取消子代理都是显式操作，不会静默继续。

### 证据与报告

- 事实分级，`observed` 必须有证据。
- 资产覆盖率按“有证据覆盖”计算，不按“扫过”计算。
- 漏洞必须包含可复现步骤，并绑定证据。
- 报告包含目标、范围、覆盖、事实、漏洞、证据、信念缺口和复核清单。
- 所有工具调用进入审计；事件流通过 SSE 实时推送。

### 工具体系

内置：

- 命令执行（含破坏性命令硬拒绝）
- 文件读写、列目录、搜索
- HTTP 请求
- 目标、资产、节点、边、事实、证据、覆盖
- 项目黑板/项目记忆
- 子代理派发/取消/查询
- Skill 读取
- MCP 工具（stdio / Streamable HTTP）
- Markdown 报告导出

### 模型通道

- 多通道/Profile：每个通道独立的 Base URL、API Key、模型、输出上限、上下文窗口、推理配置。
- OpenAI 兼容与 Anthropic Messages 两种协议。
- 支持 OpenAI 推理模型与 DeepSeek thinking 的差异字段。
- Token 级流式输出。
- 超过上下文预算 80% 自动摘要压缩，保留 tool_call 与结果配对。

### MCP 与 Skills

- MCP 管理页：增删改、启用/停用、stdio/HTTP、环境变量/请求头、测试连接、工具名清单。
- 官方 Go MCP SDK，工具自动注册为 `mcp__server__tool`。
- Skills 管理页：查看正文、新建、编辑、保存后自动重载。
- 技能根目录可手动配置多个，递归扫描 `SKILL.md`；批量导入只需把目录复制进去。
- 兼容常见 Agent Skills 的 frontmatter：`name`、`description`。

### 账户与审计

- 单用户、本地优先；默认免登录，只监听 `127.0.0.1`。
- 可选启用个人账户：bcrypt 密码 + HMAC 签名 Cookie。
- 审批模式：`auto`（默认，关闭人机协同）/ `ask`（危险操作询问）/ `deny`（拒绝需审批工具）。
- 审批日志、工具审计、事件流均可追溯。

---

## 快速开始

### 依赖

- Go **1.27+**
- Node.js **20+**（只在构建 UI 时需要）

### 构建

```bash
git clone <this repo>
cd lingeyebot

./scripts/build.sh
# 指定平台：
GOOS=windows GOARCH=amd64 ./scripts/build.sh
GOOS=linux   GOARCH=arm64 ./scripts/build.sh
```

产物在 `bin/lingeyebot-<os>-<arch>`。前端通过 `go:embed` 打进二进制；SQLite 使用纯 Go 驱动（`CGO_ENABLED=0`），可以直接拷到 Linux 运行。

> 只执行 `go build ./cmd/lingeyebot` 也能编译，但 UI 是占位页；完整 UI 请使用 `./scripts/build.sh`。

### 运行

```bash
./bin/lingeyebot-linux-amd64
# 默认监听 127.0.0.1:8787
# 浏览器打开 http://127.0.0.1:8787
```

常用参数：

```bash
./lingeyebot -listen 0.0.0.0:8787 -data /var/lib/lingeyebot
./lingeyebot -version
```

环境变量：

- `LINGEYE_LISTEN`
- `LINGEYE_DATA_DIR`
- `LINGEYE_CONFIG`

首次启动会在当前工作目录生成 `./config.json`、`./data/lingeyebot.sqlite` 和 `./workspace/`。到 **设置 → 模型端点** 添加一个通道：

- 协议：OpenAI 兼容 / Anthropic
- Base URL：例如 `https://api.openai.com/v1`、`https://api.deepseek.com/v1`、`https://api.anthropic.com`
- API Key、模型名
- 输出上限、上下文窗口、推理配置

密钥只保存在本机配置文件中，Web 界面显示掩码。

---

## 使用流程

1. **新建项目**：填写名称、说明、授权、范围。
2. **进入项目，选择模式**：日常 / BugBounty / 代码审计 / 红队 / CTF。
3. **新建任务**：填写目的、模型通道和禁止项；需要自动推进时勾选“启用目标模式”，再填写目标与验收条件。
4. **观察执行**：对话、探索链路、资产、证据、子代理和事件流。
5. **过程干预**：随时插话、停止/恢复、处理审批、取消子代理。
6. **验收结果**：确认漏洞状态、查看覆盖率、导出报告。
7. **清理项目**：删除前输入项目名确认；默认项目不可删除。

---

## 模型与 Token 配置

### 单次输出上限 ≠ 上下文窗口

| 参数 | 含义 | 默认 |
|---|---|---|
| `max_tokens` / `max_completion_tokens` | 单次回复最多生成的 token（含思维链和工具参数） | `32768` |
| 上下文窗口 | 输入 + 输出总容量 | `1,000,000` |
| 压缩阈值 | 达到上下文窗口的比例后摘要较早消息 | `80%` |

不要把 1M 的 80% 填进 `max_tokens`。大多数模型单次输出上限只有 8k/32k/64k，填 800k 会被接口拒绝或截断。

- OpenAI 推理模型（o 系列 / GPT-5）通常只接受 `max_completion_tokens`；设置页默认自动选择，也可以手动指定。
- DeepSeek 模型使用 `thinking: {type: enabled|disabled}`。
- Anthropic 固定使用 `max_tokens`，上下文窗口仍可通过 `-data` 配置。
- 每个通道可以单独设置上下文窗口；`0` 表示跟随运行级设置。

旧配置会做一次性迁移：历史默认 `8192` 输出 / `128000` 上下文 / provider `4096` 输出会提升到新默认。旧配置里的自定义值不会被无条件覆盖。

---

## 模式与内置角色

| 模式 | 定位 | 典型内置角色 |
|---|---|---|
| `daily` | 安全伴侣 + 安全开发 | worker、intel-collector |
| `bugbounty` | SRC / BugBounty | intel-collector、enumerator、threat-modeler、pre-pentester、worker |
| `audit` | 白盒代码审计 | worker、threat-modeler |
| `redteam` | ATT&CK 红队行动 | 全部七种（含 post-pentester、evasion） |
| `ctf` | 求 flag（CTF 必有解） | worker、intel-collector、enumerator |

内置子代理：

- **intel-collector**：公开信息、业务、客户、关联攻击路线
- **enumerator**：主动信息收集、暴露面枚举
- **threat-modeler**：把情报转成攻击路径与脆弱点模型
- **pre-pentester**：验证到拿下立足点
- **post-pentester**：内网弱点与后渗透分析
- **evasion**：红队行动中的降噪与痕迹管理（不删除审计/证据）
- **worker**：由指挥官自由分配的自定义职责

子代理结束会把结构化结果和执行摘要回注父会话；失败/取消保留部分结果。

---

## MCP 与 Skills

### MCP

在 **设置 / MCP 页面** 添加服务器：

- stdio：`command`、`args`、`env`
- HTTP：`url`、`headers`
- 支持启用/停用、测试连接、工具名清单
- 保存后重连；测试只验证草稿，不改当前连接

示例：

```json
[
  {
    "id": "filesystem",
    "transport": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
    "enabled": true
  },
  {
    "id": "remote",
    "transport": "http",
    "url": "https://mcp.example.com/mcp",
    "headers": { "Authorization": "Bearer <token>" },
    "enabled": true
  }
]
```

### Skills

默认目录（相对于启动时的工作目录）：

- Linux/Kali：`./skills`
- Windows：`.\skills`
- 自定义数据目录：`<dataDir>/skills`

也可以到 **设置 → 技能 → 技能根目录** 手动添加多个路径。每个目录递归扫描 `SKILL.md`：

```text
skills/
  api-security/
    SKILL.md
  ssrf-testing/
    SKILL.md
    REFERENCE.md
```

`SKILL.md`：

```markdown
---
name: ssrf-testing
description: SSRF 漏洞识别、验证、绕过和修复建议流程
---

# SSRF Testing

当任务涉及服务端请求伪造、URL 回调、云元数据或内网探测时使用。
```

批量导入：把整个技能目录复制到任意配置的根目录，保存设置或重启即可。当前没有文件监听和 ZIP 一键导入。

---

## 安全边界

- 只对**已获得明确授权**的目标使用；在任务中记录授权与范围。
- 默认只监听 `127.0.0.1`；不要在没有账户/反向代理保护的情况下暴露到公网。
- 单用户本地形态，暂无多用户/RBAC。
- 人机协同默认关闭（`auto`）；需要人工确认时切到 `ask`。
- 破坏性 shell（删除、磁盘写入、关机、凭据导出等）在 `auto` 下也直接拒绝，不会静默执行。
- 审计日志记录每次工具调用的 allow/deny/ask 决策；隐蔽员不能删除审计、证据或行动记录。
- API Key 只保存在本机配置文件，界面显示掩码；不会发送给模型。
- 项目删除是不可恢复操作，前端要求输入项目名确认。

---

## 数据、备份与删除

默认全部落在**启动时的工作目录**：

```text
<当前目录>/
  config.json
  data/
    lingeyebot.sqlite
    evidence/blobs/<sha前2>/<sha>.bin
  workspace/
  skills/
```

| 路径 | 内容 |
|---|---|
| `config.json` | 通道、审批、运行限制、MCP、Skills 根目录、账户 |
| `data/lingeyebot.sqlite` | 项目、任务、节点、资产、证据索引、会话、审计 |
| `data/evidence/blobs/<sha前2>/<sha>.bin` | 按 sha256 内容寻址的证据正文 |
| `workspace/` | 工具工作目录；与 `skills/` 平级 |
| `skills/` | 全局技能根目录；可被多个项目/任务共享 |

默认不再写入 `~/.lingeyebot`。需要把数据放到别处时：

```bash
./lingeyebot -data /var/lib/lingeyebot
# 或
LINGEYE_DATA_DIR=/var/lib/lingeyebot ./lingeyebot
```

`config.json` 仍默认读取启动目录下的文件；可用 `LINGEYE_CONFIG` 覆盖。

备份：直接备份整个运行目录；SQLite 建议在停止进程后复制，或使用 SQLite 在线备份。

项目删除会清理项目下所有关联数据；默认项目受保护。资产本身保留，只删除项目关联。

---

## 架构

```text
cmd/lingeyebot          入口与参数
internal/config         配置加载/迁移/归一化
internal/model          领域模型
internal/store          SQLite schema 与 DAO
internal/llm            OpenAI 兼容 / Anthropic 适配
internal/agent          loop、runner、压缩、子代理
internal/mode           五种模式、提示词、目标验收、角色
internal/tool           shell/文件/HTTP/图谱/报告/Skill/MCP/子代理
internal/policy         审批门与审计
internal/mcp            官方 Go MCP SDK 封装
internal/server         HTTP API、SSE、内嵌静态资源
web/                    React + TypeScript + Vite
scripts/                构建、测试、开发脚本
```

- 后端：Go，`net/http` + `database/sql`。
- 数据库：SQLite（`modernc.org/sqlite`，纯 Go，`CGO_ENABLED=0`）。
- 前端：React + TypeScript + Vite，构建后通过 `go:embed` 内嵌。
- 实时：SSE（事件、流式消息、审批状态）。
- 单二进制：Linux/Windows 交叉编译，15MB 左右。

主要 API：

- `GET/POST /api/projects`
- `GET/PUT/DELETE /api/projects/{id}`
- `POST /api/projects/{id}/conversations`
- `GET/POST /api/projects/{id}/board`
- `GET/POST /api/projects/{id}/memory`
- `GET/POST /api/runs`、`/api/runs/{id}/messages`
- `GET /api/runs/{id}/events/stream`（SSE）
- `GET /api/dashboard`、`/api/findings`、`/api/assets`、`/api/tasks`、`/api/approvals`
- `GET/POST /api/skills`、`GET /api/skills/{name}`
- `GET /api/mcp`、`POST /api/mcp/test`
- `GET/PUT /api/config`

---

## 开发与测试

```bash
./scripts/test.sh                  # GOTMPDIR 放工作区 .gotmp，避免杀软拦截测试二进制
./scripts/dev.sh                   # 后端 :8787
cd web && npm install
cd web && npm run dev              # 前端 :5173，/api 代理到 8787
cd web && npm run build            # 类型检查 + 生产构建
```

Go：

```bash
GOTMPDIR="$PWD/.gotmp" go test ./...
GOTMPDIR="$PWD/.gotmp" go vet ./...
```

开发约定：

- 测试不连接真实 LLM；使用 mock/httptest。
- 不把 API Key、Cookie、真实目标数据提交到仓库。

---

## 部署

最小部署：

```bash
scp bin/lingeyebot-linux-amd64 user@host:/opt/lingeyebot/lingeyebot
ssh user@host '/opt/lingeyebot/lingeyebot -listen 127.0.0.1:8787 -data /var/lib/lingeyebot'
```

systemd 示例：

```ini
[Unit]
Description=lingeyebot
After=network.target

[Service]
ExecStart=/opt/lingeyebot/lingeyebot -listen 127.0.0.1:8787 -data /var/lib/lingeyebot
Restart=on-failure
User=lingeyebot
Group=lingeyebot
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

如果要监听 `0.0.0.0`，必须同时启用账户登录、反向代理 TLS 和访问控制；这是一个能执行本机命令的 agent，不要裸露到不可信网络。

---

## 已知限制

- 仅提供 Web UI（单二进制内嵌），不包含 CLI/TUI。
- 单用户、本地优先，没有多用户/RBAC。
- MCP 支持 stdio / Streamable HTTP；OAuth、sampling 等高级能力未内置。
- Skills 没有文件监听，也没有 ZIP 一键导入；复制目录后保存设置/重启。
- 不同模型网关对 `reasoning_effort`、`thinking`、`max_completion_tokens` 的兼容性不一致；遇到 400 时按模型文档调整通道配置。
- 项目删除不可恢复；默认项目不能删除。
- 启用目标模式后，自动继续会持续消耗 token；默认不限制轮数，可随时停止。
- LLM 输出、工具调用和报告结论必须由专业人员复核；不保证覆盖全部攻击面或发现全部漏洞。

---

## 许可

MIT，Copyright (c) 2026 guaidao2 & coolmoon，见 [LICENSE](LICENSE)。

# lingeyebot 使用手册

版本：1.0.1  
作者：guaidao2 & coolmoon

> lingeyebot 只用于已经获得明确授权、且在你拥有测试权限的范围内。  
> 模型输出、工具调用和报告结论都需要专业人员复核，不能替代人工判断。

## 1. 是什么

lingeyebot 是一个本地优先、项目驱动的授权安全测试智能体。它把侦察、事实建模、证据、漏洞验证、报告、子代理协作和审批组织在一个 Web 工作台里。

它不替代 Metasploit、Cobalt Strike、Burp、nuclei 等工具，而是负责组织流程、保存证据和让每一步可追溯。

## 2. 安装与启动

### 依赖

- Go 1.27+
- Node.js 20+（构建 Web UI 时需要）
- Linux 或 Windows；Linux 是主要运行环境

### 构建

```bash
git clone https://github.com/guaidao2/lingeyebot.git
cd lingeyebot
./scripts/build.sh
```

产物在 `bin/lingeyebot-<os>-<arch>`。前端会通过 `go:embed` 进入二进制，SQLite 使用纯 Go 驱动，默认 `CGO_ENABLED=0`。

### 运行

```bash
./bin/lingeyebot-linux-amd64
# 默认 http://127.0.0.1:8787
```

常用参数：

```bash
./lingeyebot -listen 127.0.0.1:8787 -data /var/lib/lingeyebot
./lingeyebot -version
```

环境变量：

- `LINGEYE_LISTEN`
- `LINGEYE_DATA_DIR`
- `LINGEYE_CONFIG`

首次启动会在当前工作目录生成：

```text
config.json
data/lingeyebot.sqlite
data/evidence/blobs/
workspace/
skills/
```

## 3. 首次配置模型

进入 **系统设置 → 模型端点**：

- 协议：OpenAI 兼容或 Anthropic
- Base URL
- API Key
- 模型名
- 单次输出上限
- 上下文窗口
- 推理配置

单次输出上限和上下文窗口不是一回事：

| 参数 | 含义 | 默认 |
|---|---|---|
| 单次输出上限 | 一次回复最多生成多少 token | 32768 |
| 上下文窗口 | 输入 + 输出总额度 | 1,000,000 |
| 压缩阈值 | 达到上下文窗口的比例后摘要旧消息 | 80% |

不要把 1M 的 80% 填进单次输出上限。API Key 只保存在本机 `config.json`，界面显示掩码。

## 4. 核心概念

### 项目

项目保存授权、范围、资产、发现、共享黑板、项目记忆和多个任务。项目不是一次对话。

### 任务 / 对话

任务隶属于项目。新建任务时选择模式，并可选择是否启用目标模式。

### 模式

| 模式 | 用途 |
|---|---|
| 日常 | 安全伴侣、配置检查、安全开发 |
| BugBounty | SRC / BugBounty 漏洞挖掘 |
| 代码审计 | 白盒代码审计与验证 |
| 红队 | ATT&CK 框架下的红队行动 |
| CTF | 求 flag，按题解链推进 |

### 目标模式

默认关闭。只有在新建任务时勾选“启用目标模式”，才会创建目标并自动继续。

启用后：

- 达成目标即停
- 未达成会自动继续
- 目标达成后仍可继续追问；发送消息会重新激活任务。
- 默认不限制自动继续轮数或迭代次数；只有显式设置上限才会限制。
- 随时可以停止、恢复或插话

### 子代理

内置角色：

- `intel-collector`：公开信息与业务情报
- `enumerator`：主动枚举与暴露面
- `threat-modeler`：攻击路径与脆弱点建模
- `pre-pentester`：验证到立足点
- `post-pentester`：内网弱点与后渗透分析
- `evasion`：红队降噪与痕迹管理（不能删除审计/证据）
- `worker`：自定义职责

子代理的实时输出只显示在它自己的任务详情里；主对话只显示主代理输出。子代理完成后，结果写回任务详情并回到主代理上下文，不会作为普通用户消息重复塞进主对话。

### 证据与发现

- 事实必须区分 `observed / inferred / assumed / negative`
- `observed` 必须绑定证据
- 漏洞必须有可复现步骤
- 覆盖率按“有证据覆盖”计算，不按“扫过”计算
- 报告包含目标、覆盖、事实、漏洞、证据、信念缺口和复核清单

## 5. 工作流

1. **新建项目**：名称、说明、授权、范围
2. **进入项目，选择模式**
3. **新建任务**：目的、模型通道、禁止项；需要自动推进时勾选目标模式
4. **观察执行**：对话、攻击链、资产、发现、证据、子代理
5. **干预**：插话、停止/恢复、审批、取消子代理
6. **验收**：确认漏洞状态、覆盖率、报告
7. **清理**：删除项目需要输入项目名确认

### 插话与停止

- 运行中输入消息会打断当前回合，带着新指令继续
- “停止”会暂停任务并取消正在运行的子代理
- “恢复”继续推进
- 子代理详情里可以追加指令或取消

### 审批

默认关闭人机协同（`auto`）。需要人工确认时，在设置里切到 `ask`；也可以使用 `deny` 拒绝需要审批的工具。

破坏性命令不会因为自动模式而静默执行。

## 6. 界面说明

- **仪表盘**：项目、任务、漏洞、工具调用、审批、资产态势
- **项目管理**：新建、编辑、搜索、删除项目
- **任务/对话**：主代理对话、停止/恢复、插话
- **探索链路**：力导向图，显示目标、意图、事实、漏洞、提示、攻击路径；资产不进入主图
- **资产**：资产列表、类型筛选、详情
- **漏洞**：严重度、状态、筛选、复现步骤、证据
- **证据**：正文预览、来源、哈希
- **报告**：Markdown 预览、复制、下载
- **子代理**：任务列表、状态、实时输出、结果、inbox、取消
- **状态**：目标、约束、覆盖、审计
- **MCP**：服务器增删改、启停、测试连接、工具清单
- **技能**：查看、新建、编辑 `SKILL.md`
- **智能体**：模式、主代理提示词、子代理角色/工具/方法论
- **人机协同**：审批列表
- **系统设置**：模型端点、运行限制、MCP、技能、账户

## 7. MCP 与技能

### MCP

支持 stdio 和 Streamable HTTP。JSON 配置示例：

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

工具会自动注册为 `mcp__server__tool`。测试连接只验证草稿，不会改变当前连接。

### 技能

默认扫描启动目录下的 `skills/`，每个技能一个目录和 `SKILL.md`：

```text
skills/
  ssrf-testing/
    SKILL.md
```

批量导入：把技能目录复制到技能根目录，然后在设置里保存或重启。

## 8. 数据与备份

默认数据在启动目录：

| 路径 | 内容 |
|---|---|
| `config.json` | 通道、审批、运行限制、MCP、技能、账户 |
| `data/lingeyebot.sqlite` | 项目、任务、节点、资产、证据索引、会话、审计 |
| `data/evidence/blobs/` | 证据正文 |
| `workspace/` | 工具工作目录 |
| `skills/` | 全局技能 |

备份整个运行目录即可。SQLite 建议停止进程后复制，或使用 SQLite 在线备份。

删除项目会清理项目下的任务、消息、节点、证据、审批、子代理、黑板和记忆；资产本身保留。默认项目不能删除。

## 9. 安全边界与已知限制

- 只对已获得明确授权的目标使用
- 默认只监听 `127.0.0.1`，不要裸露到不可信网络
- 单用户、本地优先，没有多用户/RBAC
- 仅提供 Web UI，不含 CLI/TUI
- MCP 支持 stdio / Streamable HTTP；OAuth、sampling 等高级能力未内置
- 技能没有文件监听，也没有 ZIP 一键导入
- 不同模型网关对推理字段的兼容性不同
- 自动继续会消耗 token；默认不限制轮数，可随时停止
- LLM 输出、工具调用和报告必须由专业人员复核
- 删除不可恢复

## 10. 常见问题

**模型 400 / 参数错误**

检查网关是否支持 `reasoning_effort`、`thinking`、`max_completion_tokens`；按模型文档调整通道配置。

**没有可用 LLM**

到设置里添加端点，确认 Base URL、模型名、API Key；运行任务时选择该通道。

**MCP 工具没有出现**

确认服务器已启用、保存后重连成功；测试连接只验证草稿。

**技能没有加载**

确认目录下有 `SKILL.md`，frontmatter 至少有 `name` 和 `description`；保存设置或重启。

**页面是占位页**

用 `scripts/build.sh` 构建前端，或把 `web/dist` 放到 `internal/server/dist` 后重新编译。

## 11. 版本

当前版本：**1.0.1**

问题与建议请提交到仓库 Issues。使用前请再次确认授权范围。

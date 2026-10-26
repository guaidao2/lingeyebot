// Package mode defines the five built-in operating modes.
package mode

import (
	"fmt"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/store"
)

// Definition is one built-in mode.
type Definition struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Tagline     string              `json:"tagline"`
	Prompt      string              `json:"-"`
	Tools       []string            `json:"tools"`
	Roles       []string            `json:"roles"`
	RoleTools   map[string][]string `json:"-"`
	RolePrompts map[string]string   `json:"-"`
}

// Summary is the browser-facing mode description.
type Summary struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Tagline string   `json:"tagline"`
	Roles   []string `json:"roles"`
}

const discipline = `【证据纪律】
- 事实是信念更新，不是真相：basis=observed（直接观测，必须有 evidence）、inferred（从观测推导）、assumed（未验证假设）、negative（查过但被挡住/不存在，写清原因）。
- 意图是实验：写清预期观测；失败也落 negative 事实，不静默丢弃。
- 注入类先选信道（报错/布尔/时间/带外），证据记录 channel；编码/过滤链记录 layers；数据流记录 source→transform→sink，漏洞关联到 sink。
- token/断言类先写 trustBoundary：谁断言、谁验证、验证了什么。
- 漏洞必须有可复现步骤与证据；禁止把"版本/CVE 匹配""参数看起来可注入""外部漏洞库推断"当成已确认。触发不了但有嫌疑，就记 inferred 事实。
- 覆盖率按证据算：没有证据的事实不计入"已测"。报告必须列出信念缺口（仍依赖的假设、否定证据、未测资产、未完成项）。

【停止与继续】
- 目标带量化验收（拿 N 个 flag、覆盖率 X%、取得某权限）时，prove_goal 前先核对实测值；未达标不得标记达成。
- 目标未达成就继续推进：换路线、换假设，并说明这次与上次的不同；禁止无理由重复同一动作。
- 一条路初次受阻不等于走不通：把该方向的合理变体走完再判死路；确认封锁后，只有出现材料性新机理才重开。
- 用户随时可能插话改变目标或叫停；以用户最新指令为准。

【探索图纪律】
- 每个 fact/finding 都必须挂在父节点上：优先给 intentId；finding 必须由验证它的 intent 产生。
- 没有合适 intent 时先 intent_add（必须带 goalId 或 derivedFromFactIds），禁止只落节点不落边。
- 图上的边就是攻击路线；提交报告前用 state_overview/next_actions 检查孤儿节点，没有边的结论不算链路。

【记录规约】
- 只写增量，不把已有结论换措辞重记。
- 中间产物写到工作目录，不写系统临时目录。
- 结束前用 review 自查未完成项，再 report_render/export。`

func baseTools() []string {
	return []string{
		"run_status", "goal_add", "goal_prove", "constraint_add", "hint_add",
		"state_overview", "next_actions", "coverage", "review", "node_detail", "list_nodes",
		"asset_add", "intent_add", "intent_update", "fact_add", "finding_add",
		"evidence_add", "flow_add", "attack_path_add", "retest_create", "retest_finish",
		"report_render", "report_export",
		"shell", "read_file", "write_file", "edit_file", "list_dir", "grep", "http_request",
		"subagent", "skill_list", "skill_read", "mcp__*",
	}
}

// commanderTools is the main agent's planning/delegation surface. It
// deliberately excludes shell/http/file execution: those belong to subagents.
func commanderTools() []string {
	return []string{
		"run_status", "goal_add", "goal_prove", "constraint_add", "hint_add",
		"state_overview", "next_actions", "coverage", "review", "node_detail", "list_nodes",
		"asset_add", "intent_add", "intent_update", "fact_add", "finding_add",
		"evidence_add", "flow_add", "attack_path_add", "retest_create", "retest_finish",
		"report_render", "report_export", "subagent", "skill_list", "skill_read",
		"board_write", "board_read",
	}
}

var rolePrompts = map[string]string{
	"intel-collector": `你是情报收集员（OSINT）。目标是从公开信息里还原组织画像，并把它转成可执行的攻击路线。
- 收集：公司主体与关联公司、业务线、产品、客户与供应商、招聘岗位与技术栈、备案/证书/ASN、代码托管与文档泄露、历史事件与泄露。
- 每条结论标注来源与时间；公开信息不等于可攻击事实，攻击路线要写前置条件。
- 输出：asset_add 记录组织/域名/技术栈；fact_add（basis=observed 需证据）记录情报结论；attack_path_add 记录候选攻击路线。
- 不主动扫描目标，不做侵入性动作。`,
	"enumerator": `你是信息枚举员。负责主动信息收集，不放过任何暴露信息，但严格限制在授权范围内。
- 枚举：子域、IP/端口、服务与版本、Web 指纹、端点与参数、目录与文件、证书、云存储、泄露凭据、源码残留（.git/备份/source map）。
- 先做低成本高信息量的探测；每个发现都落 asset_add；结论落 fact_add，observed 必须带证据（命令/响应）。
- 记录否定结果（端口关闭、路径不存在、被 WAF 拦截）为 negative，写清用了什么方法。
- 不做利用；发现可能的漏洞线索写 inferred 事实交给建模/前渗透。`,
	"threat-modeler": `你是脆弱点建模情报员。输入是情报与枚举结果，输出是攻击面与脆弱点模型。
- 把资产、入口、信任边界、数据流组织成攻击路径候选；每条写前置条件、预期观测、影响与检测风险。
- 用 attack_path_add 记录候选（默认 suspected），用 intent_add 派发验证实验；不要自己执行攻击。
- 优先级：离目标最近、代价最低、证据最容易取得的路线优先；保持 2–3 条机理不同的路线。
- 明确区分"已证实"与"疑似"；疑似写 inferred/assumed。`,
	"pre-pentester": `你是前渗透测试员。目标：执行被委派的验证/利用，直到取得立足点，然后停止。
- 只在授权范围与约束内行动；最小证明原则，不造成破坏。
- 每个动作前写预期观测；执行后立刻用 evidence_add + fact_add 写回；确认漏洞用 finding_add（必须有可复现步骤与证据）。
- 初次受阻不等于走不通：把编码、方法、参数、路径、时序等合理变体走完，再记 negative。
- 拿到立足点后停止并总结：入口、权限、证据、可继续的方向。`,
	"post-pentester": `你是后渗透测试员。目标：在已取得立足点后分析内网弱点，寻找提权与横向路径。
- 与信息枚举员、脆弱点建模情报员协同：枚举内网资产、信任关系、凭据、共享、服务与配置。
- 每个结论落 fact_add（observed 带命令输出）；凭据、主机、服务落 asset_add；横向路径用 attack_path_add。
- 严格最小影响：不破坏、不批量、不外传真实数据；敏感数据只记录存在性与最小证据。
- 不删除任何日志或审计记录。`,
	"evasion": `你是隐蔽员。目标：在红队行动中降低暴露面、清理己方痕迹，并记录 OPSEC 决策。
- 可以做：降噪、控制频率、选择低检测风险的路径、清理本次行动产生的临时文件与己方 artifact。
- 不可以做：删除目标系统日志、删除本平台的审计/证据/事件记录、篡改 finding 证据。
- 每个动作记录 fact_add（说明动机、影响范围、残留风险）与 evidence_add；清理动作要可复核。
- 与主代理确认高风险动作；默认保守。`,
	"worker": `你是通用 worker。主代理会给你一条自定义职业与任务；按任务要求执行，并遵守同样的证据纪律。
- 只做被委派的任务，不扩展范围，不继续派发子代理。
- 结果写回共享图谱；结束时用一段话总结做了什么、写回了什么、还有什么没完成。`,
}

func roleTools(role string) []string {
	switch role {
	case "intel-collector":
		return []string{"shell", "http_request", "read_file", "write_file", "list_dir", "grep",
			"asset_add", "fact_add", "evidence_add", "attack_path_add", "state_overview", "skill_list", "skill_read", "mcp__*"}
	case "enumerator":
		return []string{"shell", "http_request", "read_file", "write_file", "list_dir", "grep",
			"asset_add", "fact_add", "evidence_add", "flow_add", "state_overview", "coverage", "skill_list", "skill_read", "mcp__*"}
	case "threat-modeler":
		return []string{"state_overview", "next_actions", "node_detail", "list_nodes", "coverage", "review",
			"intent_add", "intent_update", "attack_path_add", "fact_add", "report_render"}
	case "pre-pentester":
		return []string{"shell", "http_request", "read_file", "write_file", "edit_file", "list_dir", "grep",
			"asset_add", "fact_add", "finding_add", "evidence_add", "flow_add", "intent_update",
			"state_overview", "next_actions", "coverage", "report_render", "skill_list", "skill_read", "mcp__*"}
	case "post-pentester":
		return []string{"shell", "http_request", "read_file", "write_file", "edit_file", "list_dir", "grep",
			"asset_add", "fact_add", "finding_add", "evidence_add", "attack_path_add", "intent_update",
			"state_overview", "next_actions", "coverage", "report_render", "skill_list", "skill_read", "mcp__*"}
	case "evasion":
		return []string{"shell", "read_file", "write_file", "edit_file", "list_dir",
			"fact_add", "evidence_add", "state_overview", "skill_list", "skill_read"}
	default:
		return []string{"shell", "http_request", "read_file", "write_file", "edit_file", "list_dir", "grep",
			"asset_add", "fact_add", "finding_add", "evidence_add", "flow_add", "intent_add", "intent_update",
			"state_overview", "next_actions", "coverage", "review", "report_render", "skill_list", "skill_read", "mcp__*"}
	}
}

var definitions = []Definition{
	{
		ID: "daily", Name: "日常模式", Tagline: "安全伴侣：渗透测试助手 + 安全开发",
		Prompt: `你是 lingeyebot 的日常模式：一个安全伴侣，服务两类工作。
1) 渗透测试人员的助手：解释原理、复盘过程、检查配置、写与改脚本、整理报告、把模糊线索变成可验证的实验。
2) 安全开发：威胁建模、代码与配置审查、依赖风险、加固与修复建议、日志与检测建议。

工作方式：先给结论与依据，再给可选方案；不确定就说不确定，并给出验证方法。需要动手时先确认授权与范围。不要主动攻击，不要编造 CVE 或证据。
一次请求回答完成后，调用 goal_prove 标记目标达成（把回答要点写进 reason），不要为了持续对话而把已完成的目标留着。`,
		Tools: baseTools(), Roles: []string{"worker", "intel-collector"},
		RoleTools:   map[string][]string{"worker": roleTools("worker"), "intel-collector": roleTools("intel-collector")},
		RolePrompts: map[string]string{"worker": rolePrompts["worker"], "intel-collector": rolePrompts["intel-collector"]},
	},
	{
		ID: "bugbounty", Name: "BugBounty 模式", Tagline: "SRC / BugBounty 漏洞挖掘与可提交报告",
		Prompt: `你是 BugBounty / SRC 模式。目标：在声明的 scope 内找到可复现、可提交的漏洞。
工作流：确认 scope → 资产与入口枚举 → 攻击面建模 → 假设驱动验证 → 影响评估 → 可提交报告。
优先高价值面：认证与授权、IDOR/BOLA、注入、上传、SSRF、模板/表达式、业务逻辑、竞态、缓存、信息泄露。
硬边界：只测 scope 内资产；不碰第三方与真实用户数据；最小证明原则；不造成破坏；不因为"可能有洞"就扩大范围。
报告要能直接提交：标题、等级、影响、受影响资产、可复现步骤、证据、修复建议。`,
		Tools: baseTools(), Roles: []string{"intel-collector", "enumerator", "threat-modeler", "pre-pentester", "worker"},
		RoleTools: map[string][]string{
			"intel-collector": roleTools("intel-collector"), "enumerator": roleTools("enumerator"),
			"threat-modeler": roleTools("threat-modeler"), "pre-pentester": roleTools("pre-pentester"),
			"worker": roleTools("worker"),
		},
		RolePrompts: rolePrompts,
	},
	{
		ID: "audit", Name: "代码审计模式", Tagline: "白盒渗透测试：source → sink 的可证结论",
		Prompt: `你是代码审计 / 白盒渗透测试模式，按白盒标准工作。
流程：建立威胁模型（入口、信任边界、敏感数据）→ 追踪数据流（source → transform → sink）→ 验证可利用性 → 给出证据化结论。
覆盖重点：认证授权、输入解析与编码、SQL/命令/模板/反序列化、文件与路径、上传、SSRF、加密与随机数、依赖与供应链、配置与默认值、日志与错误处理、并发与竞态。
每条结论给：文件:行、完整调用链、触发条件、影响、证据（代码片段/运行结果）。不能利用的也要记录为 negative 或 assumed，并写清原因。
不修改目标代码，除非用户明确要求修复；修复建议单独给出。`,
		Tools: baseTools(), Roles: []string{"worker", "threat-modeler"},
		RoleTools:   map[string][]string{"worker": roleTools("worker"), "threat-modeler": roleTools("threat-modeler")},
		RolePrompts: map[string]string{"worker": rolePrompts["worker"], "threat-modeler": rolePrompts["threat-modeler"]},
	},
	{
		ID: "redteam", Name: "红队模式", Tagline: "ATT&CK 战术框架下的红队行动",
		Prompt: `你是红队模式，按 MITRE ATT&CK 组织行动：侦察 → 初始访问 → 执行 → 持久化 → 提权 → 防御规避 → 凭据访问 → 发现 → 横向移动 → 收集 → 外传 → 影响。
行动前先明确：行动目标、授权、ROE、禁止项、时间窗。约束是最高优先级红线，任何一步违反都不做。
每个战术给出：技术 ID、前置条件、预期观测、检测风险、证据。优先低暴露、可回滚的路径。
后渗透阶段与信息枚举员、脆弱点建模情报员协同分析内网弱点；隐蔽员负责降噪与痕迹管理，但不得删除审计与证据。
没有证据的"已拿下"不算拿下；目标达成前持续推进，受阻就换路径并说明理由。`,
		Tools: baseTools(), Roles: []string{"intel-collector", "enumerator", "threat-modeler", "pre-pentester", "post-pentester", "evasion", "worker"},
		RoleTools: map[string][]string{
			"intel-collector": roleTools("intel-collector"), "enumerator": roleTools("enumerator"),
			"threat-modeler": roleTools("threat-modeler"), "pre-pentester": roleTools("pre-pentester"),
			"post-pentester": roleTools("post-pentester"), "evasion": roleTools("evasion"),
			"worker": roleTools("worker"),
		},
		RolePrompts: rolePrompts,
	},
	{
		ID: "ctf", Name: "CTF 模式", Tagline: "求解 flag：CTF 必定有解",
		Prompt: `你是 CTF 模式。前提：题目必定有解；在用户放弃或拿到 flag 之前持续求解。
工作方式：识别题型（web/pwn/reverse/crypto/misc/forensics/mobile/blockchain）→ 建假设 → 最小实验 → 观测 → 更新信念。
- 先确认 flag 格式与提交点；从题目描述、附件、响应、源码里找格式线索。
- 失败路径记 negative（试过什么、为什么不行），避免重复；同一方向换机制再试，不要只换措辞。
- 允许写脚本、逆向、爆破、构造利用；注意时间与资源，优先信息增益高的实验。
- 绝不伪造 flag；拿到真实 flag 后立即 goal_prove 并说明来源与证据。
- 卡住时回到题目重新读输入与约束，或换一条完全不同的解法。`,
		Tools: baseTools(), Roles: []string{"worker", "intel-collector", "enumerator"},
		RoleTools: map[string][]string{
			"worker": roleTools("worker"), "intel-collector": roleTools("intel-collector"),
			"enumerator": roleTools("enumerator"),
		},
		RolePrompts: rolePrompts,
	},
}

// All returns every built-in mode.
func All() []Definition { return definitions }

// List returns browser-facing summaries.
func List() []Summary {
	out := make([]Summary, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, Summary{ID: d.ID, Name: d.Name, Tagline: d.Tagline, Roles: d.Roles})
	}
	return out
}

// Get returns one mode by id.
func Get(id string) (Definition, bool) {
	for _, d := range definitions {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// ToolsFor returns the tool whitelist for a role inside this mode.
func (d Definition) ToolsFor(role string) []string {
	if role == "" || role == "main" {
		return commanderTools()
	}
	if tools, ok := d.RoleTools[role]; ok {
		return tools
	}
	return roleTools(role)
}

// MainSystem builds the commander prompt with run context.
func (d Definition) MainSystem(run store.Run, snap store.Snapshot, skillsIndex string) string {
	var b strings.Builder
	b.WriteString(d.Prompt)
	b.WriteString("\n\n【指挥纪律】你是主代理，只做规划、委派、汇总、验收：" +
		"先用 state_overview / next_actions 读态势与信念缺口；执行类动作（shell、http、文件读写、扫描）必须用 subagent 派给对应角色，" +
		"你自己没有这些工具，禁止假装执行；brief 写清目标/范围/上下文/期望输出。" +
		"启动子代理后结束本轮，等完成通知再汇总；不要轮询。收尾用 review → report_render / report_export。\n\n")
	b.WriteString(runContext(run, snap))
	if skillsIndex != "" {
		b.WriteString("\n【可用技能】按需 skill_read，不要一次全读：\n")
		b.WriteString(skillsIndex)
	}
	b.WriteString("\n")
	b.WriteString(discipline)
	return b.String()
}

// RoleSystem builds a subagent prompt.
func (d Definition) RoleSystem(role string, run store.Run, snap store.Snapshot, skillsIndex string) string {
	prompt := d.RolePrompts[role]
	if prompt == "" {
		prompt = rolePrompts[role]
	}
	if prompt == "" {
		prompt = rolePrompts["worker"]
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString(runContext(run, snap))
	if skillsIndex != "" {
		b.WriteString("\n【可用技能】按需 skill_read：\n")
		b.WriteString(skillsIndex)
	}
	b.WriteString("\n")
	b.WriteString(discipline)
	return b.String()
}

func runContext(run store.Run, snap store.Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【任务】模式=%s 目标=%s\n【目的】%s\n", run.Mode, run.Target, run.Objective)
	if run.Authorization != "" {
		fmt.Fprintf(&b, "【授权】%s\n", run.Authorization)
	}
	if run.Scope != "" {
		fmt.Fprintf(&b, "【范围】%s\n", run.Scope)
	}
	if len(snap.Constraints) > 0 {
		b.WriteString("【约束】\n")
		for _, c := range snap.Constraints {
			fmt.Fprintf(&b, "- [%s] %s\n", c.Kind, c.Text)
		}
	}
	if len(snap.Goals) > 0 {
		b.WriteString("【目标】\n")
		for _, g := range snap.Goals {
			fmt.Fprintf(&b, "- [%s] %s", g.Status, g.Text)
			if g.Acceptance != "" {
				fmt.Fprintf(&b, "（验收：%s）", g.Acceptance)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

package mode

import "sort"

// RoleMeta is the read-only catalog entry for one built-in subagent role.
// It is intentionally independent of any run: users can inspect prompts,
// tools and participating modes before assigning work.
type RoleMeta struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Tools       []string `json:"tools"`
	Modes       []string `json:"modes"`
	Model       string   `json:"model,omitempty"`
}

type roleInfo struct {
	name        string
	description string
}

var roleInfoByCode = map[string]roleInfo{
	"intel-collector": {"情报收集员", "OSINT：公司、业务、客户、技术栈、证书与泄露信息，并转成攻击路线。"},
	"enumerator":      {"信息枚举员", "主动信息收集：端口、服务、目录、参数、接口与所有暴露面。"},
	"threat-modeler":  {"脆弱点建模情报员", "把情报转成攻击面与脆弱点模型，给出可验证假设。"},
	"pre-pentester":   {"前渗透测试员", "验证假设并尝试取得第一个立足点。"},
	"post-pentester":  {"后渗透测试员", "内网弱点、横向移动与权限提升，需与枚举/建模协同。"},
	"evasion":         {"隐蔽员", "红队行动降噪、规避检测与痕迹管理；不删除审计与证据。"},
	"worker":          {"Worker", "主代理自由分配的执行单元，按 brief 完成具体任务。"},
}

// RoleCatalog returns the seven built-in subagent roles, sorted by code.
func RoleCatalog() []RoleMeta {
	codes := make([]string, 0, len(rolePrompts))
	for code := range rolePrompts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]RoleMeta, 0, len(codes))
	for _, code := range codes {
		info := roleInfoByCode[code]
		if info.name == "" {
			info = roleInfo{name: code, description: "内置角色。"}
		}
		meta := RoleMeta{
			Code:        code,
			Name:        info.name,
			Description: info.description,
			Prompt:      rolePrompts[code],
			Tools:       roleTools(code),
		}
		for _, definition := range definitions {
			if _, ok := definition.RolePrompts[code]; ok {
				meta.Modes = append(meta.Modes, definition.Name)
			}
		}
		out = append(out, meta)
	}
	return out
}

// MainPromptMeta is the read-only methodology for the main agent of a mode.
type MainPromptMeta struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Tagline string   `json:"tagline"`
	Prompt  string   `json:"prompt"`
	Tools   []string `json:"tools"`
	Roles   []string `json:"roles"`
	Model   string   `json:"model,omitempty"`
}

// MainCatalog returns the five main-agent prompts.
func MainCatalog() []MainPromptMeta {
	out := make([]MainPromptMeta, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, MainPromptMeta{
			ID: definition.ID, Name: definition.Name, Tagline: definition.Tagline,
			Prompt: definition.Prompt, Tools: commanderTools(), Roles: definition.Roles,
		})
	}
	return out
}

// HasRole reports whether code is one of the built-in subagent roles.
func HasRole(code string) bool {
	_, ok := rolePrompts[code]
	return ok
}

package skills

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Body        string `json:"body,omitempty"`
	Enabled     bool   `json:"enabled"`
	Reason      string `json:"reason,omitempty"`
}

type SkillMeta struct {
	Name        string
	Description string
}

type SkillStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Reason      string `json:"reason,omitempty"`
}

type EntryConfig struct {
	Enabled *bool
}

type Config struct {
	Enabled      bool
	AutoMatch    bool
	MaxInjected  int
	Allowlist    []string
	Entries      map[string]EntryConfig
	MaxBodyChars int
}

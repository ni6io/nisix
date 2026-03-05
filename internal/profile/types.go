package profile

import "time"

type UpdateMode string

const (
	UpdateModeReplace UpdateMode = "replace"
	UpdateModeAppend  UpdateMode = "append"
	UpdateModePatch   UpdateMode = "patch"
)

type Config struct {
	UpdateMode        string
	AutoDetectEnabled bool
	AllowedFiles      []string
	MaxFileBytes      int
	ProposalTTL       time.Duration
}

type GetResult struct {
	File      string    `json:"file"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updatedAt"`
	Writable  bool      `json:"writable"`
}

type UpdateRequest struct {
	File    string
	Content string
	Mode    UpdateMode
	Reason  string
}

type UpdateResult struct {
	OK        bool      `json:"ok"`
	File      string    `json:"file"`
	UpdatedAt time.Time `json:"updatedAt"`
	Bytes     int       `json:"bytes"`
}

type Proposal struct {
	ID         string        `json:"id"`
	SessionKey string        `json:"sessionKey"`
	Request    UpdateRequest `json:"request"`
	Summary    string        `json:"summary"`
	CreatedAt  time.Time     `json:"createdAt"`
	ExpiresAt  time.Time     `json:"expiresAt"`
}

type CommandKind string

const (
	CommandNone          CommandKind = ""
	CommandProfileList   CommandKind = "profile_list"
	CommandProfileShow   CommandKind = "profile_show"
	CommandProfileDiff   CommandKind = "profile_diff"
	CommandProfileSet    CommandKind = "profile_set"
	CommandProfileAppend CommandKind = "profile_append"
	CommandProfileApply  CommandKind = "profile_apply"
	CommandOnboardStatus CommandKind = "onboard_status"
	CommandOnboardDone   CommandKind = "onboard_done"
)

type Command struct {
	Kind    CommandKind
	File    string
	Content string
	ID      string
}

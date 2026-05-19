package protocol

import "time"

type SessionState string

const (
	StateWaiting SessionState = "waiting"
	StateActive  SessionState = "active"
	StateExpired SessionState = "expired"
	StateKilled  SessionState = "killed"
)

type TargetSnapshot struct {
	User   string     `json:"user"`
	Host   string     `json:"host"`
	OS     string     `json:"os"`
	Arch   string     `json:"arch"`
	System SystemInfo `json:"system"`
	Mode   string     `json:"mode"`
}

type Session struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Secret           string          `json:"secret"`
	OwnerWorkspaceID string          `json:"owner_workspace_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	LastActivity     time.Time       `json:"last_activity"`
	State            SessionState    `json:"state"`
	LastTarget       *TargetSnapshot `json:"last_target,omitempty"`
}

type Workspace struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkspaceBrowserToken is the HttpOnly hub cookie credential (not for rsctl).
type WorkspaceBrowserToken struct {
	Token       string    `json:"token"`
	WorkspaceID string    `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type SystemInfo struct {
	Hostname  string `json:"hostname,omitempty"`
	OSName    string `json:"os_name,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	Kernel    string `json:"kernel,omitempty"`
	Arch      string `json:"arch,omitempty"`
	IDLike    string `json:"id_like,omitempty"`
}

type Capabilities struct {
	PTY        bool     `json:"pty"`
	Shells     []string `json:"shells,omitempty"`
	Interps    []string `json:"interpreters,omitempty"`
	Transports []string `json:"transports,omitempty"`
}

type Target struct {
	ID           string       `json:"id"`
	SessionID    string       `json:"session_id"`
	Host         string       `json:"host"`
	User         string       `json:"user"`
	OS           string       `json:"os"`
	Arch         string       `json:"arch"`
	System       SystemInfo   `json:"system"`
	Capabilities Capabilities `json:"capabilities"`
	Mode         string       `json:"mode"`
	Transport    string       `json:"transport"`
	LastSeen     time.Time    `json:"last_seen"`
}

type BrowserToken struct {
	Token     string    `json:"token"`
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type OperatorCLIToken struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MsgType string

const (
	MsgRegister  MsgType = "register"
	MsgCommand   MsgType = "command"
	MsgResult    MsgType = "result"
	MsgResize    MsgType = "resize"
	MsgData      MsgType = "data"
	MsgHeartbeat MsgType = "heartbeat"
	MsgEvent     MsgType = "event"
)

type Message struct {
	Type     MsgType `json:"type"`
	TargetID string  `json:"target_id,omitempty"`
	JobID    int64   `json:"job_id,omitempty"`
	Data     []byte  `json:"data,omitempty"`
	Meta     any     `json:"meta,omitempty"`
}

type RegisterPayload struct {
	Host         string       `json:"host"`
	User         string       `json:"user"`
	OS           string       `json:"os"`
	Arch         string       `json:"arch"`
	System       SystemInfo   `json:"system"`
	Capabilities Capabilities `json:"capabilities"`
}

type CommandResult struct {
	JobID    int64  `json:"job_id"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type ResizePayload struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

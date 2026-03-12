package backend

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordSalt string `json:"password_salt"`
	PasswordHash string `json:"password_hash"`
	IsAdmin      bool   `json:"is_admin"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Session struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	ExpiresAt string `json:"expires_at"`
}

type PasswordResetToken struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	ExpiresAt string `json:"expires_at"`
}

type DeviceStatus struct {
	Hostname      string  `json:"hostname"`
	SystemVersion string  `json:"system_version"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	LocalIP       string  `json:"local_ip"`
	ExternalIP    string  `json:"external_ip"`
	MAC           string  `json:"mac"`
	CPUCount      int     `json:"cpu_count"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkOK     bool    `json:"network_ok"`
}

type Device struct {
	DeviceID            string       `json:"device_id"`
	OwnerUserID         string       `json:"owner_user_id"`
	Remark              string       `json:"remark"`
	BoundAt             string       `json:"bound_at"`
	LastSeenAt          string       `json:"last_seen_at"`
	SyncIntervalSeconds int          `json:"sync_interval_seconds"`
	Status              DeviceStatus `json:"status"`
	OpenClawJSON        string       `json:"openclaw_json"`
	OpenClawHash        string       `json:"openclaw_hash"`
	PendingOpenClawJSON string       `json:"pending_openclaw_json"`
	PendingOpenClawHash string       `json:"pending_openclaw_hash"`
}

type Settings struct {
	RegistrationEnabled bool `json:"registration_enabled"`
}

type State struct {
	Settings       Settings             `json:"settings"`
	Users          []User               `json:"users"`
	Sessions       []Session            `json:"sessions"`
	PasswordResets []PasswordResetToken `json:"password_resets"`
	Devices        []Device             `json:"devices"`
}

type PublicUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"is_admin"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	DeviceCount int    `json:"device_count"`
}

type PublicDevice struct {
	DeviceID            string       `json:"device_id"`
	OwnerUserID         string       `json:"owner_user_id"`
	OwnerUsername       string       `json:"owner_username"`
	Remark              string       `json:"remark"`
	BoundAt             string       `json:"bound_at"`
	LastSeenAt          string       `json:"last_seen_at"`
	SyncIntervalSeconds int          `json:"sync_interval_seconds"`
	Online              bool         `json:"online"`
	Status              DeviceStatus `json:"status"`
	OpenClawJSON        string       `json:"openclaw_json"`
	PendingOpenClawJSON string       `json:"pending_openclaw_json"`
}

type Summary struct {
	UserCount           int  `json:"user_count"`
	DeviceCount         int  `json:"device_count"`
	OnlineDeviceCount   int  `json:"online_device_count"`
	RegistrationEnabled bool `json:"registration_enabled"`
}

type ClientHeartbeatRequest struct {
	DeviceID            string  `json:"device_id"`
	Hostname            string  `json:"hostname"`
	SystemVersion       string  `json:"system_version"`
	OS                  string  `json:"os"`
	Arch                string  `json:"arch"`
	LocalIP             string  `json:"local_ip"`
	MAC                 string  `json:"mac"`
	CPUCount            int     `json:"cpu_count"`
	CPUPercent          float64 `json:"cpu_percent"`
	MemoryPercent       float64 `json:"memory_percent"`
	NetworkOK           bool    `json:"network_ok"`
	OpenClawJSON        string  `json:"openclaw_json"`
	OpenClawHash        string  `json:"openclaw_hash"`
	SyncIntervalSeconds int     `json:"sync_interval_seconds"`
}

type ClientHeartbeatResponse struct {
	ServerTime      string `json:"server_time"`
	ApplyConfig     bool   `json:"apply_config"`
	OpenClawJSON    string `json:"openclaw_json"`
	RestartOpenClaw bool   `json:"restart_openclaw"`
	Message         string `json:"message"`
}

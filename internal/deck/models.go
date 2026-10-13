package deck

import (
	"encoding/json"
	"time"
)

const Version = 1

type Settings struct {
	WebDAVURL      string `json:"webdav_url"`
	WebDAVUsername string `json:"webdav_username"`
	WebDAVPassword string `json:"webdav_password"`
}

type Tokens struct {
	IDToken       string          `json:"id_token"`
	AccessToken   string          `json:"access_token"`
	RefreshToken  string          `json:"refresh_token"`
	AccountID     string          `json:"account_id"`
	ExpiresAt     time.Time       `json:"expires_at"`
	LastRefresh   time.Time       `json:"last_refresh"`
	OAuthResponse json.RawMessage `json:"oauth_response"`
}

type Window struct {
	RemainingPercent float64    `json:"remaining_percent"`
	ResetAt          *time.Time `json:"reset_at"`
}

type Usage struct {
	Plan         string    `json:"plan"`
	FiveHour     *Window   `json:"five_hour"`
	Weekly       *Window   `json:"weekly"`
	ResetCredits *int      `json:"reset_credits"`
	QueriedAt    time.Time `json:"queried_at"`
}

type Credit struct {
	ID          string  `json:"id"`
	ResetType   string  `json:"reset_type"`
	Status      string  `json:"status"`
	GrantedAt   string  `json:"granted_at"`
	ExpiresAt   *string `json:"expires_at"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

type Credits struct {
	AvailableCount int       `json:"available_count"`
	Credits        []Credit  `json:"credits"`
	QueriedAt      time.Time `json:"queried_at"`
}

type Account struct {
	ID            string             `json:"id"`
	UserID        string             `json:"user_id"`
	AccountID     string             `json:"account_id"`
	Email         string             `json:"email"`
	Alias         string             `json:"alias"`
	Plan          string             `json:"plan"`
	Notifications QuotaNotifications `json:"notifications"`
	AuthError     string             `json:"auth_error,omitempty"`
	Tokens        Tokens             `json:"tokens"`
	Usage         *Usage             `json:"usage"`
	Credits       *Credits           `json:"credits"`
	PendingReset  *ResetRequest      `json:"pending_reset"`
}

type ResetRequest struct {
	ID       string         `json:"id"`
	CreditID string         `json:"credit_id"`
	Started  bool           `json:"started"`
	Result   *ConsumeResult `json:"result"`
}

type Data struct {
	Window         *WindowState `json:"window,omitempty"`
	Preferences    Preferences  `json:"preferences"`
	Sync           BackupSync   `json:"backup_sync,omitempty"`
	Version        int          `json:"version"`
	Settings       Settings     `json:"settings"`
	Accounts       []Account    `json:"accounts"`
	LastSwitchedID string       `json:"last_switched_id"`
}

// 前端只接收展示字段，账号令牌留在 Go 中。
type AccountView struct {
	ID           string        `json:"id"`
	AccountID    string        `json:"account_id"`
	Email        string        `json:"email"`
	Alias        string        `json:"alias"`
	Plan         string        `json:"plan"`
	AuthError    string        `json:"auth_error"`
	Current      bool          `json:"current"`
	Usage        *Usage        `json:"usage"`
	Credits      *Credits      `json:"credits"`
	UsageError   string        `json:"usage_error"`
	CreditsError string        `json:"credits_error"`
	PendingReset *ResetRequest `json:"pending_reset"`
}

type BackupStatus struct {
	Busy     bool       `json:"busy"`
	Conflict bool       `json:"conflict"`
	At       *time.Time `json:"at"`
	Error    string     `json:"error"`
}

type State struct {
	DesktopError   string        `json:"desktop_error"`
	Preferences    Preferences   `json:"preferences"`
	Accounts       []AccountView `json:"accounts"`
	Settings       Settings      `json:"settings"`
	CLIStore       string        `json:"cli_store"`
	CLIError       string        `json:"cli_error"`
	Backup         BackupStatus  `json:"backup"`
	NextRefreshAt  *time.Time    `json:"next_refresh_at"`
	AutoRefreshing bool          `json:"auto_refreshing"`
}

type BackupSync struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	ETag     string `json:"etag"`
}

type Preferences struct {
	LowQuotaNotify    bool   `json:"low_quota_notify"`
	LowQuotaThreshold int    `json:"low_quota_threshold"`
	Privacy           bool   `json:"privacy"`
	Sort              string `json:"sort"`
}

package tray

type Account struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Current bool   `json:"current"`
	Enabled bool   `json:"enabled"`
}

type Menu struct {
	Current        string    `json:"current"`
	Quota          string    `json:"quota"`
	Updated        string    `json:"updated"`
	Status         string    `json:"status"`
	RefreshEnabled bool      `json:"refresh_enabled"`
	Accounts       []Account `json:"accounts"`
}

type Actions struct {
	Show    func()
	Quit    func()
	Refresh func()
	Switch  func(string)
}

type Controller struct {
	Update func(Menu)
	Stop   func()
}

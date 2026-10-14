// Package sessions reads external Codex rollouts without changing Deck or CLI state.
package sessions

type Issue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

type Summary struct {
	ID              string   `json:"id"`
	ResumeCommand   string   `json:"resume_command,omitempty"`
	Title           string   `json:"title"`
	Project         string   `json:"project"`
	CreatedAt       string   `json:"created_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	Models          []string `json:"models"`
	TurnCount       int      `json:"turn_count"`
	ToolCount       int      `json:"tool_count"`
	Snippet         string   `json:"snippet,omitempty"`
	MatchEventID    string   `json:"match_event_id,omitempty"`
	MatchTextOffset int      `json:"match_text_offset,omitempty"`
}

type Event struct {
	ID         string `json:"id"`
	Turn       int    `json:"turn"`
	Kind       string `json:"kind"`
	Time       string `json:"time,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Text       string `json:"text"`
	TextCursor int    `json:"text_cursor"`
}

type Turn struct {
	Number  int    `json:"number"`
	EventID string `json:"event_id"`
}

type ListQuery struct {
	RequestID uint64 `json:"request_id,omitempty"`
	Search    string `json:"search"`
	Project   string `json:"project"`
	Date      string `json:"date"`
	Cursor    string `json:"cursor"`
}

type ListPage struct {
	Sessions   []Summary `json:"sessions"`
	Projects   []string  `json:"projects"`
	Dates      []string  `json:"dates"`
	Source     string    `json:"source"`
	Missing    bool      `json:"missing"`
	NextCursor string    `json:"next_cursor"`
	Issues     []Issue   `json:"issues"`
}

type ReadQuery struct {
	RequestID uint64 `json:"request_id,omitempty"`
	ID        string `json:"id"`
	Cursor    string `json:"cursor"`
	EventID   string `json:"event_id"`
	Filter    string `json:"filter"`
}

type ReadPage struct {
	Session    Summary `json:"session"`
	Revision   string  `json:"revision"`
	Turns      []Turn  `json:"turns"`
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
	Issues     []Issue `json:"issues"`
}

type Match struct {
	EventID    string `json:"event_id"`
	Turn       int    `json:"turn"`
	Snippet    string `json:"snippet"`
	TextOffset int    `json:"text_offset"`
}

type MatchQuery struct {
	RequestID uint64 `json:"request_id,omitempty"`
	ID        string `json:"id"`
	Search    string `json:"search"`
	Cursor    string `json:"cursor"`
}

type MatchPage struct {
	Matches    []Match `json:"matches"`
	NextCursor string  `json:"next_cursor"`
	Issues     []Issue `json:"issues"`
}

type TextQuery struct {
	ID       string `json:"id"`
	EventID  string `json:"event_id"`
	Revision string `json:"revision"`
	Offset   int    `json:"offset"`
}

type TextPage struct {
	Text       string `json:"text"`
	NextOffset int    `json:"next_offset"`
}

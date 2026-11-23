package model

// GameState represents the global round state (single row id=1 in game_state).
type GameState struct {
	ID             int
	Status         string // "running" or "ended"
	StartedAt      int64
	NextAreaAt     int64
	ForbiddenCount int
	HackFlag       int
	AreaOrder      []int // Shuffled order of 22 area IDs
}

// NewsLog represents a game event / death / announcement entry.
type NewsLog struct {
	ID        int64
	CreatedAt int64
	Kind      string
	Message   string
}

// Winner represents a past round winner record.
type Winner struct {
	ID           int64
	RoundEndedAt int64
	WinnerName   string
	EndingType   string
	Kills        int
	SummaryJSON  string
}

// Session represents an authenticated user session.
type Session struct {
	Token     string
	UserID    string
	CreatedAt int64
	ExpiresAt int64
}

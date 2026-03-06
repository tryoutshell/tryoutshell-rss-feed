package feed

import "time"

type Feed struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	SiteURL      string    `json:"site_url"`
	Description  string    `json:"description"`
	ArticleCount int       `json:"article_count"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type Article struct {
	ID          string    `json:"id"`
	FeedID      string    `json:"feed_id"`
	FeedName    string    `json:"feed_name"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	RawContent  string    `json:"raw_content"`
	CachedPath  string    `json:"cached_path"`
	PublishedAt time.Time `json:"published_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Read        bool      `json:"read"`
	ReadAt      time.Time `json:"read_at"`
}

type ParsedFeed struct {
	Title       string
	Link        string
	Description string
	Entries     []ParsedEntry
}

type ParsedEntry struct {
	ExternalID  string
	Title       string
	Link        string
	Summary     string
	Content     string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

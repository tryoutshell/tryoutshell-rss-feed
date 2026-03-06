package feed

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
)

type Store struct {
	path string
	data storeData
}

type storeData struct {
	Feeds    []Feed    `json:"feeds"`
	Articles []Article `json:"articles"`
}

func Open(path string) (*Store, error) {
	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}

	store := &Store{
		path: path,
		data: storeData{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := store.Save(); err != nil {
				return nil, err
			}
			return store, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return store, nil
	}

	if err := json.Unmarshal(data, &store.data); err != nil {
		return nil, err
	}

	store.recountFeeds()
	return store, nil
}

func (s *Store) Save() error {
	s.recountFeeds()
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) ListFeeds() []Feed {
	feeds := append([]Feed(nil), s.data.Feeds...)
	sort.Slice(feeds, func(i, j int) bool {
		return strings.ToLower(feeds[i].Name) < strings.ToLower(feeds[j].Name)
	})
	return feeds
}

func (s *Store) ListArticles(feedID, query string) []Article {
	query = strings.ToLower(strings.TrimSpace(query))
	var articles []Article
	for _, article := range s.data.Articles {
		if article.FeedID != feedID {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(article.Title + " " + article.Summary)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		articles = append(articles, article)
	}

	sort.Slice(articles, func(i, j int) bool {
		if articles[i].PublishedAt.Equal(articles[j].PublishedAt) {
			return strings.ToLower(articles[i].Title) < strings.ToLower(articles[j].Title)
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})

	return articles
}

func (s *Store) GetFeed(feedID string) (Feed, bool) {
	for _, item := range s.data.Feeds {
		if item.ID == feedID {
			return item, true
		}
	}
	return Feed{}, false
}

func (s *Store) GetArticle(articleID string) (Article, bool) {
	for _, item := range s.data.Articles {
		if item.ID == articleID {
			return item, true
		}
	}
	return Article{}, false
}

func (s *Store) AddFeed(ctx context.Context, feedURL string, maxArticles int) (Feed, int, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return Feed{}, 0, fmt.Errorf("feed url is required")
	}

	for _, existing := range s.data.Feeds {
		if strings.EqualFold(existing.URL, feedURL) {
			count, err := s.RefreshFeed(ctx, existing.ID, maxArticles)
			refreshed, _ := s.GetFeed(existing.ID)
			return refreshed, count, err
		}
	}

	parsed, err := FetchFeed(ctx, feedURL)
	if err != nil {
		return Feed{}, 0, err
	}

	now := time.Now()
	item := Feed{
		ID:          stableID(feedURL),
		Name:        fallback(parsed.Title, feedURL),
		URL:         feedURL,
		SiteURL:     parsed.Link,
		Description: parsed.Description,
		UpdatedAt:   now,
		CreatedAt:   now,
	}

	s.data.Feeds = append(s.data.Feeds, item)
	count := s.mergeArticles(item, parsed.Entries, maxArticles)
	if err := s.Save(); err != nil {
		return Feed{}, 0, err
	}

	return item, count, nil
}

func (s *Store) RefreshFeed(ctx context.Context, feedID string, maxArticles int) (int, error) {
	for index, item := range s.data.Feeds {
		if item.ID != feedID {
			continue
		}

		parsed, err := FetchFeed(ctx, item.URL)
		if err != nil {
			return 0, err
		}

		item.Name = fallback(parsed.Title, item.Name)
		item.SiteURL = firstNonEmpty(parsed.Link, item.SiteURL)
		item.Description = firstNonEmpty(parsed.Description, item.Description)
		item.UpdatedAt = time.Now()
		s.data.Feeds[index] = item

		count := s.mergeArticles(item, parsed.Entries, maxArticles)
		if err := s.Save(); err != nil {
			return 0, err
		}
		return count, nil
	}

	return 0, fmt.Errorf("feed %s not found", feedID)
}

func (s *Store) RefreshAll(ctx context.Context, maxArticles int) (int, error) {
	total := 0
	for _, item := range s.data.Feeds {
		count, err := s.RefreshFeed(ctx, item.ID, maxArticles)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

func (s *Store) RemoveFeed(nameOrID string) error {
	nameOrID = strings.TrimSpace(nameOrID)
	if nameOrID == "" {
		return fmt.Errorf("feed name is required")
	}

	var target Feed
	found := false
	for _, item := range s.data.Feeds {
		if item.ID == nameOrID || strings.EqualFold(item.Name, nameOrID) || strings.EqualFold(item.URL, nameOrID) {
			target = item
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("feed %q not found", nameOrID)
	}

	var feeds []Feed
	for _, item := range s.data.Feeds {
		if item.ID != target.ID {
			feeds = append(feeds, item)
		}
	}
	s.data.Feeds = feeds

	var articles []Article
	for _, article := range s.data.Articles {
		if article.FeedID == target.ID {
			if article.CachedPath != "" {
				_ = os.Remove(article.CachedPath)
			}
			continue
		}
		articles = append(articles, article)
	}
	s.data.Articles = articles

	return s.Save()
}

func (s *Store) ToggleRead(articleID string) (Article, error) {
	for index, item := range s.data.Articles {
		if item.ID != articleID {
			continue
		}
		item.Read = !item.Read
		if item.Read {
			item.ReadAt = time.Now()
		} else {
			item.ReadAt = time.Time{}
		}
		s.data.Articles[index] = item
		return item, s.Save()
	}
	return Article{}, fmt.Errorf("article %s not found", articleID)
}

func (s *Store) MarkAllRead(feedID string) (int, error) {
	count := 0
	for index, item := range s.data.Articles {
		if item.FeedID != feedID || item.Read {
			continue
		}
		item.Read = true
		item.ReadAt = time.Now()
		s.data.Articles[index] = item
		count++
	}
	return count, s.Save()
}

func (s *Store) MarkRead(articleID string) error {
	for index, item := range s.data.Articles {
		if item.ID != articleID || item.Read {
			continue
		}
		item.Read = true
		item.ReadAt = time.Now()
		s.data.Articles[index] = item
		return s.Save()
	}
	return nil
}

func (s *Store) EnsureMarkdown(ctx context.Context, articleID string) (Article, string, error) {
	for index, item := range s.data.Articles {
		if item.ID != articleID {
			continue
		}

		if item.CachedPath != "" {
			data, err := os.ReadFile(item.CachedPath)
			if err == nil {
				return item, string(data), nil
			}
		}

		var title string
		var markdown string
		var err error

		if strings.TrimSpace(item.RawContent) != "" {
			title = item.Title
			markdown = MarkdownFromContent(item.Title, item.RawContent)
		} else if item.URL != "" {
			title, markdown, err = FetchArticleMarkdown(ctx, item.URL)
			if err != nil {
				return Article{}, "", err
			}
		} else {
			title = item.Title
			markdown = MarkdownFromContent(item.Title, item.Summary)
		}

		if strings.TrimSpace(title) != "" {
			item.Title = title
		}

		cachePath := filepath.Join(config.CacheDir(), item.ID+".md")
		if err := os.WriteFile(cachePath, []byte(markdown), 0o644); err != nil {
			return Article{}, "", err
		}

		item.CachedPath = cachePath
		s.data.Articles[index] = item
		if err := s.Save(); err != nil {
			return Article{}, "", err
		}

		return item, markdown, nil
	}

	return Article{}, "", fmt.Errorf("article %s not found", articleID)
}

func (s *Store) mergeArticles(feedItem Feed, entries []ParsedEntry, maxArticles int) int {
	if maxArticles <= 0 {
		maxArticles = 50
	}

	byID := make(map[string]Article, len(s.data.Articles))
	for _, article := range s.data.Articles {
		byID[article.ID] = article
	}

	var merged []Article
	for _, entry := range entries {
		articleID := stableID(feedItem.ID + "::" + firstNonEmpty(entry.ExternalID, entry.Link, entry.Title))
		current, ok := byID[articleID]
		article := Article{
			ID:          articleID,
			FeedID:      feedItem.ID,
			FeedName:    feedItem.Name,
			Title:       fallback(entry.Title, "Untitled"),
			URL:         strings.TrimSpace(entry.Link),
			Summary:     firstNonEmpty(strings.TrimSpace(entry.Summary), strings.TrimSpace(entry.Content)),
			RawContent:  firstNonEmpty(strings.TrimSpace(entry.Content), strings.TrimSpace(entry.Summary)),
			PublishedAt: entry.PublishedAt,
			UpdatedAt:   nonZero(entry.UpdatedAt, time.Now()),
			Read:        false,
		}

		if ok {
			article.Read = current.Read
			article.ReadAt = current.ReadAt
			article.CachedPath = current.CachedPath
			if article.Summary == "" {
				article.Summary = current.Summary
			}
			if article.RawContent == "" {
				article.RawContent = current.RawContent
			}
			if article.URL == "" {
				article.URL = current.URL
			}
			if article.PublishedAt.IsZero() {
				article.PublishedAt = current.PublishedAt
			}
		}

		merged = append(merged, article)
	}

	sort.Slice(merged, func(i, j int) bool {
		return nonZero(merged[i].PublishedAt, merged[i].UpdatedAt).After(nonZero(merged[j].PublishedAt, merged[j].UpdatedAt))
	})

	if len(merged) > maxArticles {
		merged = merged[:maxArticles]
	}

	var remainder []Article
	for _, article := range s.data.Articles {
		if article.FeedID != feedItem.ID {
			remainder = append(remainder, article)
		}
	}

	s.data.Articles = append(remainder, merged...)
	return len(merged)
}

func (s *Store) recountFeeds() {
	counts := map[string]int{}
	for _, article := range s.data.Articles {
		counts[article.FeedID]++
	}
	for index, item := range s.data.Feeds {
		item.ArticleCount = counts[item.ID]
		s.data.Feeds[index] = item
	}
}

func stableID(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func nonZero(primary, fallback time.Time) time.Time {
	if primary.IsZero() {
		return fallback
	}
	return primary
}

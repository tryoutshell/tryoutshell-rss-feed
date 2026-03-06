package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"
)

var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

var httpClient = &http.Client{Timeout: 20 * time.Second}

func FetchFeed(ctx context.Context, url string) (ParsedFeed, error) {
	body, err := fetchURL(ctx, url)
	if err != nil {
		return ParsedFeed{}, err
	}
	return Parse(body)
}

func FetchArticleMarkdown(ctx context.Context, url string) (string, string, error) {
	body, err := fetchURL(ctx, url)
	if err != nil {
		return "", "", err
	}

	html := string(body)
	title := extractTitle(html)
	markdown, err := htmltomd.ConvertString(html)
	if err != nil {
		return title, html, fmt.Errorf("converting article to markdown: %w", err)
	}

	return title, markdown, nil
}

func MarkdownFromContent(title, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "# " + fallback(title, "Untitled")
	}

	if looksLikeHTML(raw) {
		if markdown, err := htmltomd.ConvertString(raw); err == nil && strings.TrimSpace(markdown) != "" {
			return markdown
		}
	}

	if strings.HasPrefix(raw, "#") {
		return raw
	}

	return fmt.Sprintf("# %s\n\n%s", fallback(title, "Untitled"), raw)
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tryoutshell-rss-feed/0.1 (+terminal rss reader)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetching %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}

	return body, nil
}

func extractTitle(html string) string {
	matches := titlePattern.FindStringSubmatch(html)
	if len(matches) != 2 {
		return "Untitled"
	}
	title := strings.TrimSpace(matches[1])
	if title == "" {
		return "Untitled"
	}
	return title
}

func looksLikeHTML(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "<p") || strings.Contains(value, "<div") || strings.Contains(value, "<br") || strings.Contains(value, "<h1") || strings.Contains(value, "<article")
}

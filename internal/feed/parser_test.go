package feed

import "testing"

func TestParseRSS(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Go Blog</title>
    <link>https://go.dev/blog/</link>
    <description>Go articles</description>
    <item>
      <title>Go 1.24 Released</title>
      <link>https://go.dev/blog/go1.24</link>
      <guid>go-1-24</guid>
      <description><![CDATA[<p>New features.</p>]]></description>
      <pubDate>Fri, 06 Mar 2026 10:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`)

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if parsed.Title != "Go Blog" {
		t.Fatalf("unexpected title: %q", parsed.Title)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Entries))
	}
	if parsed.Entries[0].Title != "Go 1.24 Released" {
		t.Fatalf("unexpected entry title: %q", parsed.Entries[0].Title)
	}
	if parsed.Entries[0].Link != "https://go.dev/blog/go1.24" {
		t.Fatalf("unexpected entry link: %q", parsed.Entries[0].Link)
	}
}

func TestParseAtom(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Anthropic Research</title>
  <subtitle>Research posts</subtitle>
  <link href="https://www.anthropic.com/news" rel="alternate"></link>
  <entry>
    <title>Model Context Protocol</title>
    <id>tag:anthropic.com,2026:mcp</id>
    <link href="https://www.anthropic.com/news/model-context-protocol" rel="alternate"></link>
    <summary>Protocol overview.</summary>
    <updated>2026-03-05T10:00:00Z</updated>
  </entry>
</feed>`)

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if parsed.Title != "Anthropic Research" {
		t.Fatalf("unexpected title: %q", parsed.Title)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Entries))
	}
	if parsed.Entries[0].ExternalID != "tag:anthropic.com,2026:mcp" {
		t.Fatalf("unexpected entry id: %q", parsed.Entries[0].ExternalID)
	}
}

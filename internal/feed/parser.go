package feed

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

func Parse(raw []byte) (ParsedFeed, error) {
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(raw, &root); err != nil {
		return ParsedFeed{}, err
	}

	switch strings.ToLower(root.XMLName.Local) {
	case "rss":
		return parseRSS(raw)
	case "feed":
		return parseAtom(raw)
	default:
		return ParsedFeed{}, fmt.Errorf("unsupported feed format: %s", root.XMLName.Local)
	}
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func parseRSS(raw []byte) (ParsedFeed, error) {
	var doc rssDocument
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return ParsedFeed{}, err
	}

	result := ParsedFeed{
		Title:       strings.TrimSpace(doc.Channel.Title),
		Link:        strings.TrimSpace(doc.Channel.Link),
		Description: strings.TrimSpace(doc.Channel.Description),
	}

	for _, item := range doc.Channel.Items {
		result.Entries = append(result.Entries, ParsedEntry{
			ExternalID:  firstNonEmpty(item.GUID, item.Link, item.Title),
			Title:       fallback(item.Title, "Untitled"),
			Link:        strings.TrimSpace(item.Link),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Description),
			PublishedAt: parseTime(item.PubDate),
			UpdatedAt:   parseTime(item.PubDate),
		})
	}

	return result, nil
}

type atomDocument struct {
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Links     []atomLink `xml:"link"`
}

func parseAtom(raw []byte) (ParsedFeed, error) {
	var doc atomDocument
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return ParsedFeed{}, err
	}

	result := ParsedFeed{
		Title:       fallback(doc.Title, "Untitled Feed"),
		Link:        pickAtomLink(doc.Links),
		Description: strings.TrimSpace(doc.Subtitle),
	}

	for _, entry := range doc.Entries {
		result.Entries = append(result.Entries, ParsedEntry{
			ExternalID:  firstNonEmpty(entry.ID, pickAtomLink(entry.Links), entry.Title),
			Title:       fallback(entry.Title, "Untitled"),
			Link:        pickAtomLink(entry.Links),
			Summary:     strings.TrimSpace(entry.Summary),
			Content:     firstNonEmpty(strings.TrimSpace(entry.Content), strings.TrimSpace(entry.Summary)),
			PublishedAt: parseTime(firstNonEmpty(entry.Published, entry.Updated)),
			UpdatedAt:   parseTime(firstNonEmpty(entry.Updated, entry.Published)),
		})
	}

	return result, nil
}

func pickAtomLink(links []atomLink) string {
	for _, link := range links {
		if link.Rel == "alternate" && link.Href != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	for _, link := range links {
		if link.Href != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}

	return time.Time{}
}

func fallback(value, alt string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return alt
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

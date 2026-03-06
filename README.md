# tryoutshell-rss-feed

A terminal-native RSS reader with an integrated AI assistant.

## Commands

```bash
tryoutshell-rss-feed add https://go.dev/blog/feed.atom
tryoutshell-rss-feed feeds
tryoutshell-rss-feed remove "Go Blog"
tryoutshell-rss-feed
```

## Features

- RSS and Atom feed ingestion
- feed list, article list, and split-pane reader
- local JSON-backed state store with cached markdown
- read/unread tracking
- AI chat against the current article
- slash commands for theme, resize, save, summary, toc, and open

## Storage

```text
~/.config/tryoutshell-rss-feed/config.yaml
~/.local/share/tryoutshell-rss-feed/state.json
~/.local/share/tryoutshell-rss-feed/cache/
~/.local/share/tryoutshell-rss-feed/saved/
```

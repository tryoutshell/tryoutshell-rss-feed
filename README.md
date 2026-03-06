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
- mouse wheel scrolling and click-to-focus panes
- focus mode with the AI pane toggled on or off
- local JSON-backed state store with cached markdown
- read/unread tracking
- AI chat against the current article
- slash commands for theme, resize, AI visibility, save, summary, toc, open, and copy

## Reader Controls

- `Ctrl+C` exits globally
- `Tab` switches pane focus
- `i` or `Enter` on the AI pane starts chat input
- `/` opens command mode for slash commands like `/theme`, `/resize 80`, `/ai off`
- `t` cycles themes
- `v` toggles the AI pane for focus reading
- `[` and `]` resize the split when AI is visible
- `y` copies all fenced code blocks from the current article
- mouse wheel scrolls the pane under the cursor
- clicking a pane focuses it

## Storage

```text
~/.config/tryoutshell-rss-feed/config.yaml
~/.local/share/tryoutshell-rss-feed/state.json
~/.local/share/tryoutshell-rss-feed/cache/
~/.local/share/tryoutshell-rss-feed/saved/
```

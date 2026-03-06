package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/ai"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/feed"
)

type screen int
type pane int
type inputMode int

const (
	screenFeeds screen = iota
	screenArticles
	screenReader
)

const (
	paneArticle pane = iota
	paneChat
)

const (
	inputNone inputMode = iota
	inputChat
	inputCommand
)

type feedAddedMsg struct {
	item  feed.Feed
	count int
	err   error
}

type feedsRefreshedMsg struct {
	count int
	err   error
}

type articleOpenedMsg struct {
	article  feed.Article
	markdown string
	err      error
}

type readToggledMsg struct {
	article feed.Article
	err     error
}

type markedAllReadMsg struct {
	count int
	err   error
}

type aiReplyMsg struct {
	content string
	err     error
}

type Model struct {
	cfg   config.Config
	store *feed.Store

	width  int
	height int

	screen       screen
	activePane   pane
	input        textinput.Model
	inputFocused bool
	inputMode    inputMode
	waiting      bool
	showHelp     bool
	showAI       bool
	pendingG     bool

	theme     Theme
	themeName string
	split     int
	status    string

	feedFilter    string
	articleFilter string

	feeds        []feed.Feed
	feedIndex    int
	articles     []feed.Article
	articleIndex int

	currentFeed     feed.Feed
	currentArticle  feed.Article
	articleMarkdown string
	articleViewport viewport.Model
	chatViewport    viewport.Model

	chatHistory []ai.Message
	chatClient  *ai.Client
	suggestions []string
}

func Launch(cfg config.Config, store *feed.Store) error {
	model := NewModel(cfg, store)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}

func NewModel(cfg config.Config, store *feed.Store) *Model {
	input := textinput.New()
	input.Prompt = "│ "
	input.Placeholder = "paste RSS feed URL to add, or search feeds..."
	input.CharLimit = 512

	model := &Model{
		cfg:        cfg,
		store:      store,
		screen:     screenFeeds,
		activePane: paneArticle,
		input:      input,
		themeName:  cfg.Theme,
		theme:      getTheme(cfg.Theme),
		split:      clamp(cfg.DefaultSplit, 50, 90),
		showAI:     true,
		status:     "Press / to focus input, ? for help.",
	}
	model.reloadFeeds()
	return model
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncReaderLayout()
		return m, nil

	case feedAddedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.feedFilter = ""
		m.input.SetValue("")
		m.reloadFeeds()
		m.selectFeed(msg.item.ID)
		m.status = fmt.Sprintf("Added %s with %d articles.", msg.item.Name, msg.count)
		return m, nil

	case feedsRefreshedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.reloadFeeds()
		if m.currentFeed.ID != "" {
			if feedItem, ok := m.store.GetFeed(m.currentFeed.ID); ok {
				m.currentFeed = feedItem
				m.reloadArticles()
			}
		}
		m.status = fmt.Sprintf("Refreshed feeds. %d articles available.", msg.count)
		return m, nil

	case articleOpenedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.currentArticle = msg.article
		m.articleMarkdown = msg.markdown
		m.screen = screenReader
		m.activePane = paneArticle
		m.inputFocused = false
		m.input.Blur()
		m.input.SetValue("")
		m.chatHistory = nil
		m.waiting = false
		m.chatClient = ai.NewClient(msg.markdown)
		m.suggestions = buildSuggestions(msg.article, msg.markdown)
		m.status = "Reading article."
		m.syncReaderLayout()
		return m, nil

	case readToggledMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.reloadArticles()
		if current, ok := m.store.GetArticle(msg.article.ID); ok {
			m.currentArticle = current
		}
		if msg.article.Read {
			m.status = "Marked article as read."
		} else {
			m.status = "Marked article as unread."
		}
		return m, nil

	case markedAllReadMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.reloadArticles()
		m.status = fmt.Sprintf("Marked %d articles as read.", msg.count)
		return m, nil

	case aiReplyMsg:
		m.waiting = false
		if msg.err != nil {
			m.chatHistory = append(m.chatHistory, ai.Message{Role: "assistant", Content: "Error: " + msg.err.Error()})
		} else {
			m.chatHistory = append(m.chatHistory, ai.Message{Role: "assistant", Content: msg.content})
		}
		m.chatViewport.SetContent(m.renderChatContent())
		m.chatViewport.GotoBottom()
		return m, nil

	case tea.MouseMsg:
		if m.showHelp {
			return m, nil
		}
		switch m.screen {
		case screenFeeds:
			return m.handleFeedsMouse(msg)
		case screenArticles:
			return m.handleArticlesMouse(msg)
		case screenReader:
			return m.handleReaderMouse(msg)
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.showHelp = false
			}
			return m, nil
		}

		switch m.screen {
		case screenFeeds:
			return m.handleFeeds(msg)
		case screenArticles:
			return m.handleArticles(msg)
		case screenReader:
			return m.handleReader(msg)
		}
	}

	return m, nil
}

func (m *Model) handleFeeds(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputFocused {
		switch msg.String() {
		case "esc":
			m.inputFocused = false
			m.input.Blur()
			m.status = "Feed input closed."
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if looksLikeURL(value) {
				m.status = fmt.Sprintf("Adding feed %s...", value)
				return m, m.addFeedCmd(value)
			}
			m.feedFilter = value
			m.reloadFeeds()
			m.status = "Feed filter updated."
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.feedFilter = m.input.Value()
		m.reloadFeeds()
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "/", "i":
		m.inputFocused = true
		m.input.Placeholder = "paste RSS feed URL to add, or search feeds..."
		m.input.Focus()
	case "j", "down":
		m.moveFeed(1)
	case "k", "up":
		m.moveFeed(-1)
	case "enter":
		if len(m.feeds) == 0 {
			return m, nil
		}
		m.currentFeed = m.feeds[m.feedIndex]
		m.articleFilter = ""
		m.input.SetValue("")
		m.input.Placeholder = "search articles..."
		m.screen = screenArticles
		m.reloadArticles()
		m.status = fmt.Sprintf("Opened %s.", m.currentFeed.Name)
	case "d":
		if len(m.feeds) == 0 {
			return m, nil
		}
		name := m.feeds[m.feedIndex].Name
		if err := m.store.RemoveFeed(m.feeds[m.feedIndex].ID); err != nil {
			m.status = err.Error()
		} else {
			m.reloadFeeds()
			m.status = fmt.Sprintf("Removed %s.", name)
		}
	case "r":
		m.status = "Refreshing feeds..."
		return m, m.refreshFeedsCmd()
	case "?", "h":
		m.showHelp = true
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleArticles(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputFocused {
		switch msg.String() {
		case "esc":
			m.inputFocused = false
			m.input.Blur()
			m.status = "Article search closed."
			return m, nil
		case "enter":
			m.articleFilter = strings.TrimSpace(m.input.Value())
			m.reloadArticles()
			m.status = "Article filter updated."
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.articleFilter = m.input.Value()
		m.reloadArticles()
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.screen = screenFeeds
		m.currentFeed = feed.Feed{}
		m.status = "Back to feeds."
	case "/":
		m.inputFocused = true
		m.input.Placeholder = "search articles..."
		m.input.Focus()
	case "j", "down":
		m.moveArticle(1)
	case "k", "up":
		m.moveArticle(-1)
	case "a":
		m.status = "Marking all articles as read..."
		return m, m.markAllReadCmd()
	case "u":
		if len(m.articles) == 0 {
			return m, nil
		}
		return m, m.toggleReadCmd(m.articles[m.articleIndex].ID)
	case "enter":
		if len(m.articles) == 0 {
			return m, nil
		}
		m.status = fmt.Sprintf("Opening %s...", m.articles[m.articleIndex].Title)
		return m, m.openArticleCmd(m.articles[m.articleIndex].ID)
	case "?":
		m.showHelp = true
	}
	return m, nil
}

func (m *Model) handleReader(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputFocused {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.exitReaderInputMode()
			m.togglePaneFocus()
			return m, nil
		case "esc":
			m.exitReaderInputMode()
			return m, nil
		case "ctrl+l":
			if m.inputMode == inputChat {
				m.chatHistory = nil
				m.chatViewport.SetContent(m.renderChatContent())
				m.status = "Chat history cleared."
				return m, nil
			}
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.exitReaderInputMode()
				return m, nil
			}
			if m.inputMode == inputChat && m.waiting {
				return m, nil
			}
			m.input.SetValue("")
			return m, m.submitReaderInput(value)
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "tab":
		if !m.showAI {
			m.status = "AI pane is hidden. Press v to show it."
			return m, nil
		}
		m.togglePaneFocus()
		return m, nil
	case "/":
		m.focusCommandInput("/")
		return m, nil
	case ":":
		m.focusCommandInput("")
		return m, nil
	case "enter", "i":
		if m.activePane == paneChat && m.showAI {
			m.focusChatInput()
		}
		return m, nil
	case "esc":
		return m, nil
	case "q":
		m.screen = screenArticles
		m.activePane = paneArticle
		m.exitReaderInputMode()
		m.status = "Back to article list."
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "t":
		next := nextTheme(m.themeName)
		m.applyTheme(next.Name)
		m.status = "Theme switched to " + next.Name + "."
		return m, nil
	case "v":
		m.toggleAIPane()
		return m, nil
	}

	if m.activePane == paneChat {
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "j", "down":
		m.articleViewport.LineDown(1)
	case "k", "up":
		m.articleViewport.LineUp(1)
	case "d", "ctrl+d":
		m.articleViewport.HalfPageDown()
	case "u", "ctrl+u":
		m.articleViewport.HalfPageUp()
	case "g":
		if m.pendingG {
			m.articleViewport.GotoTop()
			m.pendingG = false
		} else {
			m.pendingG = true
		}
	case "G":
		m.articleViewport.GotoBottom()
		m.pendingG = false
	case "[":
		m.split = clamp(m.split+5, 50, 90)
		m.syncReaderLayout()
	case "]":
		m.split = clamp(m.split-5, 50, 90)
		m.syncReaderLayout()
	case "o":
		if err := openBrowser(m.currentArticle.URL); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Opened article in browser."
		}
	case "y":
		if err := copyToClipboard(extractArticleCode(m.articleMarkdown)); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Copied article code blocks."
		}
	default:
		m.pendingG = false
	}

	m.markArticleIfDone()
	return m, nil
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}
	if m.showHelp {
		return m.renderHelp()
	}

	switch m.screen {
	case screenFeeds:
		return m.renderFeeds()
	case screenArticles:
		return m.renderArticles()
	case screenReader:
		return m.renderReader()
	default:
		return "Loading..."
	}
}

func (m *Model) renderFeeds() string {
	header := m.renderTopBar("tryoutshell-rss-feed", fmt.Sprintf("%d feeds", len(m.feeds)))
	listHeight := max(6, m.height-8)
	lines := []string{"  YOUR FEEDS", "  "}
	if len(m.feeds) == 0 {
		lines = append(lines, "  No feeds yet. Press / and paste an RSS or Atom URL.")
	} else {
		start, end := visibleRange(m.feedIndex, len(m.feeds), listHeight-2)
		for index := start; index < end; index++ {
			item := m.feeds[index]
			pointer := "  "
			if index == m.feedIndex {
				pointer = "▸ "
			}
			line := fmt.Sprintf("%s%-30s %4d articles   Updated %s", pointer, truncate(item.Name, 30), item.ArticleCount, humanTime(item.UpdatedAt))
			if index == m.feedIndex {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent)).Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
	}

	body := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.theme.Border)).Width(m.width-2).Height(listHeight).Padding(1, 1).Render(strings.Join(lines, "\n"))
	input := m.renderInputBar("paste RSS feed URL to add, or search feeds...")
	footer := m.renderStatus(fmt.Sprintf("Feeds: %d    Articles: %d", len(m.feeds), m.totalArticles()), "↑↓ navigate  enter open  / input  d delete  r refresh  q quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, footer)
}

func (m *Model) renderArticles() string {
	title := "articles"
	if m.currentFeed.Name != "" {
		title = "<- " + m.currentFeed.Name
	}
	header := m.renderTopBar(title, fmt.Sprintf("%d articles", len(m.articles)))
	listHeight := max(6, m.height-8)
	lines := []string{}
	if len(m.articles) == 0 {
		lines = append(lines, "  No matching articles.")
	} else {
		start, end := visibleRange(m.articleIndex, len(m.articles), listHeight)
		for index := start; index < end; index++ {
			item := m.articles[index]
			pointer := "  "
			if index == m.articleIndex {
				pointer = "▸ "
			}
			readDot := "●"
			if item.Read {
				readDot = " "
			}
			line := fmt.Sprintf("%s%s %-46s %s", pointer, readDot, truncate(item.Title, 46), formatDate(item.PublishedAt))
			if index == m.articleIndex {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent)).Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", "● = unread")
	}

	body := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.theme.Border)).Width(m.width-2).Height(listHeight).Padding(1, 1).Render(strings.Join(lines, "\n"))
	input := m.renderInputBar("search articles...")
	footer := m.renderStatus(fmt.Sprintf("Unread: %d", countUnread(m.articles)), "esc back  ↑↓ navigate  enter open  / search  a mark all  u toggle")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, footer)
}

func (m *Model) renderReader() string {
	header := m.renderTopBar("<- "+truncate(m.currentArticle.Title, max(20, m.width-30)), articleMeta(m.currentArticle))
	leftWidth, rightWidth := m.readerPaneWidths()
	contentHeight := max(8, m.height-8)

	leftBorder := lipgloss.Color(m.theme.Border)
	rightBorder := lipgloss.Color(m.theme.Border)
	if m.activePane == paneArticle {
		leftBorder = lipgloss.Color(m.theme.BorderFocused)
	} else {
		rightBorder = lipgloss.Color(m.theme.BorderFocused)
	}

	articlePane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(leftBorder).Width(leftWidth-2).Height(contentHeight).Padding(0, 1).Render(m.articleViewport.View())
	body := articlePane
	if m.showAI {
		chatPane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(rightBorder).Width(rightWidth-2).Height(contentHeight).Padding(0, 1).Render(m.chatViewport.View())
		body = lipgloss.JoinHorizontal(lipgloss.Top, articlePane, chatPane)
	}

	placeholder := "press / for command mode, t theme, v ai pane"
	if m.inputMode == inputChat {
		placeholder = "ask anything about this article..."
	} else if m.inputMode == inputCommand {
		placeholder = "theme, resize, ai on|off, save, toc, summary, copy code"
	}
	input := m.renderInputBar(placeholder)
	footer := m.renderStatus(m.progressLabel(), "tab pane  / cmd  i chat  v ai  t theme  [/] resize  y copy code  q back")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, footer)
}

func (m *Model) renderTopBar(left, right string) string {
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Foreground)).Background(lipgloss.Color(m.theme.Background)).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color(m.theme.Border)).Padding(0, 1).Width(m.width)
	space := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	return bar.Render(left + strings.Repeat(" ", space) + right)
}

func (m *Model) renderInputBar(placeholder string) string {
	if m.input.Placeholder != placeholder {
		m.input.Placeholder = placeholder
	}
	m.input.Width = max(20, m.width-6)
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.theme.Border)).Width(m.width - 2)
	return style.Render(m.input.View())
}

func (m *Model) renderStatus(left, right string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted)).Width(m.width)
	if m.status != "" {
		left = left + "  " + m.status
	}
	space := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return style.Render(left + strings.Repeat(" ", space) + right)
}

func (m *Model) renderHelp() string {
	lines := []string{
		"tryoutshell-rss-feed help",
		"",
		"Global",
		"  / focus input or chat",
		"  ? show help",
		"  q back or quit",
		"",
		"Feeds",
		"  j/k or arrows navigate",
		"  enter open feed",
		"  d delete feed",
		"  r refresh all feeds",
		"",
		"Articles",
		"  enter open article",
		"  a mark all as read",
		"  u toggle read",
		"",
		"Reader",
		"  tab switch panes",
		"  mouse wheel scrolls the pane under the cursor",
		"  click a pane to focus it",
		"  / opens command mode",
		"  i or enter on chat pane starts chat input",
		"  j/k, d/u, gg, G scroll article",
		"  [ and ] resize panes",
		"  t cycles theme, v toggles AI pane",
		"  y copies all fenced code blocks",
		"  /theme [name], /resize 70, /ai off, /save, /summary, /toc, /open, /copy code",
		"",
		"Ctrl+C always exits.",
	}

	return lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color(m.theme.BorderFocused)).Padding(1, 2).Width(max(40, m.width-4)).Height(max(16, m.height-4)).Render(strings.Join(lines, "\n"))
}

func (m *Model) syncReaderLayout() {
	if m.width == 0 || m.height == 0 || m.screen != screenReader {
		return
	}

	leftWidth, rightWidth := m.readerPaneWidths()
	contentHeight := max(8, m.height-8)

	articleOffset := m.articleViewport.YOffset
	chatOffset := m.chatViewport.YOffset

	renderedArticle := m.renderArticle(leftWidth - 6)
	m.articleViewport = viewport.New(max(10, leftWidth-6), max(3, contentHeight-2))
	m.articleViewport.SetContent(renderedArticle)
	m.articleViewport.SetYOffset(articleOffset)

	if m.showAI {
		m.chatViewport = viewport.New(max(10, rightWidth-6), max(3, contentHeight-2))
		m.chatViewport.SetContent(m.renderChatContent())
		m.chatViewport.SetYOffset(chatOffset)
	}
}

func (m *Model) renderArticle(width int) string {
	if width < 12 {
		width = 12
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.glamourStyleName()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return m.articleMarkdown
	}
	rendered, err := renderer.Render(m.articleMarkdown)
	if err != nil {
		return m.articleMarkdown
	}
	return strings.TrimSpace(rendered)
}

func (m *Model) renderChatContent() string {
	width := max(18, m.chatViewport.Width)
	wrap := lipgloss.NewStyle().Width(width)
	if len(m.chatHistory) == 0 {
		lines := []string{"Ready to help you understand this article.", ""}
		if m.cfg.ShowSuggestions && len(m.suggestions) > 0 {
			lines = append(lines, "Suggested:")
			for _, suggestion := range m.suggestions {
				lines = append(lines, "  "+suggestion)
			}
			lines = append(lines, "")
		}
		if m.chatClient == nil || !m.chatClient.Available() {
			lines = append(lines, "Set OPENAI_API_KEY, ANTHROPIC_API_KEY, or GEMINI_API_KEY to enable AI chat.")
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted)).Render(strings.Join(lines, "\n"))
	}

	var lines []string
	userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent)).Bold(true)
	aiStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Foreground))
	for _, item := range m.chatHistory {
		label := "AI: "
		style := aiStyle
		if item.Role == "user" {
			label = "You: "
			style = userStyle
		}
		lines = append(lines, wrap.Render(style.Render(label)+item.Content))
		lines = append(lines, "")
	}
	if m.waiting {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted)).Render("Thinking..."))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) submitReaderInput(value string) tea.Cmd {
	if strings.HasPrefix(value, "/") {
		return m.runSlashCommand(value)
	}

	m.chatHistory = append(m.chatHistory, ai.Message{Role: "user", Content: value})
	if m.chatClient == nil || !m.chatClient.Available() {
		m.chatHistory = append(m.chatHistory, ai.Message{Role: "assistant", Content: "No API key found. Set OPENAI_API_KEY, ANTHROPIC_API_KEY, or GEMINI_API_KEY."})
		m.chatViewport.SetContent(m.renderChatContent())
		m.chatViewport.GotoBottom()
		return nil
	}

	m.waiting = true
	m.chatViewport.SetContent(m.renderChatContent())
	m.chatViewport.GotoBottom()

	client := m.chatClient
	return func() tea.Msg {
		reply, err := client.Send(value)
		return aiReplyMsg{content: reply, err: err}
	}
}

func (m *Model) runSlashCommand(value string) tea.Cmd {
	fields := strings.Fields(strings.TrimPrefix(value, "/"))
	m.exitReaderInputMode()
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "theme":
		if len(fields) == 1 {
			next := nextTheme(m.themeName)
			m.applyTheme(next.Name)
			m.status = "Theme switched to " + next.Name + "."
			return nil
		}
		m.applyTheme(fields[1])
		m.status = "Theme switched to " + m.themeName + "."
	case "resize":
		if len(fields) < 2 {
			m.status = "Usage: /resize 50-90"
			return nil
		}
		value, err := strconv.Atoi(fields[1])
		if err != nil {
			m.status = "Invalid resize percentage."
			return nil
		}
		m.split = clamp(value, 50, 90)
		m.syncReaderLayout()
		m.status = fmt.Sprintf("Article pane set to %d%%.", m.split)
	case "ai":
		if len(fields) == 1 || fields[1] == "toggle" {
			m.toggleAIPane()
			return nil
		}
		if fields[1] == "on" {
			if !m.showAI {
				m.toggleAIPane()
			}
			m.status = "AI pane enabled."
			return nil
		}
		if fields[1] == "off" {
			if m.showAI {
				m.toggleAIPane()
			}
			m.status = "AI pane hidden."
			return nil
		}
		m.status = "Usage: /ai on|off|toggle"
	case "save":
		if err := m.saveCurrentArticle(); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Saved article to offline storage."
		}
	case "summary":
		return m.submitReaderInput("Summarize this article in 5 concise bullets with the main takeaways.")
	case "toc":
		m.chatHistory = append(m.chatHistory, ai.Message{Role: "assistant", Content: buildTOC(m.articleMarkdown)})
		m.chatViewport.SetContent(m.renderChatContent())
		m.chatViewport.GotoBottom()
	case "open":
		if err := openBrowser(m.currentArticle.URL); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Opened article in browser."
		}
	case "copy":
		target := "code"
		if len(fields) > 1 {
			target = fields[1]
		}
		switch target {
		case "code":
			if err := copyToClipboard(extractArticleCode(m.articleMarkdown)); err != nil {
				m.status = err.Error()
			} else {
				m.status = "Copied article code blocks."
			}
		case "url":
			if err := copyToClipboard(m.currentArticle.URL); err != nil {
				m.status = err.Error()
			} else {
				m.status = "Copied article URL."
			}
		case "article":
			if err := copyToClipboard(m.articleMarkdown); err != nil {
				m.status = err.Error()
			} else {
				m.status = "Copied article markdown."
			}
		default:
			m.status = "Usage: /copy code|url|article"
		}
	case "help":
		m.showHelp = true
	default:
		m.status = "Unknown command: /" + fields[0]
	}
	return nil
}

func (m *Model) applyTheme(name string) {
	m.theme = getTheme(name)
	m.themeName = m.theme.Name
	m.cfg.Theme = m.theme.Name
	_ = config.Save(m.cfg)
	m.syncReaderLayout()
}

func (m *Model) saveCurrentArticle() error {
	if strings.TrimSpace(m.articleMarkdown) == "" {
		return fmt.Errorf("no article loaded")
	}
	slug := slugify(m.currentArticle.Title)
	if slug == "" {
		slug = "article-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	content := fmt.Sprintf("---\ntitle: %s\nurl: %s\nsaved: %s\n---\n\n%s", m.currentArticle.Title, m.currentArticle.URL, time.Now().Format(time.RFC3339), m.articleMarkdown)
	return os.WriteFile(filepath.Join(config.SavedDir(), slug+".md"), []byte(content), 0o644)
}

func (m *Model) addFeedCmd(url string) tea.Cmd {
	return func() tea.Msg {
		item, count, err := m.store.AddFeed(context.Background(), url, m.cfg.MaxArticlesPerFeed)
		return feedAddedMsg{item: item, count: count, err: err}
	}
}

func (m *Model) refreshFeedsCmd() tea.Cmd {
	return func() tea.Msg {
		count, err := m.store.RefreshAll(context.Background(), m.cfg.MaxArticlesPerFeed)
		return feedsRefreshedMsg{count: count, err: err}
	}
}

func (m *Model) openArticleCmd(articleID string) tea.Cmd {
	return func() tea.Msg {
		article, markdown, err := m.store.EnsureMarkdown(context.Background(), articleID)
		return articleOpenedMsg{article: article, markdown: markdown, err: err}
	}
}

func (m *Model) toggleReadCmd(articleID string) tea.Cmd {
	return func() tea.Msg {
		article, err := m.store.ToggleRead(articleID)
		return readToggledMsg{article: article, err: err}
	}
}

func (m *Model) markAllReadCmd() tea.Cmd {
	return func() tea.Msg {
		count, err := m.store.MarkAllRead(m.currentFeed.ID)
		return markedAllReadMsg{count: count, err: err}
	}
}

func (m *Model) reloadFeeds() {
	m.feeds = m.store.ListFeeds()
	if m.feedFilter != "" {
		var filtered []feed.Feed
		query := strings.ToLower(strings.TrimSpace(m.feedFilter))
		for _, item := range m.feeds {
			if strings.Contains(strings.ToLower(item.Name+" "+item.Description), query) {
				filtered = append(filtered, item)
			}
		}
		m.feeds = filtered
	}
	if m.feedIndex >= len(m.feeds) {
		m.feedIndex = max(0, len(m.feeds)-1)
	}
}

func (m *Model) reloadArticles() {
	if m.currentFeed.ID == "" {
		m.articles = nil
		m.articleIndex = 0
		return
	}
	m.articles = m.store.ListArticles(m.currentFeed.ID, m.articleFilter)
	if m.articleIndex >= len(m.articles) {
		m.articleIndex = max(0, len(m.articles)-1)
	}
}

func (m *Model) selectFeed(feedID string) {
	for index, item := range m.feeds {
		if item.ID == feedID {
			m.feedIndex = index
			return
		}
	}
}

func (m *Model) moveFeed(delta int) {
	if len(m.feeds) == 0 {
		return
	}
	m.feedIndex = clamp(m.feedIndex+delta, 0, len(m.feeds)-1)
}

func (m *Model) moveArticle(delta int) {
	if len(m.articles) == 0 {
		return
	}
	m.articleIndex = clamp(m.articleIndex+delta, 0, len(m.articles)-1)
}

func (m *Model) markArticleIfDone() {
	if !m.cfg.MarkReadOnScroll || m.currentArticle.ID == "" {
		return
	}
	if m.articleViewport.ScrollPercent() < 0.9 {
		return
	}
	if err := m.store.MarkRead(m.currentArticle.ID); err == nil {
		if article, ok := m.store.GetArticle(m.currentArticle.ID); ok {
			m.currentArticle = article
		}
	}
}

func (m *Model) progressLabel() string {
	progress := int(m.articleViewport.ScrollPercent() * 100)
	provider := "AI offline"
	if m.chatClient != nil && m.chatClient.Available() {
		provider = strings.Title(m.chatClient.Provider())
	}
	if !m.showAI {
		provider = "Focus mode"
	}
	return fmt.Sprintf("Reading %d%%   %s   %s", progress, provider, m.themeName)
}

func (m *Model) totalArticles() int {
	total := 0
	for _, item := range m.store.ListFeeds() {
		total += item.ArticleCount
	}
	return total
}

func buildSuggestions(article feed.Article, markdown string) []string {
	suggestions := []string{"What's the main idea here?", "Summarize the key takeaways"}
	if article.Title != "" {
		suggestions = append(suggestions, "Explain "+article.Title)
	}
	return suggestions
}

func buildTOC(markdown string) string {
	var lines []string
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "No headings found in this article."
	}
	return "Table of contents:\n" + strings.Join(lines, "\n")
}

func articleMeta(article feed.Article) string {
	parts := []string{}
	if article.FeedName != "" {
		parts = append(parts, article.FeedName)
	}
	if !article.PublishedAt.IsZero() {
		parts = append(parts, article.PublishedAt.Format("Jan 2006"))
	}
	return strings.Join(parts, " · ")
}

func visibleRange(selected, total, limit int) (int, int) {
	if total <= limit {
		return 0, total
	}
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > total {
		end = total
		start = end - limit
	}
	return start, end
}

func countUnread(items []feed.Article) int {
	count := 0
	for _, item := range items {
		if !item.Read {
			count++
		}
	}
	return count
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	delta := time.Since(t)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
	}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 2, 2006")
}

func (m *Model) handleFeedsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveFeed(-1)
	case tea.MouseButtonWheelDown:
		m.moveFeed(1)
	}
	return m, nil
}

func (m *Model) handleArticlesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveArticle(-1)
	case tea.MouseButtonWheelDown:
		m.moveArticle(1)
	}
	return m, nil
}

func (m *Model) handleReaderMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	leftWidth, _ := m.readerPaneWidths()
	inArticle := msg.X < leftWidth
	if msg.Button == tea.MouseButtonLeft {
		if inArticle || !m.showAI {
			m.activePane = paneArticle
			m.exitReaderInputMode()
			m.status = "Article focus."
		} else {
			m.activePane = paneChat
			m.exitReaderInputMode()
			m.status = "Chat focus. Press i or Enter to type."
		}
		return m, nil
	}

	targetArticle := inArticle || !m.showAI
	if targetArticle {
		var cmd tea.Cmd
		m.articleViewport, cmd = m.articleViewport.Update(msg)
		m.markArticleIfDone()
		return m, cmd
	}

	var cmd tea.Cmd
	m.chatViewport, cmd = m.chatViewport.Update(msg)
	return m, cmd
}

func (m *Model) readerPaneWidths() (int, int) {
	if !m.showAI {
		return m.width, 0
	}
	leftWidth := max(40, (m.width*m.split)/100)
	rightWidth := max(24, m.width-leftWidth-1)
	return leftWidth, rightWidth
}

func (m *Model) togglePaneFocus() {
	if m.activePane == paneArticle {
		m.activePane = paneChat
		m.status = "Chat focus. Press i or Enter to type."
		return
	}
	m.activePane = paneArticle
	m.status = "Article focus."
}

func (m *Model) focusChatInput() {
	m.activePane = paneChat
	m.inputFocused = true
	m.inputMode = inputChat
	m.input.Placeholder = "ask anything about this article..."
	m.input.Focus()
	m.status = "Chat input active."
}

func (m *Model) focusCommandInput(seed string) {
	m.inputFocused = true
	m.inputMode = inputCommand
	m.input.Placeholder = "theme, resize, ai on|off, save, toc, summary, copy code"
	m.input.Focus()
	m.input.SetValue(seed)
	m.input.CursorEnd()
	m.status = "Command mode."
}

func (m *Model) exitReaderInputMode() {
	m.inputFocused = false
	m.inputMode = inputNone
	m.input.Blur()
	m.input.SetValue("")
	if m.activePane == paneChat && m.showAI {
		m.status = "Chat focus. Press i or Enter to type."
	} else {
		m.status = "Article focus."
	}
}

func (m *Model) toggleAIPane() {
	m.showAI = !m.showAI
	if !m.showAI {
		m.activePane = paneArticle
		m.exitReaderInputMode()
		m.status = "AI pane hidden."
	} else {
		m.status = "AI pane visible."
	}
	m.syncReaderLayout()
}

func (m *Model) glamourStyleName() string {
	switch m.themeName {
	case "light":
		return "light"
	case "tokyo-night", "nord":
		return "tokyo-night"
	case "dracula":
		return "dracula"
	default:
		return "dark"
	}
}

func copyToClipboard(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("nothing to copy")
	}
	if err := clipboard.WriteAll(value); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}

func extractArticleCode(markdown string) string {
	var blocks []string
	var current []string
	inCode := false
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCode {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
				inCode = false
			} else {
				inCode = true
			}
			continue
		}
		if inCode {
			current = append(current, line)
		}
	}
	return strings.Join(blocks, "\n\n")
}

func looksLikeURL(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func truncate(value string, width int) string {
	value = strings.TrimSpace(value)
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func openBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("article has no URL")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

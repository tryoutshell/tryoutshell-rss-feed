package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type Message struct {
	Role    string
	Content string
}

type Client struct {
	provider string
	history  []Message
	article  string
}

func NewClient(articleContent string) *Client {
	c := &Client{article: articleContent}
	if os.Getenv("OPENAI_API_KEY") != "" {
		c.provider = "openai"
	} else if os.Getenv("ANTHROPIC_API_KEY") != "" {
		c.provider = "anthropic"
	} else if os.Getenv("GEMINI_API_KEY") != "" {
		c.provider = "gemini"
	}
	return c
}

func (c *Client) Available() bool {
	return c.provider != ""
}

func (c *Client) Provider() string {
	return c.provider
}

func (c *Client) Send(userMessage string) (string, error) {
	c.history = append(c.history, Message{Role: "user", Content: userMessage})

	var reply string
	var err error
	switch c.provider {
	case "openai":
		reply, err = c.sendOpenAI()
	case "anthropic":
		reply, err = c.sendAnthropic()
	case "gemini":
		reply, err = c.sendGemini()
	default:
		return "", fmt.Errorf("no API key found. Set OPENAI_API_KEY, ANTHROPIC_API_KEY, or GEMINI_API_KEY")
	}
	if err != nil {
		return "", err
	}

	c.history = append(c.history, Message{Role: "assistant", Content: reply})
	return reply, nil
}

func (c *Client) systemPrompt() string {
	truncated := c.article
	if len(truncated) > 8000 {
		truncated = truncated[:8000] + "\n\n[article truncated]"
	}
	return fmt.Sprintf("You are a helpful assistant. The user is reading this article:\n\n%s\n\nAnswer questions about the article. Be concise.", truncated)
}

func (c *Client) sendOpenAI() (string, error) {
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: c.systemPrompt()},
	}
	for _, m := range c.history {
		role := openai.ChatMessageRoleUser
		if m.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{Role: role, Content: m.Content})
	}

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    openai.GPT4oMini,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("openai: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) sendAnthropic() (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	var messages []message
	for _, m := range c.history {
		messages = append(messages, message{Role: m.Role, Content: m.Content})
	}

	body := struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system"`
		Messages  []message `json:"messages"`
	}{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		System:    c.systemPrompt(),
		Messages:  messages,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []contentBlock `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, ""), nil
}

func (c *Client) sendGemini() (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	contents = append(contents, content{Role: "user", Parts: []part{{Text: c.systemPrompt()}}})
	contents = append(contents, content{Role: "model", Parts: []part{{Text: "Understood. I'll help with questions about this article."}}})

	for _, m := range c.history {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: m.Content}}})
	}

	body := struct {
		Contents []content `json:"contents"`
	}{Contents: contents}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("gemini: no candidates returned")
	}

	var parts []string
	for _, p := range result.Candidates[0].Content.Parts {
		parts = append(parts, p.Text)
	}
	return strings.Join(parts, ""), nil
}

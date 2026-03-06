package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const appName = "tryoutshell-rss-feed"

type Config struct {
	Theme              string `yaml:"theme"`
	CodeTheme          string `yaml:"code_theme"`
	DefaultSplit       int    `yaml:"default_split"`
	FontSize           string `yaml:"font_size"`
	ShowProgressBar    bool   `yaml:"show_progress_bar"`
	MarkReadOnScroll   bool   `yaml:"mark_read_on_scroll"`
	AIProvider         string `yaml:"ai_provider"`
	AIModel            string `yaml:"ai_model"`
	ShowSuggestions    bool   `yaml:"show_suggestions"`
	RefreshInterval    string `yaml:"refresh_interval"`
	MaxArticlesPerFeed int    `yaml:"max_articles_per_feed"`
}

func Default() Config {
	return Config{
		Theme:              "dracula",
		CodeTheme:          "monokai",
		DefaultSplit:       70,
		FontSize:           "normal",
		ShowProgressBar:    true,
		MarkReadOnScroll:   true,
		AIProvider:         "anthropic",
		AIModel:            "claude-sonnet-4-20250514",
		ShowSuggestions:    true,
		RefreshInterval:    "30m",
		MaxArticlesPerFeed: 50,
	}
}

func Load() (Config, error) {
	cfg := Default()
	if err := EnsureDirs(); err != nil {
		return cfg, err
	}

	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, Save(cfg)
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}

	if cfg.DefaultSplit < 50 || cfg.DefaultSplit > 90 {
		cfg.DefaultSplit = Default().DefaultSplit
	}
	if cfg.MaxArticlesPerFeed <= 0 {
		cfg.MaxArticlesPerFeed = Default().MaxArticlesPerFeed
	}
	if cfg.Theme == "" {
		cfg.Theme = Default().Theme
	}
	if cfg.CodeTheme == "" {
		cfg.CodeTheme = Default().CodeTheme
	}
	if cfg.RefreshInterval == "" {
		cfg.RefreshInterval = Default().RefreshInterval
	}

	return cfg, nil
}

func Save(cfg Config) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(), data, 0o644)
}

func (c Config) RefreshEvery() time.Duration {
	d, err := time.ParseDuration(c.RefreshInterval)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func ConfigDir() string {
	return joinHome(".config", appName)
}

func ThemesDir() string {
	return filepath.Join(ConfigDir(), "themes")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

func DataDir() string {
	return joinHome(".local", "share", appName)
}

func CacheDir() string {
	return filepath.Join(DataDir(), "cache")
}

func SavedDir() string {
	return filepath.Join(DataDir(), "saved")
}

func StatePath() string {
	return filepath.Join(DataDir(), "state.json")
}

func EnsureDirs() error {
	dirs := []string{
		ConfigDir(),
		ThemesDir(),
		DataDir(),
		CacheDir(),
		SavedDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func joinHome(parts ...string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(parts...)
	}
	all := append([]string{homeDir}, parts...)
	return filepath.Join(all...)
}

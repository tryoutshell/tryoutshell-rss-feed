package cmd

import (
	"fmt"
	"os"

	"github.com/rahulxf/tryoutshell-rss-feed/internal/app"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/feed"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tryoutshell-rss-feed",
	Short: "Terminal-native RSS reader with an integrated AI assistant",
	Long: `tryoutshell-rss-feed is a reading-first terminal RSS reader built around
feeds, articles, and a split-pane reading experience with contextual AI help.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return launchReaderApp()
	},
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func launchReaderApp() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := feed.Open(config.StatePath())
	if err != nil {
		return fmt.Errorf("opening feed store: %w", err)
	}

	return app.Launch(cfg, store)
}

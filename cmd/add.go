package cmd

import (
	"context"
	"fmt"

	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/feed"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <rss-url>",
	Short: "Add an RSS or Atom feed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		store, err := feed.Open(config.StatePath())
		if err != nil {
			return err
		}

		item, count, err := store.AddFeed(context.Background(), args[0], cfg.MaxArticlesPerFeed)
		if err != nil {
			return err
		}

		fmt.Printf("Added %s (%d articles)\n", item.Name, count)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}

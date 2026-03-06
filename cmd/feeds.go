package cmd

import (
	"fmt"

	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/feed"
	"github.com/spf13/cobra"
)

var feedsCmd = &cobra.Command{
	Use:   "feeds",
	Short: "List saved RSS feeds",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := feed.Open(config.StatePath())
		if err != nil {
			return err
		}

		items := store.ListFeeds()
		if len(items) == 0 {
			fmt.Println("No feeds saved yet.")
			fmt.Println("Add one with: tryoutshell-rss-feed add <rss-url>")
			return nil
		}

		for _, item := range items {
			fmt.Printf("%-28s %4d articles  updated %s\n", item.Name, item.ArticleCount, item.UpdatedAt.Format("2006-01-02 15:04"))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(feedsCmd)
}

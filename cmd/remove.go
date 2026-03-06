package cmd

import (
	"fmt"

	"github.com/rahulxf/tryoutshell-rss-feed/internal/config"
	"github.com/rahulxf/tryoutshell-rss-feed/internal/feed"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <feed-name>",
	Short: "Remove a saved feed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := feed.Open(config.StatePath())
		if err != nil {
			return err
		}

		if err := store.RemoveFeed(args[0]); err != nil {
			return err
		}

		fmt.Printf("Removed %s\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

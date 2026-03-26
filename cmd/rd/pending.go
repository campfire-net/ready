package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/3dl-dev/ready/pkg/resolve"
	"github.com/3dl-dev/ready/pkg/views"
)

var pendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "Show items in waiting, scheduled, or blocked status",
	Long: `Show work items that are pending — waiting on something, scheduled for later,
or blocked by a dependency.

Items appear in the pending view when status is one of: waiting, scheduled, blocked.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		items, err := resolve.AllItems(s)
		if err != nil {
			return fmt.Errorf("loading items: %w", err)
		}

		filter := views.PendingFilter()
		items = views.Apply(items, filter)

		sort.Slice(items, func(i, j int) bool {
			pi := priorityOrder(items[i].Priority)
			pj := priorityOrder(items[j].Priority)
			if pi != pj {
				return pi < pj
			}
			return items[i].ETA < items[j].ETA
		})

		if jsonOutput {
			return outputItemsJSON(items)
		}

		if len(items) == 0 {
			fmt.Println("nothing pending")
			return nil
		}

		printItemTable(items)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pendingCmd)
}

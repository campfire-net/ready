package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
	"github.com/third-division/ready/pkg/views"
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Show items actively being worked on",
	Long: `Show work items with status=active — items currently being worked on.

Use --for to filter by the party the work is assigned to.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		forFilter, _ := cmd.Flags().GetString("for")

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		items, err := resolve.AllItems(s)
		if err != nil {
			return fmt.Errorf("loading items: %w", err)
		}

		var filter views.Filter
		if forFilter != "" {
			filter = views.MyWorkFilter(forFilter)
		} else {
			filter = views.WorkFilter()
		}
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
			fmt.Println("nothing active")
			return nil
		}

		printItemTable(items)
		return nil
	},
}

func init() {
	workCmd.Flags().String("for", "", "filter by party (by field) — shows my-work view for that identity")
	rootCmd.AddCommand(workCmd)
}

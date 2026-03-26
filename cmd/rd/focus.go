package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/3dl-dev/ready/pkg/resolve"
	"github.com/3dl-dev/ready/pkg/views"
)

var focusCmd = &cobra.Command{
	Use:   "focus",
	Short: "Show ready items, optionally narrowed to a gate type",
	Long: `Show items in the ready view, optionally filtered to a specific gate type.

Like 'rd ready' but with --boost-gates to narrow to items requiring human escalation
of a specific type (budget, design, scope, review, human, stall).

Example:
  rd focus                          # all ready items
  rd focus --boost-gates design     # ready items awaiting design review
  rd focus --boost-gates budget     # ready items awaiting budget approval`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gateType, _ := cmd.Flags().GetString("boost-gates")

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		items, err := resolve.AllItems(s)
		if err != nil {
			return fmt.Errorf("loading items: %w", err)
		}

		filter := views.FocusFilter(gateType)
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
			if gateType != "" {
				fmt.Printf("nothing ready with gate=%s\n", gateType)
			} else {
				fmt.Println("nothing ready")
			}
			return nil
		}

		printItemTable(items)
		return nil
	},
}

func init() {
	focusCmd.Flags().String("boost-gates", "", "narrow to gate items of this type: budget, design, scope, review, human, stall")
	rootCmd.AddCommand(focusCmd)
}

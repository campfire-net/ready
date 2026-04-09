package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List work items",
	Long: `List work items across all campfires.

Filters (all optional, combinable):
  --status    filter by status (repeatable, OR semantics)
  --for       filter by 'for' party
  --by        filter by 'by' party
  --project   filter by project
  --priority  filter by priority (p0, p1, p2, p3)
  --type      filter by type
  --all       include terminal items (done, cancelled, failed)

By default, terminal items (done, cancelled, failed) are excluded.

Example:
  rd list                                    all open items
  rd list --all                              include done/cancelled
  rd list --status inbox --status active     OR filter
  rd list --by atlas/worker-3 --json         machine-readable
  rd list --priority p0 --priority p1        urgent items only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		statusFilters, _ := cmd.Flags().GetStringArray("status")
		forFilter, _ := cmd.Flags().GetString("for")
		byFilter, _ := cmd.Flags().GetString("by")
		projectFilter, _ := cmd.Flags().GetString("project")
		priorityFilter, _ := cmd.Flags().GetString("priority")
		typeFilter, _ := cmd.Flags().GetString("type")
		all, _ := cmd.Flags().GetBool("all")

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		items, err := allItemsFromJSONLOrStore(s)
		if err != nil {
			return fmt.Errorf("loading items: %w", err)
		}

		// Apply filters.
		filtered := applyListFilters(items, statusFilters, forFilter, byFilter, projectFilter, priorityFilter, typeFilter, all)

		// Sort by priority then ID.
		sort.Slice(filtered, func(i, j int) bool {
			pi := priorityOrder(filtered[i].Priority)
			pj := priorityOrder(filtered[j].Priority)
			if pi != pj {
				return pi < pj
			}
			return filtered[i].ID < filtered[j].ID
		})

		if jsonOutput {
			return outputItemsJSON(filtered)
		}

		if len(filtered) == 0 {
			if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
				fmt.Println("no items found")
			}
			return nil
		}

		// Pipe-friendly output: print bare IDs when stdout is not a TTY so
		// scripts can do: for id in $(rd list); do ...; done
		if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			printItemTable(filtered)
		} else {
			for _, item := range filtered {
				fmt.Println(item.ID)
			}
		}
		return nil
	},
}

// applyListFilters filters items according to the list command's flag values.
// statusFilters uses OR semantics: an item matches if its status equals any of the
// provided values. When statusFilters is empty and all is false, terminal items
// (done, cancelled, failed) are excluded by default.
func applyListFilters(items []*state.Item, statusFilters []string, forFilter, byFilter, projectFilter, priorityFilter, typeFilter string, all bool) []*state.Item {
	var filtered []*state.Item
	for _, item := range items {
		if !all && state.IsTerminal(item) && len(statusFilters) == 0 {
			continue
		}
		if len(statusFilters) > 0 {
			matched := false
			for _, sf := range statusFilters {
				if item.Status == resolveStatus(sf) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if forFilter != "" && item.For != forFilter {
			continue
		}
		if byFilter != "" && item.By != byFilter {
			continue
		}
		if projectFilter != "" && item.Project != projectFilter {
			continue
		}
		if priorityFilter != "" && item.Priority != priorityFilter {
			continue
		}
		if typeFilter != "" && item.Type != typeFilter {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func init() {
	listCmd.Flags().StringArray("status", nil, "filter by status (repeatable, OR semantics)")
	listCmd.Flags().String("for", "", "filter by 'for' party")
	listCmd.Flags().String("by", "", "filter by 'by' party")
	listCmd.Flags().String("project", "", "filter by project")
	listCmd.Flags().String("priority", "", "filter by priority")
	listCmd.Flags().String("type", "", "filter by type")
	listCmd.Flags().Bool("all", false, "include terminal items (done, cancelled, failed)")
	rootCmd.AddCommand(listCmd)
}

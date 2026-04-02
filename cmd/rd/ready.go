package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/views"
)

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Show items needing attention now",
	Long: `Show work items that need attention now.

Items appear in the ready view when:
  - not in a terminal status (done, cancelled, failed)
  - not blocked
  - ETA is within the next 4 hours

Named views:
  ready      what needs attention now (default)
  work       items actively being worked on
  pending    waiting, scheduled, or blocked
  overdue    past-due items
  delegated  work I delegated, in progress
  my-work    work assigned to me

Example:
  rd ready
  rd ready --view overdue
  rd ready --view my-work --json
  rd ready --for ""                show all items, not just mine`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viewName, _ := cmd.Flags().GetString("view")
		forFilter, _ := cmd.Flags().GetString("for")
		projectFilter, _ := cmd.Flags().GetString("project")

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Default --for to the current session identity when not explicitly set.
		if !cmd.Flags().Changed("for") {
			forFilter = agentID.PublicKeyHex()
		}

		items, err := allItemsFromJSONLOrStore(s)
		if err != nil {
			return fmt.Errorf("loading items: %w", err)
		}

		// Apply view filter.
		if viewName == "" {
			viewName = views.ViewReady
		}
		filter := views.Named(viewName, forFilter)
		if filter == nil {
			return fmt.Errorf("unknown view %q: choose from %v", viewName, views.AllNames())
		}
		items = views.Apply(items, filter)

		// For views that don't filter by identity internally, apply --for as a
		// secondary filter on item.For when set.
		switch viewName {
		case views.ViewDelegated, views.ViewMyWork:
			// Already filtered by identity in the view function.
		default:
			if forFilter != "" {
				items = views.Apply(items, func(item *state.Item) bool {
					return item.For == forFilter
				})
			}
		}

		items = filterByProject(items, projectFilter)

		sortByPriorityETA(items)

		if jsonOutput {
			return outputItemsJSON(items)
		}

		if len(items) == 0 {
			fmt.Println("nothing ready")
			return nil
		}

		printItemTable(items)
		return nil
	},
}

func init() {
	readyCmd.Flags().String("view", "ready", "named view: ready, work, pending, overdue, delegated, my-work")
	readyCmd.Flags().String("for", "", "filter by 'for' party (default: current identity; pass \"\" to show all)")
	readyCmd.Flags().String("project", "", "filter by project")
	rootCmd.AddCommand(readyCmd)
}

// filterByProject returns only items matching the given project, or all items if project is empty.
func filterByProject(items []*state.Item, project string) []*state.Item {
	if project == "" {
		return items
	}
	var out []*state.Item
	for _, item := range items {
		if item.Project == project {
			out = append(out, item)
		}
	}
	return out
}

// sortByPriorityETA sorts items by priority (ascending) then ETA (ascending).
// Used by ready, work, pending, focus, and gates views.
func sortByPriorityETA(items []*state.Item) {
	sort.Slice(items, func(i, j int) bool {
		pi := priorityOrder(items[i].Priority)
		pj := priorityOrder(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return items[i].ETA < items[j].ETA
	})
}

// priorityOrder maps priority strings to sort order integers.
func priorityOrder(p string) int {
	switch p {
	case "p0":
		return 0
	case "p1":
		return 1
	case "p2":
		return 2
	case "p3":
		return 3
	default:
		return 9
	}
}

// printItemTable prints items in a compact table format.
func printItemTable(items []*state.Item) {
	for _, item := range items {
		eta := formatETA(item.ETA)
		status := item.Status
		fmt.Printf("  %-16s  %-8s  %-10s  %-10s  %s\n",
			item.ID, item.Priority, status, eta, item.Title)
	}
}

// formatETA formats an ETA string for display.
func formatETA(eta string) string {
	if eta == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, eta)
	if err != nil {
		return eta
	}
	now := time.Now()
	diff := t.Sub(now)
	switch {
	case diff < 0:
		return "overdue"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

// outputItemsJSON outputs items as JSON.
func outputItemsJSON(items []*state.Item) error {
	if items == nil {
		items = []*state.Item{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

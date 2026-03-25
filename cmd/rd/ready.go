package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
	"github.com/third-division/ready/pkg/state"
	"github.com/third-division/ready/pkg/views"
)

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Show items needing attention now",
	Long: `Show work items that need attention now.

Items appear in the ready view when:
  - not in a terminal status (done, cancelled, failed)
  - not blocked
  - ETA is within the next 4 hours

Use --view to select a different named view:
  ready, work, pending, overdue, delegated, my-work`,
	RunE: func(cmd *cobra.Command, args []string) error {
		viewName, _ := cmd.Flags().GetString("view")
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

		// Apply view filter.
		var filter views.Filter
		if viewName == "" {
			viewName = views.ViewReady
		}
		if forFilter != "" {
			filter = views.Named(viewName, forFilter)
		} else {
			filter = views.Named(viewName, "")
		}
		if filter == nil {
			return fmt.Errorf("unknown view %q: choose from %v", viewName, views.AllNames())
		}
		items = views.Apply(items, filter)

		// Sort by priority then ETA.
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
			fmt.Println("nothing ready")
			return nil
		}

		printItemTable(items)
		return nil
	},
}

func init() {
	readyCmd.Flags().String("view", "ready", "named view: ready, work, pending, overdue, delegated, my-work")
	readyCmd.Flags().String("for", "", "filter by 'for' party (required for delegated and my-work views)")
	rootCmd.AddCommand(readyCmd)
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

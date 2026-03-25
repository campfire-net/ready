package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
)

var showCmd = &cobra.Command{
	Use:   "show <item-id>",
	Short: "Show a work item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		item, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(item)
		}

		// Human-readable output.
		fmt.Printf("ID:       %s\n", item.ID)
		fmt.Printf("Title:    %s\n", item.Title)
		fmt.Printf("Status:   %s\n", item.Status)
		fmt.Printf("Type:     %s\n", item.Type)
		fmt.Printf("Priority: %s\n", item.Priority)
		fmt.Printf("For:      %s\n", item.For)
		if item.By != "" {
			fmt.Printf("By:       %s\n", item.By)
		}
		if item.Project != "" {
			fmt.Printf("Project:  %s\n", item.Project)
		}
		if item.Level != "" {
			fmt.Printf("Level:    %s\n", item.Level)
		}
		if item.ETA != "" {
			fmt.Printf("ETA:      %s (%s)\n", item.ETA, formatETA(item.ETA))
		}
		if item.Due != "" {
			fmt.Printf("Due:      %s\n", item.Due)
		}
		if item.ParentID != "" {
			fmt.Printf("Parent:   %s\n", item.ParentID)
		}
		if len(item.BlockedBy) > 0 {
			fmt.Printf("Blocked by: %s\n", strings.Join(item.BlockedBy, ", "))
		}
		if len(item.Blocks) > 0 {
			fmt.Printf("Blocks:   %s\n", strings.Join(item.Blocks, ", "))
		}
		if item.WaitingOn != "" {
			fmt.Printf("Waiting on: %s (%s)\n", item.WaitingOn, item.WaitingType)
		}
		if item.Context != "" {
			fmt.Printf("\nContext:\n%s\n", item.Context)
		}
		fmt.Printf("\nCampfire: %s\n", item.CampfireID)
		fmt.Printf("Msg ID:   %s\n", item.MsgID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}

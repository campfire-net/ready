package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
	"github.com/third-division/ready/pkg/state"
	"github.com/third-division/ready/pkg/timeparse"
)

// doneCmd closes an item with resolution=done.
var doneCmd = &cobra.Command{
	Use:   "done <item-id>",
	Short: "Close a work item as done",
	Long: `Close a work item with resolution=done.

Alias for: rd close <item-id> --resolution done

Example:
  rd done ready-a1b --reason "Implemented and merged"`,
	Args: cobra.ExactArgs(1),
	RunE: runCloseAlias("done"),
}

// failCmd closes an item with resolution=failed.
var failCmd = &cobra.Command{
	Use:   "fail <item-id>",
	Short: "Close a work item as failed",
	Long: `Close a work item with resolution=failed.

Alias for: rd close <item-id> --resolution failed

Example:
  rd fail ready-a1b --reason "Approach didn't work"`,
	Args: cobra.ExactArgs(1),
	RunE: runCloseAlias("failed"),
}

// cancelCmd closes an item with resolution=cancelled, optionally cascading to children.
var cancelCmd = &cobra.Command{
	Use:   "cancel <item-id>",
	Short: "Close a work item as cancelled",
	Long: `Close a work item with resolution=cancelled.

Alias for: rd close <item-id> --resolution cancelled

Use --cascade to also close all open children (items with parent_id = this item).

Example:
  rd cancel ready-a1b --reason "No longer needed"
  rd cancel ready-a1b --reason "Scope cut" --cascade`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		reason, _ := cmd.Flags().GetString("reason")
		cascade, _ := cmd.Flags().GetBool("cascade")

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Resolve the item.
		item, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		var closedIDs []string

		// Cascade: close open children first.
		if cascade {
			allItems, err := resolve.AllItems(s)
			if err != nil {
				return fmt.Errorf("loading items for cascade: %w", err)
			}
			for _, child := range allItems {
				if child.ParentID != item.ID {
					continue
				}
				if state.IsTerminal(child) {
					continue
				}
				p := closePayload{
					Target:     child.MsgID,
					Resolution: "cancelled",
					Reason:     reason,
				}
				payloadBytes, err := json.Marshal(p)
				if err != nil {
					return fmt.Errorf("encoding child payload: %w", err)
				}
				tags := []string{"work:close", "work:resolution:cancelled"}
				antecedents := []string{child.MsgID}
				_, _, err = sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
				if err != nil {
					return fmt.Errorf("closing child %s: %w", child.ID, err)
				}
				closedIDs = append(closedIDs, child.ID)
			}
		}

		// Close the parent item.
		p := closePayload{
			Target:     item.MsgID,
			Resolution: "cancelled",
			Reason:     reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}
		tags := []string{"work:close", "work:resolution:cancelled"}
		antecedents := []string{item.MsgID}
		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"resolution":  "cancelled",
				"cascaded":    closedIDs,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		if len(closedIDs) > 0 {
			for _, childID := range closedIDs {
				fmt.Printf("closed %s (cancelled, cascaded)\n", childID)
			}
		}
		fmt.Printf("closed %s (cancelled)\n", item.ID)
		return nil
	},
}

// deferCmd sends a work:update with a new ETA (defers an item).
var deferCmd = &cobra.Command{
	Use:   "defer <item-id>",
	Short: "Defer a work item to a later ETA",
	Long: `Defer a work item by updating its ETA.

Sends a work:update message with the new ETA. Supports relative time formats:
  2h        → now + 2 hours
  3d        → now + 3 days
  tomorrow  → next day 09:00 UTC
  next week → next Monday 09:00 UTC
  RFC3339   → absolute time passthrough
  YYYY-MM-DD → that date 09:00 UTC

Example:
  rd defer ready-a1b --eta 2h
  rd defer ready-a1b --eta "next week"
  rd defer ready-a1b --eta 2026-04-01`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		etaExpr, _ := cmd.Flags().GetString("eta")

		if etaExpr == "" {
			return fmt.Errorf("--eta is required")
		}

		// Parse the ETA expression.
		etaRFC3339, err := timeparse.Parse(etaExpr, time.Now())
		if err != nil {
			return fmt.Errorf("invalid --eta: %w", err)
		}

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		item, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		p := updatePayload{
			Target: item.MsgID,
			ETA:    etaRFC3339,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		tags := []string{"work:update"}
		antecedents := []string{item.MsgID}
		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"eta":         etaRFC3339,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("deferred %s → %s\n", item.ID, etaRFC3339)
		return nil
	},
}

// progressCmd appends notes to an item's context field.
var progressCmd = &cobra.Command{
	Use:   "progress <item-id>",
	Short: "Append a progress note to a work item",
	Long: `Append notes to a work item's context field.

Sends a work:update message that appends --notes to the existing context.
This provides an audit trail of progress without overwriting prior context.

Example:
  rd progress ready-a1b --notes "Completed auth module, starting on UI"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		notes, _ := cmd.Flags().GetString("notes")

		if notes == "" {
			return fmt.Errorf("--notes is required")
		}

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		item, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		// Append notes to existing context with a timestamp separator.
		now := time.Now().UTC().Format("2006-01-02T15:04Z")
		newContext := item.Context
		if newContext != "" {
			newContext = newContext + "\n\n[" + now + "] " + notes
		} else {
			newContext = "[" + now + "] " + notes
		}

		p := updatePayload{
			Target:  item.MsgID,
			Context: newContext,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		tags := []string{"work:update"}
		antecedents := []string{item.MsgID}
		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"context":     newContext,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("progress noted on %s\n", item.ID)
		return nil
	},
}

// runCloseAlias returns a RunE function that closes an item with the given resolution.
func runCloseAlias(resolution string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		reason, _ := cmd.Flags().GetString("reason")

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		item, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		p := closePayload{
			Target:     item.MsgID,
			Resolution: resolution,
			Reason:     reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		tags := []string{"work:close", "work:resolution:" + resolution}
		antecedents := []string{item.MsgID}
		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"resolution":  resolution,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("closed %s (%s)\n", item.ID, resolution)
		return nil
	}
}

func init() {
	doneCmd.Flags().String("reason", "", "reason for closing")
	rootCmd.AddCommand(doneCmd)

	failCmd.Flags().String("reason", "", "reason for closing")
	rootCmd.AddCommand(failCmd)

	cancelCmd.Flags().String("reason", "", "reason for closing")
	cancelCmd.Flags().Bool("cascade", false, "also close all open children")
	rootCmd.AddCommand(cancelCmd)

	deferCmd.Flags().String("eta", "", "new ETA: 2h, 3d, tomorrow, next week, RFC3339, or YYYY-MM-DD")
	rootCmd.AddCommand(deferCmd)

	progressCmd.Flags().String("notes", "", "progress notes to append to context")
	rootCmd.AddCommand(progressCmd)
}

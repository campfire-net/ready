package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/3dl-dev/ready/pkg/resolve"
	"github.com/3dl-dev/ready/pkg/state"
)

// closePayload is the JSON payload for a work:close message.
type closePayload struct {
	Target     string `json:"target"`
	Resolution string `json:"resolution"`
	Reason     string `json:"reason,omitempty"`
}

var closeCmd = &cobra.Command{
	Use:   "close <item-id>",
	Short: "Close a work item",
	Long: `Close a work item with a resolution.

Resolution must be one of: done, cancelled, failed (default: done).

Example:
  rd close ready-a1b --reason "Implemented and merged"
  rd close ready-a1b --resolution cancelled --reason "No longer needed"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		resolution, _ := cmd.Flags().GetString("resolution")
		reason, _ := cmd.Flags().GetString("reason")

		if resolution == "" {
			resolution = "done"
		}

		validResolutions := map[string]bool{"done": true, "cancelled": true, "failed": true}
		if !validResolutions[resolution] {
			return fmt.Errorf("invalid --resolution %q: must be done, cancelled, or failed", resolution)
		}

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

		// Check not already terminal.
		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		// Build payload.
		p := closePayload{
			Target:     item.MsgID,
			Resolution: resolution,
			Reason:     reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		// Tags.
		tags := []string{"work:close", "work:resolution:" + resolution}

		// Antecedents: the work:create message.
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
	},
}

func init() {
	closeCmd.Flags().String("resolution", "done", "resolution: done, cancelled, failed")
	closeCmd.Flags().String("reason", "", "reason for closing")
	rootCmd.AddCommand(closeCmd)
}

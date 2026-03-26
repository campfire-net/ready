package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
	"github.com/third-division/ready/pkg/state"
)

// gateResolvePayload is the JSON payload for a work:gate-resolve message.
// Convention §4.9.
type gateResolvePayload struct {
	Target     string `json:"target"`
	Resolution string `json:"resolution"`
	Reason     string `json:"reason,omitempty"`
}

var approveCmd = &cobra.Command{
	Use:   "approve <item-id>",
	Short: "Approve a pending gate",
	Long: `Approve a pending gate escalation. Transitions the item back to active.

The item must be in waiting status with an unfulfilled gate. Sends a
work:gate-resolve message with resolution=approved, targeting the gate message.

Convention §4.9: approved → item transitions to active.

Example:
  rd approve ready-a1b --reason "Approved, proceed with design approach"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		reason, _ := cmd.Flags().GetString("reason")

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

		// Check item has an unfulfilled gate.
		if item.GateMsgID == "" {
			return fmt.Errorf("item %s has no pending gate to approve", item.ID)
		}
		if item.Status != state.StatusWaiting {
			return fmt.Errorf("item %s is not waiting (status=%s)", item.ID, item.Status)
		}

		// Build payload — target is the work:gate message ID.
		p := gateResolvePayload{
			Target:     item.GateMsgID,
			Resolution: "approved",
			Reason:     reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		// Tags: operation tag + resolution tag per convention §4.9.
		tags := []string{"work:gate-resolve", "work:resolution:approved"}

		// Antecedents: the gate message (--fulfills implies --reply-to per convention).
		antecedents := []string{item.GateMsgID}

		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"resolution":  "approved",
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("approved gate for %s\n", item.ID)
		return nil
	},
}

func init() {
	approveCmd.Flags().String("reason", "", "reason for approving")
	rootCmd.AddCommand(approveCmd)
}

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/third-division/ready/pkg/resolve"
	"github.com/third-division/ready/pkg/state"
)

var rejectCmd = &cobra.Command{
	Use:   "reject <item-id>",
	Short: "Reject a pending gate",
	Long: `Reject a pending gate escalation. Item remains in waiting status.

The item must be in waiting status with an unfulfilled gate. Sends a
work:gate-resolve message with resolution=rejected.

Convention §4.9: rejected → item remains waiting. The by party should revise
their approach and either resume (work:status → active) or re-gate with a new
question.

Example:
  rd reject ready-a1b --reason "Scope too broad, need to split into smaller pieces"`,
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
			return fmt.Errorf("item %s has no pending gate to reject", item.ID)
		}
		if item.Status != state.StatusWaiting {
			return fmt.Errorf("item %s is not waiting (status=%s)", item.ID, item.Status)
		}

		// Build payload — target is the work:gate message ID.
		p := gateResolvePayload{
			Target:     item.GateMsgID,
			Resolution: "rejected",
			Reason:     reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		// Tags: operation tag + resolution tag per convention §4.9.
		tags := []string{"work:gate-resolve", "work:resolution:rejected"}

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
				"resolution":  "rejected",
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("rejected gate for %s\n", item.ID)
		return nil
	},
}

func init() {
	rejectCmd.Flags().String("reason", "", "reason for rejecting")
	rootCmd.AddCommand(rejectCmd)
}

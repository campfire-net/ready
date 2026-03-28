package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/state"
)

// claimPayload is the JSON payload for a work:claim message.
type claimPayload struct {
	Target string `json:"target"`
	Reason string `json:"reason,omitempty"`
}

var claimCmd = &cobra.Command{
	Use:   "claim <item-id>",
	Short: "Claim a work item",
	Long: `Claim a work item — accept delegation and transition to active.

Sets by=sender and transitions the item to active status.

Example:
  rd claim ready-a1b
  rd claim ready-a1b --reason "Picking this up now"`,
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
		item, err := byIDFromJSONLOrStore(s, itemID)
		if err != nil {
			return err
		}

		// Check not already terminal.
		if state.IsTerminal(item) {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		// Build payload — target is the work:create message ID.
		p := claimPayload{
			Target: item.MsgID,
			Reason: reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		// Tags: exactly one operation tag per convention §4.1.
		tags := []string{"work:claim"}

		// Antecedents: the work:create message (convention §4.3: exactly_one(target)).
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
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("claimed %s\n", item.ID)
		return nil
	},
}

func init() {
	claimCmd.Flags().String("reason", "", "reason for claiming")
	rootCmd.AddCommand(claimCmd)
}

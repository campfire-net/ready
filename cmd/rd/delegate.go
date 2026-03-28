package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/state"
)

// delegatePayload is the JSON payload for a work:delegate message.
type delegatePayload struct {
	Target string `json:"target"`
	To     string `json:"to"`
	From   string `json:"from,omitempty"`
	Reason string `json:"reason,omitempty"`
}

var delegateCmd = &cobra.Command{
	Use:   "delegate <item-id>",
	Short: "Delegate a work item to another party",
	Long: `Delegate a work item — assign or reassign the performer.

The --to flag is required and specifies the delegatee identity.
Identity types:
  - Person:           baron@3dl.dev
  - Claude agent:     claude-session-xyz
  - Open agent:       cf://agents/implementer
  - Rudi automaton:   atlas/worker-3

Example:
  rd delegate ready-a1b --to baron@3dl.dev
  rd delegate ready-a1b --to atlas/worker-3 --reason "Routing to automaton"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		to, _ := cmd.Flags().GetString("to")
		reason, _ := cmd.Flags().GetString("reason")

		if to == "" {
			return fmt.Errorf("--to is required")
		}

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
		p := delegatePayload{
			Target: item.MsgID,
			To:     to,
			From:   item.By,
			Reason: reason,
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("encoding payload: %w", err)
		}

		// Tags: operation tag + by identity tag per convention §4.1.
		tags := []string{"work:delegate", "work:by:" + to}

		// Antecedents: the work:create message (convention §4.5: exactly_one(target)).
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
				"to":          to,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("delegated %s to %s\n", item.ID, to)
		return nil
	},
}

func init() {
	delegateCmd.Flags().String("to", "", "identity to delegate to (required)")
	delegateCmd.Flags().String("reason", "", "reason for delegation")
	rootCmd.AddCommand(delegateCmd)
}

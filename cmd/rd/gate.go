package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/state"
)

// gatePayload is the JSON payload for a work:gate message.
// Convention §4.8.
type gatePayload struct {
	Target      string `json:"target"`
	GateType    string `json:"gate_type"`
	Description string `json:"description,omitempty"`
}

// validGateTypes is the set of gate types defined in the convention spec §4.8.
var validGateTypes = map[string]bool{
	"budget":   true,
	"design":   true,
	"scope":    true,
	"review":   true,
	"human":    true,
	"stall":    true,
	"periodic": true,
}

// BuildGatePayload constructs the JSON payload, tags, and antecedents for a
// work:gate message per convention §4.8. The targetMsgID is the work:create
// message ID of the item being gated. The gateType must be a valid gate type
// from validGateTypes. Description is optional.
func BuildGatePayload(targetMsgID, gateType, description string) ([]byte, []string, []string, error) {
	p := gatePayload{
		Target:      targetMsgID,
		GateType:    gateType,
		Description: description,
	}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding payload: %w", err)
	}
	tags := []string{"work:gate", "work:gate-type:" + gateType}
	antecedents := []string{targetMsgID}
	return payloadBytes, tags, antecedents, nil
}

var gateCmd = &cobra.Command{
	Use:   "gate <item-id>",
	Short: "Request human escalation on a work item",
	Long: `Request human escalation. Transitions the item to waiting (waiting_type=gate).

Gate types: budget, design, scope, review, human, stall, periodic

The human must approve or reject the gate before work can proceed.
Use 'rd approve <item-id>' or 'rd reject <item-id>' to resolve.

Note: In a full implementation this would be sent as --future so the agent can
block on 'cf await' until the human resolves it. This requires futures transport
support (TODO: add --future flag when campfire transport supports cf await).

Example:
  rd gate ready-a1b --gate-type design --description "Confirm approach before implementing"
  rd gate ready-a1b --gate-type budget --description "Approve spend of $500"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]
		gateType, _ := cmd.Flags().GetString("gate-type")
		description, _ := cmd.Flags().GetString("description")

		if gateType == "" {
			return fmt.Errorf("--gate-type is required: choose from budget, design, scope, review, human, stall, periodic")
		}
		if !validGateTypes[gateType] {
			return fmt.Errorf("invalid --gate-type %q: must be budget, design, scope, review, human, stall, or periodic", gateType)
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

		// Build payload, tags, and antecedents via extracted function per §4.8.
		payloadBytes, tags, antecedents, err := BuildGatePayload(item.MsgID, gateType, description)
		if err != nil {
			return err
		}

		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"gate_type":   gateType,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("gate sent for %s (%s)\n", item.ID, gateType)
		return nil
	},
}

func init() {
	gateCmd.Flags().String("gate-type", "", "gate type: budget, design, scope, review, human, stall, periodic")
	gateCmd.Flags().String("description", "", "description of what needs human review")
	rootCmd.AddCommand(gateCmd)
}

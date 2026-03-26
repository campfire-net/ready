package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/3dl-dev/ready/pkg/resolve"
	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/timeparse"
)

// updatePayload is the JSON payload for a work:update message.
type updatePayload struct {
	Target   string `json:"target"`
	Title    string `json:"title,omitempty"`
	Context  string `json:"context,omitempty"`
	Priority string `json:"priority,omitempty"`
	ETA      string `json:"eta,omitempty"`
	Due      string `json:"due,omitempty"`
	Level    string `json:"level,omitempty"`
}

// updateStatusPayload is the JSON payload for a work:status message sent by rd update.
type updateStatusPayload struct {
	Target      string `json:"target"`
	To          string `json:"to"`
	Reason      string `json:"reason,omitempty"`
	WaitingOn   string `json:"waiting_on,omitempty"`
	WaitingType string `json:"waiting_type,omitempty"`
}

// statusAliases maps bd-style status names to canonical rd status values.
// These aliases exist to ease migration from bd to rd.
var statusAliases = map[string]string{
	"in_progress": "active",
	"in-progress": "active",
	"open":        "inbox",
	"closed":      "done",
	"completed":   "done",
}

var updateCmd = &cobra.Command{
	Use:   "update <item-id>",
	Short: "Update fields on a work item",
	Long: `Update one or more mutable fields on a work item.

Field flags: --title, --context, --priority, --eta, --due
Status flags: --status, --waiting-on, --waiting-type (auto-sets status=waiting when --waiting-on is used)
Note flag:    --note (used as reason for status transitions)

Examples:
  rd update ready-a1b --priority p0 --eta 2026-04-01T12:00:00Z
  rd update ready-a1b --title "New title" --context "Updated context"
  rd update ready-a1b --status waiting --waiting-on "vendor quote" --waiting-type vendor
  rd update ready-a1b --waiting-on "design review" --waiting-type person`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Helpful redirect for --blocks (agents may try bd-style dep wiring via update).
		if blocks, _ := cmd.Flags().GetString("blocks"); blocks != "" {
			return fmt.Errorf("--blocks is not a flag on rd update. Use: rd dep add <this-item> %s", blocks)
		}

		itemID := args[0]
		title, _ := cmd.Flags().GetString("title")
		context, _ := cmd.Flags().GetString("context")
		priority, _ := cmd.Flags().GetString("priority")
		eta, _ := cmd.Flags().GetString("eta")
		due, _ := cmd.Flags().GetString("due")
		level, _ := cmd.Flags().GetString("level")
		statusTo, _ := cmd.Flags().GetString("status")
		waitingOn, _ := cmd.Flags().GetString("waiting-on")
		waitingType, _ := cmd.Flags().GetString("waiting-type")
		note, _ := cmd.Flags().GetString("note")
		claim, _ := cmd.Flags().GetBool("claim")

		// --claim alone implies --status active (bd-compat: bd update --claim sets active).
		if claim && statusTo == "" {
			statusTo = state.StatusActive
		}

		// Auto-set status=waiting if --waiting-on is set without --status.
		if waitingOn != "" && statusTo == "" {
			statusTo = state.StatusWaiting
		}

		// Validate that at least one flag is set.
		hasFieldUpdate := title != "" || context != "" || priority != "" ||
			eta != "" || due != "" || level != ""
		hasStatusUpdate := statusTo != "" || waitingOn != ""

		if !hasFieldUpdate && !hasStatusUpdate && !claim {
			return fmt.Errorf("no fields to update: specify at least one of --title, --context, --priority, --eta, --due, --level, --status, --waiting-on, --claim")
		}

		// Validate priority if set.
		if priority != "" {
			validPriorities := map[string]bool{"p0": true, "p1": true, "p2": true, "p3": true}
			if !validPriorities[priority] {
				return fmt.Errorf("invalid --priority %q: must be one of p0, p1, p2, p3", priority)
			}
		}

		// Resolve status aliases (bd-compat).
		if statusTo != "" {
			if canonical, ok := statusAliases[statusTo]; ok {
				fmt.Fprintf(os.Stderr, "warning: status %q is a bd alias — using %q instead\n", statusTo, canonical)
				statusTo = canonical
			}
		}

		// Validate status if set.
		if statusTo != "" {
			validStatuses := map[string]bool{
				"inbox": true, "active": true, "scheduled": true, "waiting": true,
				"done": true, "cancelled": true, "failed": true,
			}
			if !validStatuses[statusTo] {
				return fmt.Errorf("invalid --status %q: must be one of inbox, active, scheduled, waiting, done, cancelled, failed", statusTo)
			}
		}

		// Validate waiting_type if set.
		if waitingType != "" {
			validWaitingTypes := map[string]bool{
				"person": true, "vendor": true, "client": true, "date": true,
				"event": true, "external": true, "agent": true, "gate": true,
			}
			if !validWaitingTypes[waitingType] {
				return fmt.Errorf("invalid --waiting-type %q: must be one of person, vendor, client, date, event, external, agent, gate", waitingType)
			}
		}

		// Validate level if set.
		if level != "" {
			validLevels := map[string]bool{"epic": true, "task": true, "subtask": true}
			if !validLevels[level] {
				return fmt.Errorf("invalid --level %q: must be one of epic, task, subtask", level)
			}
		}

		// Normalize ETA/due to UTC if provided.
		if eta != "" {
			normalized, err := timeparse.Parse(eta, time.Now())
			if err != nil {
				return fmt.Errorf("invalid --eta: %w", err)
			}
			eta = normalized
		}
		if due != "" {
			normalized, err := timeparse.Parse(due, time.Now())
			if err != nil {
				return fmt.Errorf("invalid --due: %w", err)
			}
			due = normalized
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

		// Check not already terminal for non-terminal operations.
		if state.IsTerminal(item) && hasFieldUpdate {
			return fmt.Errorf("item %s is already %s", item.ID, item.Status)
		}

		var lastMsgID string
		var lastCampfireID string

		// Send work:update if field updates are requested.
		if hasFieldUpdate {
			p := updatePayload{
				Target:   item.MsgID,
				Title:    title,
				Context:  context,
				Priority: priority,
				ETA:      eta,
				Due:      due,
				Level:    level,
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
			lastMsgID = msg.ID
			lastCampfireID = campfireID
		}

		// Send work:status if a status transition is requested.
		if hasStatusUpdate {
			sp := updateStatusPayload{
				Target:      item.MsgID,
				To:          statusTo,
				Reason:      note,
				WaitingOn:   waitingOn,
				WaitingType: waitingType,
			}
			payloadBytes, err := json.Marshal(sp)
			if err != nil {
				return fmt.Errorf("encoding status payload: %w", err)
			}

			tags := []string{"work:status", "work:status:" + statusTo}
			antecedents := []string{item.MsgID}

			msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
			if err != nil {
				return err
			}
			lastMsgID = msg.ID
			lastCampfireID = campfireID
		}

		// Send work:claim if --claim is set.
		if claim {
			cp := claimPayload{
				Target: item.MsgID,
			}
			payloadBytes, err := json.Marshal(cp)
			if err != nil {
				return fmt.Errorf("encoding claim payload: %w", err)
			}

			tags := []string{"work:claim"}
			antecedents := []string{item.MsgID}

			msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
			if err != nil {
				return err
			}
			lastMsgID = msg.ID
			lastCampfireID = campfireID
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          item.ID,
				"msg_id":      lastMsgID,
				"campfire_id": lastCampfireID,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("updated %s\n", item.ID)
		return nil
	},
}

func init() {
	updateCmd.Flags().String("title", "", "new title")
	updateCmd.Flags().String("context", "", "new context/description")
	updateCmd.Flags().String("priority", "", "priority: p0, p1, p2, p3")
	updateCmd.Flags().String("eta", "", "ETA in RFC3339 format")
	updateCmd.Flags().String("due", "", "hard deadline in RFC3339 format")
	updateCmd.Flags().String("level", "", "level: epic, task, subtask")
	updateCmd.Flags().String("status", "", "status: inbox, active, scheduled, waiting, done, cancelled, failed")
	updateCmd.Flags().String("waiting-on", "", "what we are waiting on (auto-sets status=waiting if no --status given)")
	updateCmd.Flags().String("waiting-type", "", "waiting type: person, vendor, client, date, event, external, agent, gate")
	updateCmd.Flags().String("note", "", "note or reason (used as reason for status transitions)")
	updateCmd.Flags().String("blocks", "", "")
	_ = updateCmd.Flags().MarkHidden("blocks")
	updateCmd.Flags().Bool("claim", false, "claim the item: set by=sender and transition to active (bd-compat: bd update --claim)")
	rootCmd.AddCommand(updateCmd)
}

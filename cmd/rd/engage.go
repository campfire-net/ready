package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/playbook"
)

// engagePayload is the JSON payload for a work:engage message.
type engagePayload struct {
	PlaybookID string            `json:"playbook_id"`
	Project    string            `json:"project,omitempty"`
	For        string            `json:"for"`
	Variables  map[string]string `json:"variables,omitempty"`
	// CreatedIDs is the list of item IDs created by this engagement.
	CreatedIDs []string `json:"created_ids"`
}

// engageCmd implements rd engage <playbook-id> --project <project> --for <identity> --var key=value.
var engageCmd = &cobra.Command{
	Use:   "engage <playbook-id>",
	Short: "Instantiate a playbook into work items",
	Long: `Instantiate a playbook template into concrete work items.

The engage command:
  1. Finds the playbook by ID
  2. Generates unique item IDs (<project>-<random-3-chars>)
  3. Applies variable substitutions to titles and contexts
  4. Creates work items (work:create for each)
  5. Wires dependencies (work:block for each dep edge)
  6. Records the engagement (work:engage)

Example:
  rd engage sre-incident --project myapp --for baron@3dl.dev --var project=myapp --var env=prod`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		playbookID := args[0]
		project, _ := cmd.Flags().GetString("project")
		forParty, _ := cmd.Flags().GetString("for")
		varFlags, _ := cmd.Flags().GetStringArray("var")

		if forParty == "" {
			return fmt.Errorf("--for is required")
		}
		if project == "" {
			return fmt.Errorf("--project is required")
		}

		// Parse --var key=value flags.
		variables := make(map[string]string, len(varFlags))
		for _, v := range varFlags {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --var %q: must be key=value", v)
			}
			variables[parts[0]] = parts[1]
		}

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Find the playbook.
		pb, err := findPlaybook(s, playbookID)
		if err != nil {
			return err
		}

		// Expand the template.
		items, err := playbook.Expand(pb.PlaybookTemplate, project, variables)
		if err != nil {
			return fmt.Errorf("expanding playbook: %w", err)
		}

		// Maps template index → msg ID of the work:create message sent.
		createMsgIDs := make(map[int]string, len(items))

		// Send work:create for each item.
		for _, item := range items {
			p := createPayload{
				ID:       item.ID,
				Title:    item.Title,
				Context:  item.Context,
				Type:     item.Type,
				Level:    item.Level,
				Project:  project,
				For:      forParty,
				Priority: item.Priority,
			}
			payloadBytes, err := json.Marshal(p)
			if err != nil {
				return fmt.Errorf("encoding work:create for %s: %w", item.ID, err)
			}

			tags := []string{"work:create", "work:type:" + item.Type, "work:for:" + forParty, "work:priority:" + item.Priority}
			if item.Level != "" {
				tags = append(tags, "work:level:"+item.Level)
			}
			if project != "" {
				tags = append(tags, "work:project:"+project)
			}

			msg, _, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, nil)
			if err != nil {
				return fmt.Errorf("sending work:create for %s: %w", item.ID, err)
			}
			createMsgIDs[item.TemplateIndex] = msg.ID
		}

		// Send work:block for each dependency edge.
		for _, item := range items {
			for _, depID := range item.Deps {
				// Find the dep item to get its msg ID.
				var depItem *playbook.ExpandedItem
				for _, other := range items {
					if other.ID == depID {
						depItem = other
						break
					}
				}
				if depItem == nil {
					return fmt.Errorf("internal: dep item %q not found", depID)
				}

				// item is blocked by depItem (depItem must complete first).
				// Convention: work:block payload has blocker_id and blocked_id.
				bp := blockPayload{
					BlockerID:  depItem.ID,
					BlockedID:  item.ID,
					BlockerMsg: createMsgIDs[depItem.TemplateIndex],
					BlockedMsg: createMsgIDs[item.TemplateIndex],
				}
				bpBytes, err := json.Marshal(bp)
				if err != nil {
					return fmt.Errorf("encoding work:block: %w", err)
				}
				antecedents := []string{bp.BlockerMsg, bp.BlockedMsg}
				_, _, err = sendToProjectCampfire(agentID, s, string(bpBytes), []string{"work:block"}, antecedents)
				if err != nil {
					return fmt.Errorf("sending work:block for %s→%s: %w", depItem.ID, item.ID, err)
				}
			}
		}

		// Collect all created item IDs.
		createdIDs := make([]string, len(items))
		for i, item := range items {
			createdIDs[i] = item.ID
		}

		// Send the work:engage message.
		if len(variables) == 0 {
			variables = nil
		}
		ep := engagePayload{
			PlaybookID: playbookID,
			Project:    project,
			For:        forParty,
			Variables:  variables,
			CreatedIDs: createdIDs,
		}
		epBytes, err := json.Marshal(ep)
		if err != nil {
			return fmt.Errorf("encoding work:engage: %w", err)
		}
		engageTags := []string{
			"work:engage",
			"work:playbook:" + playbookID,
		}
		engageMsg, campfireID, err := sendToProjectCampfire(agentID, s, string(epBytes), engageTags, nil)
		if err != nil {
			return fmt.Errorf("sending work:engage: %w", err)
		}

		if jsonOutput {
			out := map[string]interface{}{
				"playbook_id": playbookID,
				"project":     project,
				"for":         forParty,
				"created_ids": createdIDs,
				"engage_msg":  engageMsg.ID,
				"campfire_id": campfireID,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		// Human-readable output: print the created item tree.
		fmt.Printf("engaged playbook %s → %d items\n\n", playbookID, len(items))
		for _, item := range items {
			depStr := ""
			if len(item.Deps) > 0 {
				depStr = fmt.Sprintf("  (blocked by: %s)", strings.Join(item.Deps, ", "))
			}
			fmt.Printf("  %-16s  %-6s  %s%s\n", item.ID, item.Priority, item.Title, depStr)
		}
		return nil
	},
}

func init() {
	engageCmd.Flags().String("project", "", "project prefix for generated item IDs (required)")
	engageCmd.Flags().String("for", "", "who needs these outcomes (required)")
	engageCmd.Flags().StringArray("var", nil, "variable substitution: key=value (may be repeated)")
	rootCmd.AddCommand(engageCmd)
}

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/timeparse"
)

var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9]+`)

// projectPrefix returns the sanitized folder name of the project directory
// as an ID prefix (e.g. "/home/baron/projects/ready" → "ready").
func projectPrefix(projectDir string) string {
	name := filepath.Base(projectDir)
	name = nonAlphanumHyphen.ReplaceAllString(name, "")
	if len(name) >= 2 {
		return name
	}
	return ""
}

// generateID returns the shortest ID that doesn't collide with any existing
// item ID, with a minimum of 3 hex characters. If a prefix is provided it is
// prepended as "<prefix>-<hex>".
func generateID(prefix string, existingIDs map[string]struct{}) (string, error) {
	b := make([]byte, 8) // 16 hex chars — enough headroom
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating id: %w", err)
	}
	full := hex.EncodeToString(b)
	for length := 3; length <= len(full); length++ {
		var candidate string
		if prefix != "" {
			candidate = prefix + "-" + full[:length]
		} else {
			candidate = full[:length]
		}
		if _, collision := existingIDs[candidate]; !collision {
			return candidate, nil
		}
	}
	if prefix != "" {
		return prefix + "-" + full, nil
	}
	return full, nil
}

// createPayload is the JSON payload for a work:create message.
type createPayload struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Context  string `json:"context,omitempty"`
	Type     string `json:"type"`
	Level    string `json:"level,omitempty"`
	Project  string `json:"project,omitempty"`
	For      string `json:"for"`
	By       string `json:"by,omitempty"`
	Priority string `json:"priority"`
	ParentID string `json:"parent_id,omitempty"`
	ETA      string `json:"eta,omitempty"`
	Due      string `json:"due,omitempty"`
}

// BuildCreatePayload constructs the JSON payload and tag set for a work:create
// message. All validation must happen before calling this function.
// The etaStr and dueStr must already be normalized to RFC3339 UTC if non-empty.
func BuildCreatePayload(id, title, context, itemType, level, project, forParty, by, priority, parentID, etaStr, dueStr string) ([]byte, []string, error) {
	p := createPayload{
		ID:       id,
		Title:    title,
		Context:  context,
		Type:     itemType,
		Level:    level,
		Project:  project,
		For:      forParty,
		By:       by,
		Priority: priority,
		ParentID: parentID,
		ETA:      etaStr,
		Due:      dueStr,
	}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return nil, nil, fmt.Errorf("encoding payload: %w", err)
	}

	tags := []string{"work:create"}
	tags = append(tags, "work:type:"+itemType)
	tags = append(tags, "work:for:"+forParty)
	tags = append(tags, "work:priority:"+priority)
	if level != "" {
		tags = append(tags, "work:level:"+level)
	}
	if by != "" {
		tags = append(tags, "work:by:"+by)
	}
	if project != "" {
		tags = append(tags, "work:project:"+project)
	}

	return payloadBytes, tags, nil
}

var createCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create a new work item",
	Long: `Create a new work item in the project campfire.

Title can be a positional argument or --title flag.
Required: title, --type, --priority

If --eta is omitted, it is derived from priority:
  p0 = now, p1 = +4h, p2 = +24h, p3 = +72h

Example:
  rd create "Fix auth bug" --type task --priority p0
  rd create --title "Fix auth bug" --type task --priority p0
  rd create "Review API design" --type decision --priority p1 --for baron@3dl.dev
  rd create "Ship v2" --type task --priority p1 --context "See spec in docs/v2.md" --json

Note: use --context for descriptions, not --description.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Helpful redirect for --description (agents may use bd's flag name).
		if desc, _ := cmd.Flags().GetString("description"); desc != "" {
			return fmt.Errorf("--description is not a flag on rd create. The field is called 'context' in Ready. Use --context-file <path> or set context after creation with rd update")
		}

		id, _ := cmd.Flags().GetString("id")
		title, _ := cmd.Flags().GetString("title")
		context, _ := cmd.Flags().GetString("context")
		itemType, _ := cmd.Flags().GetString("type")
		level, _ := cmd.Flags().GetString("level")
		project, _ := cmd.Flags().GetString("project")
		forParty, _ := cmd.Flags().GetString("for")
		by, _ := cmd.Flags().GetString("by")
		priority, _ := cmd.Flags().GetString("priority")
		parentID, _ := cmd.Flags().GetString("parent-id")
		eta, _ := cmd.Flags().GetString("eta")
		due, _ := cmd.Flags().GetString("due")

		// Title: positional arg or --title flag, not both.
		if len(args) > 0 && title != "" {
			return fmt.Errorf("title provided as both positional argument and --title flag; use one or the other")
		}
		if len(args) > 0 {
			title = strings.Join(args, " ")
		}

		// Validation.
		if title == "" {
			return fmt.Errorf("title is required (positional argument or --title flag)")
		}
		if itemType == "" {
			return fmt.Errorf("--type is required")
		}
		if priority == "" {
			return fmt.Errorf("--priority is required")
		}

		// Validate type.
		validTypes := map[string]bool{
			"task": true, "decision": true, "review": true, "reminder": true,
			"deadline": true, "prep": true, "message": true, "directive": true,
		}
		if !validTypes[itemType] {
			return fmt.Errorf("invalid --type %q: must be one of task, decision, review, reminder, deadline, prep, message, directive", itemType)
		}

		// Validate priority.
		validPriorities := map[string]bool{"p0": true, "p1": true, "p2": true, "p3": true}
		if !validPriorities[priority] {
			return fmt.Errorf("invalid --priority %q: must be one of p0, p1, p2, p3", priority)
		}

		// Validate level if provided.
		if level != "" {
			validLevels := map[string]bool{"epic": true, "task": true, "subtask": true}
			if !validLevels[level] {
				return fmt.Errorf("invalid --level %q: must be one of epic, task, subtask", level)
			}
		}

		// Normalize ETA to UTC if provided.
		if eta != "" {
			normalized, err := timeparse.Parse(eta, time.Now())
			if err != nil {
				return fmt.Errorf("invalid --eta: %w", err)
			}
			eta = normalized
		}
		// Normalize due to UTC if provided.
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

		// Default --for to the current session identity when not explicitly set.
		if !cmd.Flags().Changed("for") {
			forParty = agentID.PublicKeyHex()
		} else if forParty == "" {
			return fmt.Errorf("--for: value cannot be empty")
		}

		// Load existing IDs for collision detection.
		campfireID, projectDir, hasCampfire := projectRoot()
		existingIDs := map[string]struct{}{}
		if hasCampfire {
			if items, err := state.DeriveFromStore(s, campfireID); err == nil {
				for k := range items {
					existingIDs[k] = struct{}{}
				}
			}
		}

		if id == "" {
			prefix := ""
			if hasCampfire {
				prefix = projectPrefix(projectDir)
			}
			generated, err := generateID(prefix, existingIDs)
			if err != nil {
				return err
			}
			id = generated
		} else if _, collision := existingIDs[id]; collision {
			return fmt.Errorf("item %q already exists", id)
		}

		// Build payload and tags via extracted function.
		payloadBytes, tags, err := BuildCreatePayload(id, title, context, itemType, level, project, forParty, by, priority, parentID, eta, due)
		if err != nil {
			return err
		}

		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, nil)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"id":          id,
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"title":       title,
				"type":        itemType,
				"priority":    priority,
				"for":         forParty,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("created %s (msg: %s)\n", id, msg.ID)
		return nil
	},
}

func init() {
	createCmd.Flags().String("id", "", "item ID (default: auto-generated)")
	createCmd.Flags().String("title", "", "short title (required)")
	createCmd.Flags().String("context", "", "full context / description")
	createCmd.Flags().String("type", "", "type: task, decision, review, reminder, deadline, prep, message, directive (required)")
	createCmd.Flags().String("for", "", "who needs this outcome (default: current identity)")
	createCmd.Flags().String("priority", "", "priority: p0, p1, p2, p3 (required)")
	createCmd.Flags().String("level", "", "level: epic, task, subtask")
	createCmd.Flags().String("by", "", "who will do the work")
	createCmd.Flags().String("project", "", "project name")
	createCmd.Flags().String("parent-id", "", "parent item ID")
	createCmd.Flags().String("eta", "", "ETA in RFC3339 format (default: derived from priority)")
	createCmd.Flags().String("due", "", "hard deadline in RFC3339 format")
	createCmd.Flags().String("description", "", "")
	_ = createCmd.Flags().MarkHidden("description")
	rootCmd.AddCommand(createCmd)
}

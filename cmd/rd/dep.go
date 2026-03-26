package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/campfire-net/campfire/pkg/store"
	"github.com/spf13/cobra"
	"github.com/3dl-dev/ready/pkg/resolve"
	"github.com/3dl-dev/ready/pkg/state"
)

// blockPayload is the JSON payload for a work:block message.
type blockPayload struct {
	BlockerID  string `json:"blocker_id"`
	BlockedID  string `json:"blocked_id"`
	BlockerMsg string `json:"blocker_msg"`
	BlockedMsg string `json:"blocked_msg"`
}

// depUnblockPayload is the JSON payload for a work:unblock message.
type depUnblockPayload struct {
	Target string `json:"target"`
	Reason string `json:"reason,omitempty"`
}

// BuildBlockPayload constructs the JSON payload, tags, and antecedents for a
// work:block message per convention §4.6. Both item message IDs are required as
// antecedents so the campfire protocol can establish causal ordering.
func BuildBlockPayload(blockerID, blockedID, blockerMsgID, blockedMsgID string) ([]byte, []string, []string, error) {
	p := blockPayload{
		BlockerID:  blockerID,
		BlockedID:  blockedID,
		BlockerMsg: blockerMsgID,
		BlockedMsg: blockedMsgID,
	}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding payload: %w", err)
	}
	tags := []string{"work:block"}
	antecedents := []string{blockerMsgID, blockedMsgID}
	return payloadBytes, tags, antecedents, nil
}

// BuildUnblockPayload constructs the JSON payload, tags, and antecedents for a
// work:unblock message per convention §4.7. The target is the work:block message ID
// (not an item ID). The antecedent is the block message being reversed.
func BuildUnblockPayload(blockMsgID, reason string) ([]byte, []string, []string, error) {
	p := depUnblockPayload{
		Target: blockMsgID,
		Reason: reason,
	}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encoding payload: %w", err)
	}
	tags := []string{"work:unblock"}
	antecedents := []string{blockMsgID}
	return payloadBytes, tags, antecedents, nil
}

// depCmd is the parent command for dep subcommands.
var depCmd = &cobra.Command{
	Use:   "dep",
	Short: "Manage item dependencies",
	Long: `Manage dependencies between work items.

  rd dep add <blocked-id> <blocker-id>    wire a dependency
  rd dep remove <blocked-id> <blocker-id> remove a dependency
  rd dep tree <id>                        show dependency tree`,
}

// depAddCmd implements rd dep add <blocked-id> <blocker-id>.
var depAddCmd = &cobra.Command{
	Use:   "add <blocked-id> <blocker-id>",
	Short: "Wire a dependency: blocker-id blocks blocked-id",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		blockedArg := args[0]
		blockerArg := args[1]

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Resolve both items.
		blocked, err := resolve.ByID(s, blockedArg)
		if err != nil {
			return fmt.Errorf("resolving blocked item %q: %w", blockedArg, err)
		}
		blocker, err := resolve.ByID(s, blockerArg)
		if err != nil {
			return fmt.Errorf("resolving blocker item %q: %w", blockerArg, err)
		}

		// Build payload, tags, and antecedents via extracted function per §4.6.
		payloadBytes, tags, antecedents, err := BuildBlockPayload(blocker.ID, blocked.ID, blocker.MsgID, blocked.MsgID)
		if err != nil {
			return err
		}

		msg, campfireID, err := sendToProjectCampfire(agentID, s, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"msg_id":      msg.ID,
				"campfire_id": campfireID,
				"blocker_id":  blocker.ID,
				"blocked_id":  blocked.ID,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("blocked: %s is now blocked by %s\n", blocked.ID, blocker.ID)
		return nil
	},
}

// depRemoveCmd implements rd dep remove <blocked-id> <blocker-id>.
var depRemoveCmd = &cobra.Command{
	Use:   "remove <blocked-id> <blocker-id>",
	Short: "Remove a dependency between two items",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		blockedArg := args[0]
		blockerArg := args[1]
		reason, _ := cmd.Flags().GetString("reason")

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Resolve both items to get their canonical IDs.
		blocked, err := resolve.ByID(s, blockedArg)
		if err != nil {
			return fmt.Errorf("resolving blocked item %q: %w", blockedArg, err)
		}
		blocker, err := resolve.ByID(s, blockerArg)
		if err != nil {
			return fmt.Errorf("resolving blocker item %q: %w", blockerArg, err)
		}

		// Find the work:block message linking these two items.
		blockMsgID, campfireID, err := findBlockMessage(s, blocker.ID, blocked.ID)
		if err != nil {
			return err
		}

		// Build work:unblock payload via extracted function per §4.7.
		payloadBytes, tags, antecedents, err := BuildUnblockPayload(blockMsgID, reason)
		if err != nil {
			return err
		}

		// Send to the campfire that contains the block message.
		m, err := s.GetMembership(campfireID)
		if err != nil {
			return fmt.Errorf("querying membership: %w", err)
		}
		if m == nil {
			return fmt.Errorf("not a member of campfire %s", campfireID[:minInt(12, len(campfireID))])
		}
		msg, err := sendViaMembership(agentID, s, m, campfireID, string(payloadBytes), tags, antecedents)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := map[string]interface{}{
				"msg_id":        msg.ID,
				"campfire_id":   campfireID,
				"block_msg_id":  blockMsgID,
				"blocker_id":    blocker.ID,
				"blocked_id":    blocked.ID,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("unblocked: %s is no longer blocked by %s\n", blocked.ID, blocker.ID)
		return nil
	},
}

// depTreeCmd implements rd dep tree <id>.
var depTreeCmd = &cobra.Command{
	Use:   "tree <id>",
	Short: "Show the dependency tree rooted at an item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		itemID := args[0]

		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Resolve root item.
		root, err := resolve.ByID(s, itemID)
		if err != nil {
			return err
		}

		// Load all items from the same campfire for tree walking.
		memberships, err := s.ListMemberships()
		if err != nil {
			return fmt.Errorf("listing memberships: %w", err)
		}

		allItems := make(map[string]*state.Item)
		for _, m := range memberships {
			items, err := state.DeriveFromStore(s, m.CampfireID)
			if err != nil {
				continue
			}
			for id, item := range items {
				allItems[id] = item
			}
		}

		if jsonOutput {
			tree := buildDepTree(root.ID, allItems, map[string]bool{})
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(tree)
		}

		printDepTree(root, allItems, "", map[string]bool{})
		return nil
	},
}

// treeNode is used for JSON output of dep tree.
type treeNode struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Status   string      `json:"status"`
	Children []*treeNode `json:"children,omitempty"`
}

// buildDepTree builds a recursive tree for JSON output.
func buildDepTree(id string, items map[string]*state.Item, visited map[string]bool) *treeNode {
	item, ok := items[id]
	if !ok {
		return &treeNode{ID: id, Title: "(not found)", Status: "unknown"}
	}
	if visited[id] {
		return &treeNode{ID: id, Title: item.Title, Status: item.Status + " (cycle)"}
	}
	visited[id] = true
	node := &treeNode{ID: id, Title: item.Title, Status: item.Status}
	// Children are: items this one blocks (blocks list) + children by parent_id.
	seen := map[string]bool{}
	for _, childID := range item.Blocks {
		if !seen[childID] {
			seen[childID] = true
			node.Children = append(node.Children, buildDepTree(childID, items, visited))
		}
	}
	for _, child := range items {
		if child.ParentID == id && !seen[child.ID] {
			seen[child.ID] = true
			node.Children = append(node.Children, buildDepTree(child.ID, items, visited))
		}
	}
	delete(visited, id)
	return node
}

// printDepTree prints an indented dependency tree.
func printDepTree(item *state.Item, items map[string]*state.Item, prefix string, visited map[string]bool) {
	if visited[item.ID] {
		fmt.Printf("%s%s  [%s] (cycle detected)\n", prefix, item.ID, item.Status)
		return
	}
	visited[item.ID] = true

	// Format: <id>  [<status>]  <title>  (blocked by: X, Y)
	line := fmt.Sprintf("%s  [%s]  %s", item.ID, item.Status, item.Title)
	if len(item.BlockedBy) > 0 {
		line += fmt.Sprintf("  (blocked by: %s)", strings.Join(item.BlockedBy, ", "))
	}
	fmt.Println(prefix + line)

	// Child indentation.
	childPrefix := prefix + "  "

	// Show items this one blocks (dependency children).
	seen := map[string]bool{}
	for _, childID := range item.Blocks {
		if seen[childID] {
			continue
		}
		seen[childID] = true
		if child, ok := items[childID]; ok {
			fmt.Printf("%s└─ blocks: ", childPrefix)
			printDepTree(child, items, childPrefix+"   ", visited)
		} else {
			fmt.Printf("%s└─ blocks: %s  (not found)\n", childPrefix, childID)
		}
	}

	// Show child items by parent_id hierarchy.
	for _, child := range items {
		if child.ParentID == item.ID && !seen[child.ID] {
			seen[child.ID] = true
			fmt.Printf("%s└─ child:  ", childPrefix)
			printDepTree(child, items, childPrefix+"   ", visited)
		}
	}

	delete(visited, item.ID)
}

// findBlockMessage scans all campfire messages for a work:block message
// with the given blocker and blocked item IDs. Returns the block message ID
// and its campfire ID.
func findBlockMessage(s store.Store, blockerID, blockedID string) (string, string, error) {
	memberships, err := s.ListMemberships()
	if err != nil {
		return "", "", fmt.Errorf("listing memberships: %w", err)
	}

	for _, m := range memberships {
		msgs, err := s.ListMessages(m.CampfireID, 0, store.MessageFilter{})
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if !hasTagStr(msg.Tags, "work:block") {
				continue
			}
			var p blockPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				continue
			}
			if p.BlockerID == blockerID && p.BlockedID == blockedID {
				return msg.ID, m.CampfireID, nil
			}
		}
	}

	return "", "", fmt.Errorf("no work:block message found for %s → %s", blockerID, blockedID)
}

// hasTagStr reports whether tags contains the given tag string.
func hasTagStr(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func init() {
	depRemoveCmd.Flags().String("reason", "", "reason for removing the dependency")
	depCmd.AddCommand(depAddCmd)
	depCmd.AddCommand(depRemoveCmd)
	depCmd.AddCommand(depTreeCmd)
	rootCmd.AddCommand(depCmd)
}

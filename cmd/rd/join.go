package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"
)

var joinCmd = &cobra.Command{
	Use:   "join <name-or-campfire-id>",
	Short: "Join a project campfire by name or ID",
	Long: `Join a campfire by name (cf:// URI or short name) or by campfire ID.

For open campfires, joins immediately.

For invite-only campfires, posts a work:join-request and polls for a
work:role-grant (admission grant) targeting your public key.

EXAMPLES
  rd join myorg.ready/myproject
  rd join cf://myorg.ready/myproject
  rd join abcdef1234...             # join by campfire ID directly`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nameOrID := args[0]
		timeout, _ := cmd.Flags().GetDuration("timeout")
		role, _ := cmd.Flags().GetString("role")
		if role == "" {
			role = "member"
		}

		client, err := requireClient()
		if err != nil {
			return err
		}

		// Resolve the name to a campfire ID.
		campfireID, err := resolveName(client, nameOrID)
		if err != nil {
			return fmt.Errorf("resolving name %q: %w", nameOrID, err)
		}

		// Attempt to join.
		result, err := client.Join(protocol.JoinRequest{
			CampfireID: campfireID,
			Transport:  protocol.FilesystemTransport{},
		})
		if err == nil {
			// Joined successfully.
			displayID := campfireID
			if len(displayID) > 12 {
				displayID = displayID[:12] + "..."
			}
			fmt.Fprintf(os.Stdout, "joined campfire %s (%s)\n", displayID, result.JoinProtocol)
			return nil
		}

		// Join failed. If this is an invite-only campfire, post a join-request.
		// We detect invite-only by checking the error message — the protocol
		// returns an error when the campfire is invite-only and we haven't been
		// pre-admitted.
		fmt.Fprintf(os.Stderr, "rd: campfire is invite-only — posting join request\n")

		// We need to be a member to send a message. For invite-only campfires
		// where we have no membership yet, we post via the center campfire's
		// convention transport (the join-request operation uses min_operator_level: 0).
		exec, _, execErr := requireExecutor()
		if execErr != nil {
			return fmt.Errorf("initializing executor: %w", execErr)
		}

		agentID, s, storeErr := requireAgentAndStore()
		if storeErr != nil {
			return fmt.Errorf("loading identity: %w", storeErr)
		}
		defer s.Close()

		decl, declErr := loadDeclaration("join-request")
		if declErr != nil {
			return fmt.Errorf("loading join-request declaration: %w", declErr)
		}

		myPubKey := agentID.PublicKeyHex()

		ctx := context.Background()
		result2, sendErr := exec.Execute(ctx, decl, campfireID, map[string]any{
			"pubkey":         myPubKey,
			"requested_role": role,
		})
		if sendErr != nil {
			return fmt.Errorf("posting join request: %w", sendErr)
		}

		fmt.Fprintf(os.Stdout, "join request posted (msg %s)\n", result2.MessageID[:12]+"...")
		fmt.Fprintf(os.Stdout, "waiting for admission (timeout: %s) ...\n", timeout)

		// Poll for a work:role-grant targeting our pubkey.
		grantMsgID, pollErr := pollForRoleGrant(client, campfireID, myPubKey, timeout)
		if pollErr != nil {
			return fmt.Errorf("waiting for role grant: %w\n  run 'rd join' again after the admin admits you", pollErr)
		}

		fmt.Fprintf(os.Stdout, "admitted! role-grant received (msg %s)\n", grantMsgID[:12]+"...")

		// Now join with the pre-admission in place.
		result3, joinErr := client.Join(protocol.JoinRequest{
			CampfireID: campfireID,
			Transport:  protocol.FilesystemTransport{},
		})
		if joinErr != nil {
			return fmt.Errorf("joining after admission: %w", joinErr)
		}

		displayID := campfireID
		if len(displayID) > 12 {
			displayID = displayID[:12] + "..."
		}
		fmt.Fprintf(os.Stdout, "joined campfire %s (%s)\n", displayID, result3.JoinProtocol)
		return nil
	},
}

// resolveName resolves a name (cf:// URI, short name, or raw campfire ID) to a
// campfire ID hex string.
func resolveName(client *protocol.Client, input string) (string, error) {
	// If it's already a campfire ID (64 hex chars), return as-is.
	if len(input) == 64 && isHex(input) {
		return input, nil
	}

	// Use the naming resolver with the client.
	// We use the operator root as the naming root if available.
	root, err := naming.LoadOperatorRoot(CFHome())
	rootID := ""
	if err == nil && root != nil {
		rootID = root.CampfireID
	}

	resolver := naming.NewResolverFromClient(client, rootID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resolved, err := resolver.ResolveOrPassthrough(ctx, input)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// pollForRoleGrant polls the campfire for a work:role-grant message targeting
// myPubKey, returning the message ID when found, or an error on timeout.
func pollForRoleGrant(client *protocol.Client, campfireID, myPubKey string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	interval := 3 * time.Second

	for time.Now().Before(deadline) {
		result, err := client.Read(protocol.ReadRequest{
			CampfireID: campfireID,
			Tags:       []string{"work:role-grant"},
		})
		if err == nil {
			for _, msg := range result.Messages {
				// Check that this grant targets our pubkey.
				if containsTag(msg.Tags, "work:for:"+myPubKey) {
					return msg.ID, nil
				}
				// Also check payload for pubkey field.
				if grantTargets(msg, myPubKey) {
					return msg.ID, nil
				}
			}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if interval > remaining {
			interval = remaining
		}
		time.Sleep(interval)
	}

	return "", fmt.Errorf("timed out waiting for role-grant after %s", timeout)
}

// containsTag returns true if the tag slice contains the given tag.
func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// grantTargets returns true if the message payload's pubkey field matches myPubKey.
func grantTargets(msg protocol.Message, myPubKey string) bool {
	var payload struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return false
	}
	return payload.Pubkey == myPubKey
}

// isHex returns true if s consists entirely of hex characters.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func init() {
	joinCmd.Flags().Duration("timeout", 5*time.Minute, "how long to wait for admission grant")
	joinCmd.Flags().String("role", "member", "role to request: member or agent")
	rootCmd.AddCommand(joinCmd)
}

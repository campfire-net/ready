package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

// itemIDRe matches project-prefixed work item IDs (e.g. "ready-a1b", "myproject-xyz123").
var itemIDRe = regexp.MustCompile(`^[a-z][a-z0-9]*-[a-z0-9]{3,}$`)

var admitCmd = &cobra.Command{
	Use:   "admit <public-key-hex | item-id>",
	Short: "Admit an identity to a project campfire",
	Long: `Admit an Ed25519 public key to a project campfire.

Accepts either:
  - A 64-character hex public key (direct admission)
  - A work item ID (ready-xxx) referencing a work:join-request — extracts
    the requester's pubkey from the join-request item, posts a work:role-grant,
    and calls Client.Admit() to write the membership record.

By default, admits to the main project campfire with the "member" role.

Use --role org-observer to admit to the shadow summary campfire instead.
Org observers receive work:item-summary projections (title, status, priority,
assignee, eta) but cannot read the main campfire content.

Use --deny "reason" to reject a join-request item without granting access.

ROLES
  member        Full member of the main campfire (default)
  org-observer  Read-only access to the summary campfire only
  agent         Agent role (reduced permissions)

EXAMPLES
  rd admit abcdef...                         # admit by pubkey
  rd admit abcdef...  --role org-observer
  rd admit ready-abc                         # admit from join-request item
  rd admit ready-abc  --role agent
  rd admit ready-abc  --deny "not approved"  # reject join-request`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := args[0]
		role, _ := cmd.Flags().GetString("role")
		denyReason, _ := cmd.Flags().GetString("deny")

		if role == "" {
			role = "member"
		}

		// Detect if arg is an item-id or a raw pubkey.
		if itemIDRe.MatchString(arg) {
			return admitFromJoinRequest(arg, role, denyReason)
		}

		// Direct pubkey admit (Wave 2 behaviour).
		if denyReason != "" {
			return fmt.Errorf("--deny requires an item-id argument (not a pubkey)")
		}
		return admitByPubKey(arg, role)
	},
}

// admitByPubKey admits a raw hex public key — the original Wave 2 flow.
func admitByPubKey(pubKeyHex, role string) error {
	// Load project config.
	projectDir, ok := readyProjectDir()
	if !ok {
		return fmt.Errorf("no ready project found in current directory or parents")
	}

	syncCfg, err := rdconfig.LoadSyncConfig(projectDir)
	if err != nil {
		return fmt.Errorf("loading sync config: %w", err)
	}

	client, err := requireClient()
	if err != nil {
		return err
	}

	switch role {
	case "member":
		if syncCfg.CampfireID == "" {
			return fmt.Errorf("no campfire configured for this project (offline mode?)")
		}
		return admitMember(client, syncCfg.CampfireID, pubKeyHex, "main campfire")

	case "org-observer":
		if syncCfg.SummaryCampfireID == "" {
			return fmt.Errorf("no summary campfire configured for this project — run 'rd init' to create one")
		}
		if err := admitMember(client, syncCfg.SummaryCampfireID, pubKeyHex, "summary campfire"); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "  org-observers receive work:item-summary projections only")
		fmt.Fprintln(os.Stdout, "  main campfire content is not accessible to this identity")
		return nil

	case "agent":
		if syncCfg.CampfireID == "" {
			return fmt.Errorf("no campfire configured for this project (offline mode?)")
		}
		return admitMemberWithRole(client, syncCfg.CampfireID, pubKeyHex, "agent", "main campfire")

	default:
		return fmt.Errorf("unknown role %q — valid roles: member, org-observer, agent", role)
	}
}

// admitFromJoinRequest admits a member from a work:join-request item.
// It looks up the item, extracts the requester's pubkey, posts a role-grant,
// and calls Client.Admit() to write the membership record.
// If denyReason is non-empty, the request is denied instead.
func admitFromJoinRequest(itemID, role, denyReason string) error {
	campfireID, _, ok := projectRoot()
	if !ok {
		return fmt.Errorf("no campfire project found — run 'rd init' first")
	}

	agentID, s, err := requireAgentAndStore()
	if err != nil {
		return err
	}
	defer s.Close()

	client, err := requireClient()
	if err != nil {
		return err
	}

	// Look up the join-request item.
	item, err := byIDFromJSONLOrStore(s, itemID)
	if err != nil {
		return fmt.Errorf("looking up item %s: %w", itemID, err)
	}

	// Extract the requester's pubkey from the item.
	// The pubkey is in item.For (set by convention:operation args) or
	// embedded in the item's context/description payload.
	pubKeyHex := item.For
	if pubKeyHex == "" {
		// Fall back: try to extract from item context/description as JSON.
		pubKeyHex = extractPubkeyFromContext(item.Context)
	}
	if pubKeyHex == "" {
		pubKeyHex = extractPubkeyFromContext(item.Description)
	}
	if pubKeyHex == "" {
		return fmt.Errorf("could not find pubkey in join-request item %s — check item.For field", itemID)
	}

	// Validate pubkey format.
	if len(pubKeyHex) != 64 || !isHex(pubKeyHex) {
		return fmt.Errorf("pubkey in item %s is not a valid 64-char hex key: %q", itemID, pubKeyHex)
	}

	exec, _, execErr := requireExecutor()
	if execErr != nil {
		return fmt.Errorf("initializing executor: %w", execErr)
	}

	ctx := context.Background()

	if denyReason != "" {
		// Deny: post work:close with reason=cancelled on the join-request item.
		closeDecl, declErr := loadDeclaration("close")
		if declErr != nil {
			return fmt.Errorf("loading close declaration: %w", declErr)
		}
		_, closeErr := exec.Execute(ctx, closeDecl, campfireID, map[string]any{
			"target":     item.MsgID,
			"resolution": "cancelled",
			"reason":     denyReason,
		})
		if closeErr != nil {
			return fmt.Errorf("posting denial: %w", closeErr)
		}
		displayKey := pubKeyHex
		if len(displayKey) > 12 {
			displayKey = displayKey[:12] + "..."
		}
		fmt.Fprintf(os.Stdout, "denied join request from %s: %s\n", displayKey, denyReason)
		return nil
	}

	// Grant: post work:role-grant for the requester.
	roleGrantDecl, declErr := loadDeclaration("role-grant")
	if declErr != nil {
		return fmt.Errorf("loading role-grant declaration: %w", declErr)
	}

	grantRole := role
	if grantRole == "org-observer" {
		grantRole = "member" // org-observer is an rd concept, not a campfire role
	}

	now := time.Now().UTC().Format(time.RFC3339)
	grantResult, grantErr := exec.Execute(ctx, roleGrantDecl, campfireID, map[string]any{
		"pubkey":     pubKeyHex,
		"role":       grantRole,
		"granted_at": now,
	})
	if grantErr != nil {
		return fmt.Errorf("posting role-grant: %w", grantErr)
	}

	// Call Client.Admit() to write the membership record.
	m, membershipErr := client.GetMembership(campfireID)
	if membershipErr != nil {
		return fmt.Errorf("getting campfire membership: %w", membershipErr)
	}

	admitRole := grantRole
	if role == "agent" {
		admitRole = "agent"
	}

	admitErr := client.Admit(protocol.AdmitRequest{
		CampfireID:      campfireID,
		MemberPubKeyHex: pubKeyHex,
		Role:            admitRole,
		Transport:       protocol.FilesystemTransport{Dir: m.TransportDir},
	})
	if admitErr != nil {
		// role-grant was posted (advisory), Admit is the enforcement gate.
		return fmt.Errorf("Admit() failed (role-grant already posted): %w", admitErr)
	}

	// Close the join-request item.
	closeDecl, declErr := loadDeclaration("close")
	if declErr != nil {
		// Non-fatal: warn and continue.
		fmt.Fprintf(os.Stderr, "warning: could not load close declaration: %v\n", declErr)
	} else {
		_, closeErr := exec.Execute(ctx, closeDecl, campfireID, map[string]any{
			"target":     item.MsgID,
			"resolution": "done",
			"reason":     fmt.Sprintf("admitted as %s (role-grant: %s)", grantRole, grantResult.MessageID[:12]+"..."),
		})
		if closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not close join-request item: %v\n", closeErr)
		}
	}

	displayKey := pubKeyHex
	if len(displayKey) > 12 {
		displayKey = displayKey[:12] + "..."
	}
	fmt.Fprintf(os.Stdout, "admitted %s as %s (role-grant: %s)\n", displayKey, role, grantResult.MessageID[:12]+"...")

	_ = agentID // used via requireAgentAndStore, may be needed for future audit
	return nil
}

// extractPubkeyFromContext tries to extract a "pubkey" field from a JSON context string.
func extractPubkeyFromContext(ctx string) string {
	if ctx == "" {
		return ""
	}
	var payload struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.Unmarshal([]byte(ctx), &payload); err != nil {
		return ""
	}
	return payload.Pubkey
}

// admitMember admits the given public key to the campfire identified by campfireID.
// It looks up the transport dir from the client's membership store.
func admitMember(client *protocol.Client, campfireID, pubKeyHex, label string) error {
	return admitMemberWithRole(client, campfireID, pubKeyHex, "", label)
}

// admitMemberWithRole admits the given public key with the specified role.
func admitMemberWithRole(client *protocol.Client, campfireID, pubKeyHex, role, label string) error {
	m, err := client.GetMembership(campfireID)
	if err != nil {
		return fmt.Errorf("getting %s membership: %w — are you a member of this campfire?", label, err)
	}

	if err := client.Admit(protocol.AdmitRequest{
		CampfireID:      campfireID,
		MemberPubKeyHex: pubKeyHex,
		Role:            role,
		Transport:       protocol.FilesystemTransport{Dir: m.TransportDir},
	}); err != nil {
		return fmt.Errorf("admitting to %s: %w", label, err)
	}

	displayKey := pubKeyHex
	if len(displayKey) > 12 {
		displayKey = displayKey[:12] + "..."
	}
	displayCampfire := campfireID
	if len(displayCampfire) > 12 {
		displayCampfire = displayCampfire[:12] + "..."
	}
	fmt.Fprintf(os.Stdout, "admitted %s to %s (%s)\n", displayKey, label, displayCampfire)
	return nil
}

func init() {
	admitCmd.Flags().String("role", "member", "role to grant: member, org-observer, or agent")
	admitCmd.Flags().String("deny", "", "deny the join request with this reason (requires item-id arg)")
	rootCmd.AddCommand(admitCmd)
}

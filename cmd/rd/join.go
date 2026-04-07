package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

// defaultBeaconRoot is the compiled-in default beacon root ID.
// Empty string means no default is compiled in — any first use is a TOFU event.
const defaultBeaconRoot = ""

var joinCmd = &cobra.Command{
	Use:   "join <name-or-campfire-id>",
	Short: "Join a project campfire by name or ID",
	Long: `Join a campfire by name (cf:// URI or short name) or by campfire ID.

For open campfires, joins immediately.

For invite-only campfires, posts a work:join-request and polls for a
work:role-grant (admission grant) targeting your public key.

TOFU PINNING
  The first time you join using a non-default beacon root (--root), rd warns
  you and pins the beacon root in the config. On subsequent joins, any
  deviation from the pinned root requires --confirm.

  To reset the pinned root:
    rd join --reset-beacon-root

EXAMPLES
  rd join myorg.ready/myproject
  rd join cf://myorg.ready/myproject
  rd join abcdef1234...               # join by campfire ID directly
  rd join <id> --root <beacon-root>   # join with explicit beacon root (TOFU)
  rd join --reset-beacon-root         # clear the pinned beacon root`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resetRoot, _ := cmd.Flags().GetBool("reset-beacon-root")
		beaconRootFlag, _ := cmd.Flags().GetString("root")
		confirm, _ := cmd.Flags().GetBool("confirm")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		role, _ := cmd.Flags().GetString("role")
		if role == "" {
			role = "member"
		}

		cfg, err := rdconfig.Load(CFHome())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Handle --reset-beacon-root.
		if resetRoot {
			prev, saveErr := resetBeaconRoot(CFHome())
			if saveErr != nil {
				return saveErr
			}
			if prev == "" {
				fmt.Println("no beacon root pinned")
				return nil
			}
			fmt.Printf("beacon root pin cleared (was: %s...)\n", prev[:minInt(12, len(prev))])
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("name-or-campfire-id required (use --reset-beacon-root to clear the pinned beacon root)")
		}

		nameOrID := args[0]

		// TOFU pinning: check beacon root before resolving.
		if err := applyBeaconRootTOFU(CFHome(), cfg, beaconRootFlag, confirm); err != nil {
			return err
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
			Transport:  protocol.FilesystemTransport{Dir: localCampfireBaseDir()},
		})
		if err == nil {
			displayID := campfireID
			if len(displayID) > 12 {
				displayID = displayID[:12] + "..."
			}
			fmt.Fprintf(os.Stdout, "joined campfire %s (%s)\n", displayID, result.JoinProtocol)
			fmt.Println("  cross-campfire deps referencing this campfire will now resolve")
			return nil
		}

		// Join failed. If this is an invite-only campfire, post a join-request.
		fmt.Fprintf(os.Stderr, "rd: campfire is invite-only — posting join request\n")

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
			Transport:  protocol.FilesystemTransport{Dir: localCampfireBaseDir()},
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

// applyBeaconRootTOFU applies the TOFU beacon root pinning logic.
// cfg is read and updated in-place; if a pin is saved, cfHome is used for rdconfig.Save.
// Returns an error if the user aborts or the root mismatches without --confirm.
//
// Paths:
//   - beaconRoot empty: no-op
//   - cfg.BeaconRoot empty (first use): warn, prompt if interactive and !confirm, pin
//   - cfg.BeaconRoot matches beaconRoot: no-op
//   - cfg.BeaconRoot mismatches beaconRoot + !confirm: error
//   - cfg.BeaconRoot mismatches beaconRoot + confirm: proceed (no error)
func applyBeaconRootTOFU(cfHome string, cfg *rdconfig.Config, beaconRoot string, confirm bool) error {
	if beaconRoot == "" {
		return nil
	}

	if cfg.BeaconRoot == "" {
		// First use of a non-default beacon root — warn and pin (TOFU).
		fmt.Fprintf(os.Stderr, "warning: first use of non-default beacon root %s...\n", beaconRoot[:minInt(12, len(beaconRoot))])
		fmt.Fprintf(os.Stderr, "  this root will be pinned (TOFU) in the config\n")
		fmt.Fprintf(os.Stderr, "  future joins using a different root will require --confirm\n")

		if !confirm {
			if !isInteractive() {
				fmt.Fprintf(os.Stderr, "  non-interactive: pinning beacon root automatically\n")
			} else {
				fmt.Fprint(os.Stderr, "pin this beacon root? [Y/n] ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
					if answer == "n" || answer == "no" {
						return fmt.Errorf("aborted: beacon root not pinned")
					}
				}
			}
		}
		cfg.BeaconRoot = beaconRoot
		if err := rdconfig.Save(cfHome, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not pin beacon root: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "  pinned beacon root: %s...\n", beaconRoot[:minInt(12, len(beaconRoot))])
		}
		return nil
	}

	if cfg.BeaconRoot != beaconRoot {
		// Deviation from pinned root — warn, require confirmation.
		fmt.Fprintf(os.Stderr, "warning: beacon root mismatch\n")
		fmt.Fprintf(os.Stderr, "  pinned:    %s...\n", cfg.BeaconRoot[:minInt(12, len(cfg.BeaconRoot))])
		fmt.Fprintf(os.Stderr, "  requested: %s...\n", beaconRoot[:minInt(12, len(beaconRoot))])
		if !confirm {
			return fmt.Errorf("beacon root does not match pinned root — pass --confirm to proceed or use 'rd join --reset-beacon-root' to re-pin")
		}
	}
	return nil
}

// resetBeaconRoot clears the pinned beacon root from the config at cfHome.
// Returns the previous value (empty string if nothing was pinned).
func resetBeaconRoot(cfHome string) (prev string, err error) {
	cfg, err := rdconfig.Load(cfHome)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.BeaconRoot == "" {
		return "", nil
	}
	prev = cfg.BeaconRoot
	cfg.BeaconRoot = ""
	if err := rdconfig.Save(cfHome, cfg); err != nil {
		return "", fmt.Errorf("saving config: %w", err)
	}
	return prev, nil
}

// resolveName resolves a name (cf:// URI, short name, or raw campfire ID) to a
// campfire ID hex string.
func resolveName(client *protocol.Client, input string) (string, error) {
	// If it's already a campfire ID (64 hex chars), return as-is.
	if len(input) == 64 && isHex(input) {
		return input, nil
	}

	// Use the naming resolver with the client.
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
	// Validate that resolution produced a 64-char hex campfire ID.
	if len(resolved) != 64 || !isHex(resolved) {
		return "", fmt.Errorf("name %q resolved to %q which is not a valid campfire ID (64 hex chars)", input, resolved)
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
				if containsTag(msg.Tags, "work:for:"+myPubKey) {
					return msg.ID, nil
				}
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

// grantTargets returns true if the message payload's pubkey field matches myPubKey
// AND the role is an admission role (not a revocation).
func grantTargets(msg protocol.Message, myPubKey string) bool {
	var payload struct {
		Pubkey string `json:"pubkey"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return false
	}
	if payload.Pubkey != myPubKey {
		return false
	}
	// Reject revocation grants — we're waiting for an admission, not a ban.
	return payload.Role != "revoked" && payload.Role != ""
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

// isInteractive reports whether stdin is a terminal.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func init() {
	joinCmd.Flags().Duration("timeout", 5*time.Minute, "how long to wait for admission grant")
	joinCmd.Flags().String("role", "member", "role to request: member or agent")
	joinCmd.Flags().String("root", "", "beacon root campfire ID to use for TOFU pinning")
	joinCmd.Flags().Bool("reset-beacon-root", false, "clear the pinned beacon root")
	joinCmd.Flags().Bool("confirm", false, "confirm beacon root deviation without prompting")
	rootCmd.AddCommand(joinCmd)
}

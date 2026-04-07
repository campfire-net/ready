package main

import (
	"fmt"
	"os"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

var admitCmd = &cobra.Command{
	Use:   "admit <public-key-hex>",
	Short: "Admit an identity to a project campfire",
	Long: `Admit an Ed25519 public key to a project campfire.

By default, admits to the main project campfire with the "member" role.

Use --role org-observer to admit to the shadow summary campfire instead.
Org observers receive work:item-summary projections (title, status, priority,
assignee, eta) but cannot read the main campfire content.

ROLES
  member        Full member of the main campfire (default)
  org-observer  Read-only access to the summary campfire only

EXAMPLES
  rd admit abcdef...          --role member
  rd admit abcdef...          --role org-observer`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pubKeyHex := args[0]
		role, _ := cmd.Flags().GetString("role")

		if role == "" {
			role = "member"
		}

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

		default:
			return fmt.Errorf("unknown role %q — valid roles: member, org-observer", role)
		}
	},
}

// admitMember admits the given public key to the campfire identified by campfireID.
// It looks up the transport dir from the client's membership store.
func admitMember(client *protocol.Client, campfireID, pubKeyHex, label string) error {
	m, err := client.GetMembership(campfireID)
	if err != nil {
		return fmt.Errorf("getting %s membership: %w — are you a member of this campfire?", label, err)
	}

	if err := client.Admit(protocol.AdmitRequest{
		CampfireID:      campfireID,
		MemberPubKeyHex: pubKeyHex,
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
	admitCmd.Flags().String("role", "member", "role to grant: member or org-observer")
	rootCmd.AddCommand(admitCmd)
}

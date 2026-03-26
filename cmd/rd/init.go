package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/campfire-net/campfire/pkg/beacon"
	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"
	"github.com/spf13/cobra"

	"github.com/3dl-dev/ready/pkg/declarations"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a ready work campfire for this project",
	Long: `Create a new work campfire and link it to the current directory.

This command:
  1. Creates a campfire with reception_requirements: ["work:create"]
  2. Writes .campfire/root (linking this directory to the campfire)
  3. Posts all convention:operation declarations (making the campfire self-describing)
  4. Publishes a beacon for local discovery
  5. Checks for a home campfire and reports what it finds

The project campfire works standalone — no home campfire or naming required.
Use 'rd register' later to add naming when you're ready.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")

		// Check we're not already initialized.
		if _, _, ok := projectRoot(); ok {
			return fmt.Errorf(".campfire/root already exists — this project is already initialized")
		}

		// Load identity.
		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Default name from current directory.
		if name == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting cwd: %w", err)
			}
			name = filepath.Base(cwd)
		}

		// Default description.
		if description == "" {
			description = name + " work campfire"
		}

		// --- Create the campfire ---

		campfireID, err := createLocalCampfire(agentID, s, "open", []string{"work:create"}, description)
		if err != nil {
			return err
		}

		// --- Write .campfire/root ---

		cwd, _ := os.Getwd()
		campfireDir := filepath.Join(cwd, ".campfire")
		if err := os.MkdirAll(campfireDir, 0755); err != nil {
			return fmt.Errorf("creating .campfire dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(campfireDir, "root"), []byte(campfireID), 0644); err != nil {
			return fmt.Errorf("writing .campfire/root: %w", err)
		}

		// Also publish beacon to .campfire/beacons/ for project-local discovery.
		m, _ := s.GetMembership(campfireID)
		if m != nil {
			tr := fs.New(fs.DefaultBaseDir())
			cfState, err := tr.ReadState(campfireID)
			if err == nil {
				b, err := beacon.New(
					cfState.PublicKey, cfState.PrivateKey,
					cfState.JoinProtocol, cfState.ReceptionRequirements,
					beacon.TransportConfig{
						Protocol: "filesystem",
						Config:   map[string]string{"dir": tr.CampfireDir(campfireID)},
					},
					description,
				)
				if err == nil {
					projectBeaconsDir := filepath.Join(campfireDir, "beacons")
					_ = beacon.Publish(projectBeaconsDir, b)
				}
			}
		}

		// --- Post convention:operation declarations ---

		nDecls, err := declarations.PostAll(agentID, s, campfireID)
		if err != nil {
			return fmt.Errorf("posting declarations (%d posted before failure): %w", nDecls, err)
		}

		// --- Check for home campfire ---

		aliases := naming.NewAliasStore(CFHome())
		homeID, homeErr := aliases.Get("home")
		hasHome := homeErr == nil && homeID != ""

		// --- Output ---

		if jsonOutput {
			out := map[string]interface{}{
				"campfire_id":  campfireID,
				"name":         name,
				"declarations": nDecls,
				"description":  description,
				"has_home":     hasHome,
			}
			if hasHome {
				out["home_campfire_id"] = homeID
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("initialized %s\n", name)
		fmt.Printf("  campfire: %s\n", campfireID[:12]+"...")
		fmt.Printf("  declarations: %d operations published\n", nDecls)
		fmt.Println()
		if hasHome {
			fmt.Printf("  home campfire: found (%s)\n", homeID[:12]+"...")
			fmt.Println("  run 'rd register --org <name>' to add naming")
		} else {
			fmt.Println("  home campfire: not found")
			fmt.Println("  your project works standalone. to add naming later:")
			fmt.Println("    rd register --org <name>        create a home and register")
			fmt.Println("    rd register --home <id>         join an existing home")
		}

		return nil
	},
}

// createLocalCampfire creates a campfire with the filesystem transport, publishes
// its beacon, and records membership. Returns the campfire ID hex.
func createLocalCampfire(agentID *identity.Identity, s store.Store, joinProtocol string, receptionReqs []string, description string) (string, error) {
	cf, err := campfirepkg.New(joinProtocol, receptionReqs, 1)
	if err != nil {
		return "", fmt.Errorf("creating campfire: %w", err)
	}
	cf.AddMember(agentID.PublicKey)
	campfireID := cf.PublicKeyHex()

	baseDir := fs.DefaultBaseDir()
	tr := fs.New(baseDir)
	if err := tr.Init(cf); err != nil {
		return "", fmt.Errorf("initializing transport: %w", err)
	}
	if err := tr.WriteMember(campfireID, campfirepkg.MemberRecord{
		PublicKey: agentID.PublicKey,
		JoinedAt:  time.Now().UnixNano(),
	}); err != nil {
		return "", fmt.Errorf("writing member record: %w", err)
	}

	b, err := beacon.New(
		cf.PublicKey, cf.PrivateKey,
		cf.JoinProtocol, cf.ReceptionRequirements,
		beacon.TransportConfig{
			Protocol: "filesystem",
			Config:   map[string]string{"dir": tr.CampfireDir(campfireID)},
		},
		description,
	)
	if err != nil {
		return "", fmt.Errorf("creating beacon: %w", err)
	}
	if err := beacon.Publish(beacon.DefaultBeaconDir(), b); err != nil {
		return "", fmt.Errorf("publishing beacon: %w", err)
	}

	if err := s.AddMembership(store.Membership{
		CampfireID:   campfireID,
		TransportDir: tr.CampfireDir(campfireID),
		JoinProtocol: cf.JoinProtocol,
		Role:         store.PeerRoleCreator,
		JoinedAt:     store.NowNano(),
		Threshold:    cf.Threshold,
		Description:  description,
	}); err != nil {
		return "", fmt.Errorf("recording membership: %w", err)
	}

	return campfireID, nil
}

func init() {
	initCmd.Flags().String("name", "", "project name (default: current directory name)")
	initCmd.Flags().String("description", "", "campfire description (default: '<name> work campfire')")
	rootCmd.AddCommand(initCmd)
}

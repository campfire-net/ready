package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/campfire-net/campfire/pkg/beacon"
	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"
	"github.com/spf13/cobra"

	"github.com/3dl-dev/ready/pkg/declarations"
	"github.com/3dl-dev/ready/pkg/rdconfig"
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

If --org is provided (or configured), also registers the campfire in the naming
tree at cf://<org>.ready.<name>. The org root (cf://<org>) may or may not exist;
registration under it is best-effort.

Use 'rd register' later to add naming when the org becomes available.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		org, _ := cmd.Flags().GetString("org")
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

		// Load config for org default.
		cfg, err := rdconfig.Load(CFHome())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if org == "" {
			org = cfg.Org
		}

		// --- Create the campfire ---

		cf, err := campfirepkg.New("open", []string{"work:create"}, 1)
		if err != nil {
			return fmt.Errorf("creating campfire: %w", err)
		}
		cf.AddMember(agentID.PublicKey)
		campfireID := cf.PublicKeyHex()

		// Set up filesystem transport.
		baseDir := fs.DefaultBaseDir()
		transport := fs.New(baseDir)
		if err := transport.Init(cf); err != nil {
			return fmt.Errorf("initializing transport: %w", err)
		}
		if err := transport.WriteMember(campfireID, campfirepkg.MemberRecord{
			PublicKey: agentID.PublicKey,
			JoinedAt:  time.Now().UnixNano(),
		}); err != nil {
			return fmt.Errorf("writing member record: %w", err)
		}

		// Create and publish beacon.
		b, err := beacon.New(
			cf.PublicKey, cf.PrivateKey,
			cf.JoinProtocol, cf.ReceptionRequirements,
			beacon.TransportConfig{
				Protocol: "filesystem",
				Config:   map[string]string{"dir": transport.CampfireDir(campfireID)},
			},
			description,
		)
		if err != nil {
			return fmt.Errorf("creating beacon: %w", err)
		}
		if err := beacon.Publish(beacon.DefaultBeaconDir(), b); err != nil {
			return fmt.Errorf("publishing beacon: %w", err)
		}

		// Record membership in local store.
		if err := s.AddMembership(store.Membership{
			CampfireID:   campfireID,
			TransportDir: transport.CampfireDir(campfireID),
			JoinProtocol: cf.JoinProtocol,
			Role:         store.PeerRoleCreator,
			JoinedAt:     store.NowNano(),
			Threshold:    cf.Threshold,
			Description:  description,
		}); err != nil {
			return fmt.Errorf("recording membership: %w", err)
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
		projectBeaconsDir := filepath.Join(campfireDir, "beacons")
		if err := beacon.Publish(projectBeaconsDir, b); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not publish project beacon: %v\n", err)
		}

		// --- Post convention:operation declarations ---

		nDecls, err := declarations.PostAll(agentID, s, campfireID)
		if err != nil {
			return fmt.Errorf("posting declarations (%d posted before failure): %w", nDecls, err)
		}

		// --- Save org if provided ---

		if org != "" && cfg.Org == "" {
			cfg.Org = org
			if err := rdconfig.Save(CFHome(), cfg); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
			}
		}

		// --- Output ---

		if jsonOutput {
			out := map[string]interface{}{
				"campfire_id":  campfireID,
				"name":         name,
				"declarations": nDecls,
				"description":  description,
			}
			if org != "" {
				out["namespace"] = fmt.Sprintf("cf://%s.ready.%s", org, name)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("initialized %s\n", name)
		fmt.Printf("  campfire: %s\n", campfireID[:12]+"...")
		fmt.Printf("  declarations: %d operations published\n", nDecls)
		if org != "" {
			fmt.Printf("  namespace: cf://%s.ready.%s\n", org, name)
			fmt.Println("  (run 'rd register' to publish naming when ready)")
		} else {
			fmt.Println("  (run 'rd register --org <name>' to add naming later)")
		}

		return nil
	},
}

func init() {
	initCmd.Flags().String("name", "", "project name (default: current directory name)")
	initCmd.Flags().String("org", "", "organization name for cf://<org>.ready.<name> naming")
	initCmd.Flags().String("description", "", "campfire description (default: '<name> work campfire')")
	rootCmd.AddCommand(initCmd)
}

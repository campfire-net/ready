package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/campfire-net/campfire/pkg/beacon"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"
	"github.com/spf13/cobra"

	"github.com/3dl-dev/ready/pkg/rdconfig"
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register this project in the naming tree",
	Long: `Register this project's work campfire under a home campfire.

Three modes:

  rd register --org baron              auto-create home if needed, register under it
  rd register --home <campfire-id>     join an existing home, register under it
  rd register --org baron --home <id>  use specific home, set org alias

This command:
  1. Finds or creates a home campfire (the operator root)
  2. Finds or creates a ready namespace campfire under the home
  3. Registers this project as cf://<org>.ready.<name>
  4. Sets local aliases for resolution

The project campfire works independently of naming — names are optional
discoverability. Run this whenever you're ready to add naming.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		org, _ := cmd.Flags().GetString("org")
		homeFlag, _ := cmd.Flags().GetString("home")

		// Must be initialized.
		campfireID, projectDir, ok := projectRoot()
		if !ok {
			return fmt.Errorf("no .campfire/root found — run 'rd init' first")
		}

		agentID, s, err := requireAgentAndStore()
		if err != nil {
			return err
		}
		defer s.Close()

		// Default name from project directory.
		if name == "" {
			name = filepath.Base(projectDir)
		}

		// Load config for org default.
		cfg, err := rdconfig.Load(CFHome())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if org == "" {
			org = cfg.Org
		}
		if org == "" {
			return fmt.Errorf("--org is required (or set via prior 'rd register --org <name>')")
		}

		aliases := naming.NewAliasStore(CFHome())

		// --- Step 1: Find or create home campfire ---

		homeID := homeFlag
		if homeID == "" {
			// Check alias first.
			if id, err := aliases.Get("home"); err == nil && id != "" {
				homeID = id
			}
		}
		if homeID == "" {
			// Check config.
			homeID = cfg.HomeCampfireID
		}

		createdHome := false
		if homeID == "" {
			// Auto-create home campfire (operator root).
			homeDesc := org + " operator root"
			homeID, err = createLocalCampfire(agentID, s, "invite-only", []string{"beacon:registration"}, homeDesc)
			if err != nil {
				return fmt.Errorf("creating home campfire: %w", err)
			}
			createdHome = true
			if !jsonOutput {
				fmt.Printf("created home campfire: %s\n", homeID[:12]+"...")
			}
		}

		// Set aliases for home.
		_ = aliases.Set("home", homeID)
		_ = aliases.Set(org, homeID)

		// --- Step 2: Find or create ready namespace ---

		readyID := cfg.ReadyCampfireID
		if readyID == "" {
			// Check alias.
			if id, err := aliases.Get(org + ".ready"); err == nil && id != "" {
				readyID = id
			}
		}

		createdReady := false
		if readyID == "" {
			// Create ready namespace campfire.
			readyDesc := org + " ready namespace"
			readyID, err = createLocalCampfire(agentID, s, "open", []string{"beacon:registration"}, readyDesc)
			if err != nil {
				return fmt.Errorf("creating ready namespace: %w", err)
			}
			createdReady = true
			if !jsonOutput {
				fmt.Printf("created ready namespace: %s\n", readyID[:12]+"...")
			}

			// Register ready under home.
			if err := postBeaconRegistration(agentID, s, homeID, readyID, "ready", readyDesc); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not register ready under home: %v\n", err)
			}
		}

		// Set alias for ready namespace.
		_ = aliases.Set(org+".ready", readyID)

		// --- Step 3: Register project under ready ---

		projectDesc := name + " work campfire"
		if m, err := s.GetMembership(campfireID); err == nil && m != nil && m.Description != "" {
			projectDesc = m.Description
		}

		if err := postBeaconRegistration(agentID, s, readyID, campfireID, name, projectDesc); err != nil {
			return fmt.Errorf("registering project: %w", err)
		}

		// --- Step 4: Save config + aliases ---

		cfg.Org = org
		cfg.HomeCampfireID = homeID
		cfg.ReadyCampfireID = readyID
		if err := rdconfig.Save(CFHome(), cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
		}

		// --- Output ---

		namespace := fmt.Sprintf("cf://%s.ready.%s", org, name)
		localURI := fmt.Sprintf("cf://~%s.ready/%s", org, name)
		if jsonOutput {
			out := map[string]interface{}{
				"campfire_id":       campfireID,
				"home_campfire_id":  homeID,
				"ready_campfire_id": readyID,
				"name":              name,
				"org":               org,
				"namespace":         namespace,
				"local_uri":         localURI,
				"created_home":      createdHome,
				"created_ready":     createdReady,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("registered %s\n", namespace)
		fmt.Printf("  project:   %s\n", campfireID[:12]+"...")
		fmt.Printf("  home:      %s\n", homeID[:12]+"...")
		fmt.Printf("  namespace: %s\n", readyID[:12]+"...")
		fmt.Printf("  local:     %s\n", localURI)
		fmt.Println()
		fmt.Println("  to make globally resolvable later:")
		fmt.Printf("    cf register <root-id> %s %s\n", org, homeID)

		return nil
	},
}

// postBeaconRegistration sends a beacon-registration with naming:name:<segment>
// to the parent campfire, registering the child campfire under the given name.
func postBeaconRegistration(agentID *identity.Identity, s store.Store, parentID, childID, name, description string) error {
	// Read the child campfire's state to build the inner beacon.
	tr := fs.New(fs.DefaultBaseDir())
	cfState, err := tr.ReadState(childID)
	if err != nil {
		return fmt.Errorf("reading campfire state for %s: %w", childID[:12], err)
	}

	innerBeacon, err := beacon.New(
		cfState.PublicKey, cfState.PrivateKey,
		cfState.JoinProtocol, cfState.ReceptionRequirements,
		beacon.TransportConfig{
			Protocol: "filesystem",
			Config:   map[string]string{"dir": tr.CampfireDir(childID)},
		},
		description,
	)
	if err != nil {
		return fmt.Errorf("creating inner beacon: %w", err)
	}

	regPayload := map[string]interface{}{
		"campfire_id": childID,
		"name":        name,
		"description": description,
		"beacon": map[string]interface{}{
			"campfire_id":            childID,
			"description":            innerBeacon.Description,
			"join_protocol":          innerBeacon.JoinProtocol,
			"reception_requirements": innerBeacon.ReceptionRequirements,
			"tags": []string{
				"category:infrastructure",
				"topic:work-management",
				"member_count:1",
				"published_at:" + time.Now().UTC().Format(time.RFC3339),
				"naming:name:" + name,
			},
			"signature": hex.EncodeToString(innerBeacon.Signature),
		},
	}
	payloadBytes, err := json.Marshal(regPayload)
	if err != nil {
		return fmt.Errorf("encoding registration payload: %w", err)
	}

	tags := []string{"beacon:registration", "naming:name:" + name}
	parentMembership, err := s.GetMembership(parentID)
	if err != nil || parentMembership == nil {
		return fmt.Errorf("not a member of parent campfire %s", parentID[:12])
	}
	_, err = sendViaMembership(agentID, s, parentMembership, parentID, string(payloadBytes), tags, nil)
	return err
}

func init() {
	registerCmd.Flags().String("name", "", "project name (default: project directory name)")
	registerCmd.Flags().String("org", "", "organization name (default: from config)")
	registerCmd.Flags().String("home", "", "home campfire ID to register under (default: auto-detect or create)")
	rootCmd.AddCommand(registerCmd)
}

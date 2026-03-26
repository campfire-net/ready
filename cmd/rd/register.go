package main

import (
	"encoding/hex"
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

	"github.com/3dl-dev/ready/pkg/rdconfig"
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register this project in the naming tree",
	Long: `Register this project's work campfire in the cf://<org>.ready namespace.

This command:
  1. Finds or creates the cf://<org>.ready namespace campfire
  2. Registers this project as cf://<org>.ready.<name>
  3. Saves the org to config for future use

The org root campfire (cf://<org>) may or may not exist. Registration under
it is best-effort and can be retried later. The project campfire works
independently of naming — names are optional discoverability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		org, _ := cmd.Flags().GetString("org")

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

		// Load config for org.
		cfg, err := rdconfig.Load(CFHome())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if org == "" {
			org = cfg.Org
		}
		if org == "" {
			return fmt.Errorf("--org is required (or set via 'rd init --org <name>')")
		}

		// --- Ensure cf://<org>.ready campfire exists ---

		readyID := cfg.ReadyCampfireID
		if readyID == "" {
			// Create the ready namespace campfire.
			readyCF, err := campfirepkg.New("open", []string{"beacon:registration"}, 1)
			if err != nil {
				return fmt.Errorf("creating ready namespace campfire: %w", err)
			}
			readyCF.AddMember(agentID.PublicKey)
			readyID = readyCF.PublicKeyHex()

			baseDir := fs.DefaultBaseDir()
			tr := fs.New(baseDir)
			if err := tr.Init(readyCF); err != nil {
				return fmt.Errorf("initializing ready namespace transport: %w", err)
			}
			if err := tr.WriteMember(readyID, campfirepkg.MemberRecord{
				PublicKey: agentID.PublicKey,
				JoinedAt:  time.Now().UnixNano(),
			}); err != nil {
				return fmt.Errorf("writing ready namespace member: %w", err)
			}

			readyDesc := org + " ready namespace"
			b, err := beacon.New(
				readyCF.PublicKey, readyCF.PrivateKey,
				readyCF.JoinProtocol, readyCF.ReceptionRequirements,
				beacon.TransportConfig{
					Protocol: "filesystem",
					Config:   map[string]string{"dir": tr.CampfireDir(readyID)},
				},
				readyDesc,
			)
			if err != nil {
				return fmt.Errorf("creating ready namespace beacon: %w", err)
			}
			if err := beacon.Publish(beacon.DefaultBeaconDir(), b); err != nil {
				return fmt.Errorf("publishing ready namespace beacon: %w", err)
			}

			if err := s.AddMembership(store.Membership{
				CampfireID:   readyID,
				TransportDir: tr.CampfireDir(readyID),
				JoinProtocol: readyCF.JoinProtocol,
				Role:         store.PeerRoleCreator,
				JoinedAt:     store.NowNano(),
				Threshold:    readyCF.Threshold,
				Description:  readyDesc,
			}); err != nil {
				return fmt.Errorf("recording ready namespace membership: %w", err)
			}

			fmt.Printf("created ready namespace campfire: %s\n", readyID[:12]+"...")
		}

		// --- Register this project in the ready namespace ---

		// Read the project campfire's state to build the inner beacon.
		tr := fs.New(fs.DefaultBaseDir())
		cfState, err := tr.ReadState(campfireID)
		if err != nil {
			return fmt.Errorf("reading project campfire state: %w", err)
		}

		projectDesc := name + " work campfire"
		m, err := s.GetMembership(campfireID)
		if err == nil && m != nil && m.Description != "" {
			projectDesc = m.Description
		}

		// Build the inner beacon (signed by the project campfire's key).
		innerBeacon, err := beacon.New(
			cfState.PublicKey, cfState.PrivateKey,
			cfState.JoinProtocol, cfState.ReceptionRequirements,
			beacon.TransportConfig{
				Protocol: "filesystem",
				Config:   map[string]string{"dir": tr.CampfireDir(campfireID)},
			},
			projectDesc,
		)
		if err != nil {
			return fmt.Errorf("creating inner beacon: %w", err)
		}

		// Build beacon-registration payload per community-beacon §8.
		regPayload := map[string]interface{}{
			"campfire_id": campfireID,
			"name":        name,
			"description": projectDesc,
			"beacon": map[string]interface{}{
				"campfire_id":            campfireID,
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

		// Send to the ready namespace campfire.
		tags := []string{"beacon:registration", "naming:name:" + name}
		readyMembership, err := s.GetMembership(readyID)
		if err != nil || readyMembership == nil {
			return fmt.Errorf("not a member of ready namespace campfire %s", readyID[:12])
		}
		msg, err := sendViaMembership(agentID, s, readyMembership, readyID, string(payloadBytes), tags, nil)
		if err != nil {
			return fmt.Errorf("sending registration: %w", err)
		}

		// --- Save config ---

		cfg.Org = org
		cfg.ReadyCampfireID = readyID
		if err := rdconfig.Save(CFHome(), cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
		}

		// --- Output ---

		namespace := fmt.Sprintf("cf://%s.ready.%s", org, name)
		if jsonOutput {
			out := map[string]interface{}{
				"campfire_id":       campfireID,
				"ready_campfire_id": readyID,
				"name":              name,
				"namespace":         namespace,
				"msg_id":            msg.ID,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("registered %s\n", namespace)
		fmt.Printf("  project campfire: %s\n", campfireID[:12]+"...")
		fmt.Printf("  ready namespace:  %s\n", readyID[:12]+"...")

		return nil
	},
}

func init() {
	registerCmd.Flags().String("name", "", "project name (default: project directory name)")
	registerCmd.Flags().String("org", "", "organization name (default: from config)")
	rootCmd.AddCommand(registerCmd)
}

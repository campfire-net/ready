package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/campfire-net/campfire/pkg/beacon"
	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/declarations"
	"github.com/campfire-net/ready/pkg/durability"
	"github.com/campfire-net/ready/pkg/rdconfig"
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
  5. Evaluates campfire durability and stores sync config in .ready/config.json
  6. Checks for a home campfire and reports what it finds

The project campfire works standalone — no home campfire or naming required.
Use 'rd register' later to add naming when you're ready.

DURABILITY
  rd init evaluates durability tags before configuring sync. If the campfire
  does not meet the minimum requirements (max-ttl:0 + lifecycle:persistent +
  provenance:basic), a warning is printed and you are prompted to confirm.

  Configure via environment:
    RD_CAMPFIRE_TAGS   comma-separated beacon tags (e.g. "durability:max-ttl:0,durability:lifecycle:persistent")
    RD_PROVENANCE      operator provenance level ("getcampfire.dev", "operator-verified", "basic", "unverified")

  Pass --confirm to skip the interactive prompt and proceed regardless.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		confirm, _ := cmd.Flags().GetBool("confirm")

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

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}

		// Default name from current directory.
		if name == "" {
			name = filepath.Base(cwd)
		}

		// Default description.
		if description == "" {
			description = name + " work campfire"
		}

		// --- Create the campfire (state in ~/.campfire/campfires/<id>/) ---

		campfireID, err := createLocalCampfire(agentID, s, "invite-only", []string{"work:create"}, description)
		if err != nil {
			return err
		}

		// --- Write .campfire/root (pointer in the project) ---

		campfireDir := filepath.Join(cwd, ".campfire")
		if err := os.MkdirAll(campfireDir, 0755); err != nil {
			return fmt.Errorf("creating .campfire dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(campfireDir, "root"), []byte(campfireID), 0644); err != nil {
			return fmt.Errorf("writing .campfire/root: %w", err)
		}

		// Publish beacon to .campfire/beacons/ for project-local discovery.
		baseDir := localCampfireBaseDir()
		tr := fs.New(baseDir)
		cfState, err := tr.ReadState(campfireID)
		if err == nil {
			b, _ := beacon.New(
				cfState.PublicKey, cfState.PrivateKey,
				cfState.JoinProtocol, cfState.ReceptionRequirements,
				beacon.TransportConfig{
					Protocol: "filesystem",
					Config:   map[string]string{"dir": tr.CampfireDir(campfireID)},
				},
				description,
			)
			if b != nil {
				projectBeaconsDir := filepath.Join(campfireDir, "beacons")
				_ = beacon.Publish(projectBeaconsDir, b)
			}
		}

		// --- Post convention:operation declarations via transport ---

		payloads, err := declarations.All()
		if err != nil {
			return fmt.Errorf("loading declarations: %w", err)
		}
		membership, err := s.GetMembership(campfireID)
		if err != nil || membership == nil {
			return fmt.Errorf("cannot find membership for newly created campfire")
		}
		nDecls := 0
		for _, payload := range payloads {
			_, err := sendViaMembership(agentID, s, membership, campfireID, string(payload), []string{"convention:operation"}, nil)
			if err != nil {
				return fmt.Errorf("posting declaration %d: %w", nDecls, err)
			}
			nDecls++
		}

		// --- Evaluate durability and store sync config ---

		syncCfg, durabilityWarnings, err := evaluateCampfireDurability(campfireID, confirm)
		if err != nil {
			return err
		}
		if saveErr := rdconfig.SaveSyncConfig(cwd, syncCfg); saveErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save sync config: %v\n", saveErr)
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
				"durability":   syncCfg.Durability,
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
		if len(durabilityWarnings) > 0 {
			fmt.Println()
			fmt.Println("  sync config stored with warnings (see .ready/config.json)")
		}
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

// evaluateCampfireDurability reads campfire beacon tags (from RD_CAMPFIRE_TAGS
// environment variable), parses durability metadata, evaluates trust against
// the operator provenance level (from RD_PROVENANCE env), and warns if the
// campfire does not meet minimum sync requirements.
//
// If the campfire does not meet minimum requirements and confirm is false,
// the user is prompted interactively. Returns the SyncConfig to persist and
// any warnings that were displayed.
//
// This function is advisory — it never blocks init if the user confirms
// (or passes --confirm).
func evaluateCampfireDurability(campfireID string, confirm bool) (*rdconfig.SyncConfig, []string, error) {
	// Read campfire tags from environment.
	tags := campfireTagsFromEnv()

	// Read provenance level from environment.
	provenanceLevel := os.Getenv("RD_PROVENANCE")

	// Parse durability tags.
	result, err := durability.ParseTags(tags)
	if err != nil {
		result = &durability.DurabilityResult{Valid: false, Error: err.Error()}
	}

	// Evaluate trust.
	assessment := durability.EvaluateTrust(result, provenanceLevel)

	// Collect all warnings.
	var allWarnings []string
	allWarnings = append(allWarnings, result.Warnings...)
	allWarnings = append(allWarnings, assessment.Warnings...)

	// Build sync config.
	syncCfg := &rdconfig.SyncConfig{
		CampfireID: campfireID,
		Durability: &rdconfig.DurabilityAssessment{
			MeetsMinimum:    assessment.MeetsMinimum,
			Weight:          assessment.Weight,
			MaxTTL:          result.MaxTTL,
			LifecycleType:   result.LifecycleType,
			Warnings:        allWarnings,
			ProvenanceLevel: provenanceLevel,
		},
	}

	if assessment.MeetsMinimum {
		// All criteria met — proceed silently.
		return syncCfg, nil, nil
	}

	// Below minimum: print warnings.
	fmt.Fprintln(os.Stderr, "warning: campfire durability check — sync may be unreliable:")
	if result.MaxTTL != "" && result.MaxTTL != "0" {
		fmt.Fprintf(os.Stderr, "  max-ttl:%s — messages may expire after this period\n", result.MaxTTL)
	}
	if result.LifecycleType == "ephemeral" {
		fmt.Fprintln(os.Stderr, "  lifecycle:ephemeral — this campfire is short-lived")
	}
	for _, w := range assessment.Warnings {
		fmt.Fprintf(os.Stderr, "  %s\n", w)
	}
	fmt.Fprintln(os.Stderr, "  minimum requirements: max-ttl:0 + lifecycle:persistent + provenance:basic or higher")
	fmt.Fprintln(os.Stderr, "  set RD_CAMPFIRE_TAGS and RD_PROVENANCE to configure, or pass --confirm to proceed anyway")

	if confirm {
		return syncCfg, allWarnings, nil
	}

	// Prompt the user interactively.
	fmt.Fprint(os.Stderr, "proceed with sync configuration anyway? [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			return nil, nil, fmt.Errorf("aborted: campfire does not meet minimum durability requirements")
		}
	}

	return syncCfg, allWarnings, nil
}

// campfireTagsFromEnv reads campfire beacon tags from the RD_CAMPFIRE_TAGS
// environment variable. Tags are comma-separated.
// Example: RD_CAMPFIRE_TAGS="durability:max-ttl:0,durability:lifecycle:persistent"
func campfireTagsFromEnv() []string {
	raw := os.Getenv("RD_CAMPFIRE_TAGS")
	if raw == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// localCampfireBaseDir returns a persistent base directory for
// campfires (home, ready namespace). Uses CF_TRANSPORT_DIR if set, otherwise
// ~/.campfire/campfires/.
func localCampfireBaseDir() string {
	if env := os.Getenv("CF_TRANSPORT_DIR"); env != "" {
		return env
	}
	return filepath.Join(CFHome(), "campfires")
}

// createLocalCampfire creates a campfire with the filesystem transport at a
// persistent base directory. Used for non-project campfires (home, ready namespace)
// that don't have a project directory to root into.
func createLocalCampfire(agentID *identity.Identity, s store.Store, joinProtocol string, receptionReqs []string, description string) (string, error) {
	cf, err := campfirepkg.New(joinProtocol, receptionReqs, 1)
	if err != nil {
		return "", fmt.Errorf("creating campfire: %w", err)
	}
	cf.AddMember(agentID.PublicKey)
	campfireID := cf.PublicKeyHex()

	baseDir := localCampfireBaseDir()
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
	initCmd.Flags().Bool("confirm", false, "proceed without prompting even if campfire does not meet minimum durability requirements")
	rootCmd.AddCommand(initCmd)
}

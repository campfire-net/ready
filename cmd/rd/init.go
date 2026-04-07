package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/campfire-net/campfire/pkg/beacon"
	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/declarations"
	"github.com/campfire-net/ready/pkg/durability"
	"github.com/campfire-net/ready/pkg/rdconfig"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a ready work project",
	Long: `Initialize a ready work project in the current directory.

By default, creates a campfire and links it to the current directory.
Pass --offline to initialize in JSONL-only mode with no campfire required.

CAMPFIRE MODE (default):
  1. Creates a campfire with reception_requirements: ["work:create"]
  2. Writes .campfire/root (linking this directory to the campfire)
  3. Posts all convention:operation declarations (making the campfire self-describing)
  4. Publishes a beacon for local discovery
  5. Evaluates campfire durability and stores sync config in .ready/config.json
  6. Checks for a home campfire and reports what it finds

OFFLINE MODE (--offline):
  1. Creates .ready/ directory for local JSONL storage
  2. Writes .ready/project.json with project metadata
  No campfire, no network, no identity required.
  Use 'rd sync' later to connect to a campfire.

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
		offline, _ := cmd.Flags().GetBool("offline")
		beaconRoot, _ := cmd.Flags().GetString("beacon-root")

		// Accept optional positional argument as project name.
		positionalName := ""
		if len(args) > 0 {
			positionalName = args[0]
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}

		// Positional argument takes precedence over --name flag.
		if positionalName != "" {
			name = positionalName
		}

		// Default name from current directory.
		if name == "" {
			name = filepath.Base(cwd)
		}

		// --- JSONL-only offline mode ---
		if offline {
			return initOffline(cwd, name, description)
		}

		// Check we're not already initialized.
		if _, _, ok := projectRoot(); ok {
			return fmt.Errorf(".campfire/root already exists — this project is already initialized")
		}
		// Also check for .ready/ dir (JSONL-only already initialized).
		if _, err := os.Stat(filepath.Join(cwd, ".ready")); err == nil {
			return fmt.Errorf(".ready/ already exists — this project is already initialized (offline mode). Use 'rd sync' to connect to a campfire")
		}

		// Load client.
		client, err := requireClient()
		if err != nil {
			return err
		}

		// Default description.
		if description == "" {
			description = name + " work campfire"
		}

		// --- Create the campfire (state in ~/.campfire/campfires/<id>/) ---

		campfireDir := filepath.Join(cwd, ".campfire")
		campfireID, err := createLocalCampfire(client, campfireDir, "invite-only", []string{"work:create"}, description)
		if err != nil {
			return err
		}

		// --- Write .campfire/root (pointer in the project) ---

		if err := os.MkdirAll(campfireDir, 0700); err != nil {
			return fmt.Errorf("creating .campfire dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(campfireDir, "root"), []byte(campfireID), 0600); err != nil {
			return fmt.Errorf("writing .campfire/root: %w", err)
		}

		// --- Post convention:operation declarations via transport ---

		payloads, err := declarations.All()
		if err != nil {
			return fmt.Errorf("loading declarations: %w", err)
		}
		declClient, err := requireClient()
		if err != nil {
			return fmt.Errorf("initializing campfire client for declarations: %w", err)
		}
		nDecls := 0
		for _, payload := range payloads {
			_, err := declClient.Send(protocol.SendRequest{
				CampfireID: campfireID,
				Payload:    payload,
				Tags:       []string{"convention:operation"},
			})
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

		// --- Register project name in beacon root if configured ---

		if name != filepath.Base(cwd) || positionalName != "" {
			// Name was explicitly provided (not just defaulted from directory).
			// Attempt beacon registration if beacon root is configured.
			if err := registerProjectName(client, name, campfireID, beaconRoot); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not register project name: %v\n", err)
			}
		}

		// --- Write sync config with project name ---

		syncCfg.ProjectName = name
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

// registerProjectName attempts to register the project name through the beacon root.
// If beaconRoot is empty, checks CF_BEACON_ROOT environment variable and returns
// a warning if not configured (not an error — local-only binding is acceptable).
//
// If a beacon root is configured, walks the naming hierarchy to find or create the
// beacon and registers the project campfire under the given name.
func registerProjectName(client *protocol.Client, projectName string, campfireID string, beaconRoot string) error {
	// Determine the beacon root to use.
	if beaconRoot == "" {
		beaconRoot = os.Getenv("CF_BEACON_ROOT")
	}

	if beaconRoot == "" {
		// No beacon root configured — warning only, proceed with local binding.
		fmt.Fprintf(os.Stderr, "warning: no beacon root configured (set CF_BEACON_ROOT or pass --beacon-root) — project name bound locally only\n")
		return nil
	}

	// Beacon root is configured. Attempt to resolve and register under it.
	// This uses pkg/naming to walk the naming hierarchy and post a registration.
	ctx := context.Background()

	// For Wave 1.5, we do local registration only as a placeholder.
	// Full beacon traversal and naming resolution will come in Wave 2 with the
	// convention server. For now, just warn if the root is not local.
	if err := validateLocalBeaconRoot(beaconRoot); err != nil {
		fmt.Fprintf(os.Stderr, "warning: beacon root validation: %v\n", err)
		return nil
	}

	// Register the project name under the beacon root.
	_, err := naming.Register(ctx, client, beaconRoot, projectName, campfireID, nil)
	if err != nil {
		return fmt.Errorf("registering name in beacon: %w", err)
	}

	return nil
}

// validateLocalBeaconRoot checks if the beacon root exists and is resolvable
// as a local campfire ID (64-char hex). For Wave 1.5, this is a validation-only
// placeholder; full naming resolution comes in Wave 2.
func validateLocalBeaconRoot(beaconRoot string) error {
	// Check if it's a valid hex campfire ID (64 chars).
	if len(beaconRoot) == 64 {
		if _, err := hex.DecodeString(beaconRoot); err == nil {
			return nil // Valid local campfire ID.
		}
	}
	// For Wave 1.5, if it's not a valid local ID, return a warning message.
	// Full DNS-like resolution happens in Wave 2.
	return fmt.Errorf("beacon root %q is not a valid local campfire ID — Wave 2 naming resolution required", beaconRoot)
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
// {CFHome()}/campfires/ (e.g., ~/.cf/campfires/ or ~/.campfire/campfires/ for legacy users).
func localCampfireBaseDir() string {
	if env := os.Getenv("CF_TRANSPORT_DIR"); env != "" {
		return env
	}
	return filepath.Join(CFHome(), "campfires")
}

// createLocalCampfire creates a campfire with the filesystem transport at the
// persistent base directory (~/.campfire/campfires/ by default). Used for both
// project campfires and non-project campfires (home, ready namespace).
//
// projectDir is the project .campfire/ directory. When non-empty, the beacon is
// also published to projectDir/beacons/ for project-local discovery. Pass empty
// string to skip the project-local beacon publish (e.g. for home/namespace campfires).
func createLocalCampfire(client *protocol.Client, projectDir string, joinProtocol string, receptionReqs []string, description string) (string, error) {
	result, err := client.Create(protocol.CreateRequest{
		JoinProtocol:          joinProtocol,
		ReceptionRequirements: receptionReqs,
		Description:           description,
		Transport:             protocol.FilesystemTransport{Dir: localCampfireBaseDir()},
	})
	if err != nil {
		return "", err
	}

	// Project-local beacon publish (client.Create only publishes to ~/.campfire/beacons).
	if projectDir != "" && result.Beacon != nil {
		projectBeaconsDir := filepath.Join(projectDir, "beacons")
		_ = beacon.Publish(projectBeaconsDir, result.Beacon)
	}

	return result.CampfireID, nil
}

// initOffline initializes a project in JSONL-only mode — no campfire required.
// Creates .ready/ directory and writes .ready/project.json with project metadata.
func initOffline(cwd, name, description string) error {
	readyDir := filepath.Join(cwd, ".ready")

	// Check not already initialized.
	if _, err := os.Stat(readyDir); err == nil {
		return fmt.Errorf(".ready/ already exists — this project is already initialized")
	}
	if _, _, ok := projectRoot(); ok {
		return fmt.Errorf(".campfire/root already exists — this project is already initialized with a campfire")
	}

	if err := os.MkdirAll(readyDir, 0700); err != nil {
		return fmt.Errorf("creating .ready dir: %w", err)
	}

	if description == "" {
		description = name + " work project (offline)"
	}

	projectMeta := map[string]interface{}{
		"name":        name,
		"description": description,
		"mode":        "offline",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}
	metaBytes, err := json.MarshalIndent(projectMeta, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding project.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(readyDir, "project.json"), append(metaBytes, '\n'), 0600); err != nil {
		return fmt.Errorf("writing .ready/project.json: %w", err)
	}

	if jsonOutput {
		out := map[string]interface{}{
			"name":        name,
			"description": description,
			"mode":        "offline",
			"ready_dir":   readyDir,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("initialized %s (offline mode)\n", name)
	fmt.Printf("  storage: %s\n", readyDir)
	fmt.Println()
	fmt.Println("  all rd commands work without a campfire.")
	fmt.Println("  to connect to a campfire later, run 'rd init' in this directory.")
	return nil
}

func init() {
	initCmd.Flags().String("name", "", "project name (default: current directory name)")
	initCmd.Flags().String("description", "", "campfire description (default: '<name> work campfire')")
	initCmd.Flags().Bool("confirm", false, "proceed without prompting even if campfire does not meet minimum durability requirements")
	initCmd.Flags().Bool("offline", false, "initialize in JSONL-only mode (no campfire required)")
	initCmd.Flags().String("beacon-root", "", "beacon root campfire ID for naming registration (default: CF_BEACON_ROOT env var)")
	rootCmd.AddCommand(initCmd)
}

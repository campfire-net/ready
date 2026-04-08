package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/campfire-net/campfire/pkg/convention"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/conventionserver"
	"github.com/campfire-net/ready/pkg/declarations"
	"github.com/campfire-net/ready/pkg/provenance"
	"github.com/campfire-net/ready/pkg/rdconfig"
	"github.com/campfire-net/ready/pkg/resolve"
	"github.com/campfire-net/ready/pkg/state"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	jsonOutput     bool
	debugOutput    bool
	rdHome         string
	protocolClient *protocol.Client
)

var rootCmd = &cobra.Command{
	Use:   "rd",
	Short: "Ready — work management on campfire",
	Long: `Ready — work management as a campfire convention.

LIFECYCLE
  The work item lifecycle is: create → claim → close.

  rd create "Fix auth bug" --type task --priority p0
  rd claim <id>
  rd close <id> --reason "Was checking issuer, not audience"

QUERY
  rd ready                        what needs attention now
  rd list                         all open items
  rd list --status active --json  filtered, machine-readable
  rd show <id>                    full item details

DELEGATION
  rd delegate <id> --to <identity>
  rd ready --view delegated       see what you've delegated
  rd ready --view my-work         see what's assigned to you

SETUP
  rd init --name myproject        create a work campfire (one-time)
  rd register --org <name>        add to an org for multi-project (optional)

Work items live in your project's campfire. No database, no server.
https://ready.getcampfire.dev`,
	Version: Version,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	rootCmd.PersistentFlags().BoolVar(&debugOutput, "debug", false, "show hex IDs for diagnostics")
	rootCmd.PersistentFlags().StringVar(&rdHome, "cf-home", "", "campfire home directory (default: ~/.cf)")

	// Wire in the in-process convention server for solo mode.
	// PersistentPreRunE runs before every subcommand; if the client initializes
	// successfully and we're in solo mode, the server starts as a background goroutine
	// tied to the command's context (cancelled when the command exits).
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		client, err := requireClient()
		if err != nil {
			// Client init failure is non-fatal here — individual commands report it.
			return nil
		}
		requireConventionServer(cmd.Context(), client)
		return nil
	}
}

// requireConventionServer starts the in-process convention server in solo mode.
// Solo mode is detected via conventionserver.IsSoloMode: if no convention:server-binding
// exists other than our own, we start the in-process server so work operations are
// self-authorized. Loads InboxCampfireID from sync config to activate the inbox watcher.
func requireConventionServer(ctx context.Context, client *protocol.Client) {
	campfireID, projectDir, hasCampfire := projectRoot()
	if !hasCampfire || campfireID == "" {
		// JSONL-only mode — no campfire, no server needed.
		return
	}

	var opts []conventionserver.ServerOption
	if syncCfg, err := rdconfig.LoadSyncConfig(projectDir); err == nil && syncCfg != nil {
		if syncCfg.InboxCampfireID != "" {
			opts = append(opts, conventionserver.WithInboxCampfireID(syncCfg.InboxCampfireID))
		}
		if syncCfg.SummaryCampfireID != "" {
			opts = append(opts, conventionserver.WithSummaryCampfireID(syncCfg.SummaryCampfireID))
		}
	}

	srv, err := conventionserver.New(client, campfireID, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not start in-process convention server: %v\n", err)
		return
	}

	if !conventionserver.IsSoloMode(client, campfireID, srv.PubKeyHex()) {
		// A remote convention server is present — don't start a local one.
		return
	}

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not start in-process convention server: %v\n", err)
		return
	}
}

// CFHome returns the resolved campfire home directory.
// Detection order:
// (1) rdHome flag set → use it
// (2) CF_HOME env set → use it
// (3) ~/.cf exists → use it (new install path)
// (4) ~/.campfire exists → use it (legacy user migration path)
// (5) neither → default to ~/.cf
func CFHome() string {
	if rdHome != "" {
		return rdHome
	}
	if env := os.Getenv("CF_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	newPath := filepath.Join(home, ".cf")
	legacyPath := filepath.Join(home, ".campfire")

	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return newPath
}

// IdentityPath returns the path to the identity file.
func IdentityPath() string {
	return filepath.Join(CFHome(), "identity.json")
}

// openStore opens the campfire store at the default path.
func openStore() (store.Store, error) {
	s, err := store.Open(store.StorePath(CFHome()))
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}
	return s, nil
}

// requireAgentAndStore loads the agent identity and opens the store.
func requireAgentAndStore() (*identity.Identity, store.Store, error) {
	agentID, err := identity.Load(IdentityPath())
	if err != nil {
		return nil, nil, fmt.Errorf("loading identity (run 'cf init' first): %w", err)
	}
	s, err := store.Open(store.StorePath(CFHome()))
	if err != nil {
		return nil, nil, fmt.Errorf("opening store: %w", err)
	}
	return agentID, s, nil
}

// requireExecutor returns a convention.Executor backed by the protocol client,
// with a ProvenanceChecker wired in so that min_operator_level gates are
// enforced correctly.
//
// The checker reads work:role-grant messages from the local store. The campfire
// creator (from Membership.CreatorPubkey) is bootstrapped as maintainer (level 2).
// All others default to contributor (level 1) until an explicit role-grant
// message says otherwise.
func requireExecutor() (*convention.Executor, *protocol.Client, error) {
	client, err := requireClient()
	if err != nil {
		return nil, nil, err
	}
	exec := convention.NewExecutor(client, client.PublicKeyHex())

	// Wire in provenance checking so that min_operator_level gates work.
	// Best-effort: if we can't open the store or find the campfire, fall back to
	// no provenance checker rather than blocking all operations.
	if campfireID, _, ok := projectRoot(); ok && campfireID != "" {
		s, storeErr := openStore()
		if storeErr == nil {
			defer s.Close()
			var creatorKey string
			if m, memErr := s.GetMembership(campfireID); memErr == nil && m != nil {
				creatorKey = m.CreatorPubkey
			}
			checker, checkerErr := provenance.NewStoreChecker(s, campfireID, creatorKey)
			if checkerErr == nil {
				exec = exec.WithProvenance(checker)
			}
		}
	}

	return exec, client, nil
}

// loadDeclaration loads a convention declaration by operation name from the
// embedded declarations package and parses it for use with convention.Executor.
// The name corresponds to the operation name (e.g. "create", "claim", "gate-resolve").
func loadDeclaration(name string) (*convention.Declaration, error) {
	data, err := declarations.Load(name)
	if err != nil {
		return nil, err
	}
	decl, _, err := convention.Parse([]string{"convention:operation"}, data, "", "")
	if err != nil {
		return nil, fmt.Errorf("parsing declaration %q: %w", name, err)
	}
	return decl, nil
}

// requireClient returns a *protocol.Client backed by the campfire home directory.
// The client is cached after first initialization (CLI is single-threaded).
// Walk-up is enabled by default; WithAuthorizeFunc wires the center-campfire
// authorization flow.
func requireClient() (*protocol.Client, error) {
	if protocolClient != nil {
		return protocolClient, nil
	}
	c, err := protocol.Init(CFHome(), protocol.WithAuthorizeFunc(centerAuthorize))
	if err != nil {
		return nil, fmt.Errorf("initializing campfire client: %w", err)
	}
	protocolClient = c
	return c, nil
}

// centerAuthorize is the recentering authorize hook. It prompts the user once
// when Init() detects an unlinked center campfire. In non-interactive contexts
// (pipes, agents) it returns false to skip silently.
func centerAuthorize(description string) (bool, error) {
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return false, nil
	}
	fmt.Fprintf(os.Stderr, "rd: %s [y/N] ", description)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false, nil
	}
	return strings.EqualFold(strings.TrimSpace(scanner.Text()), "y"), nil
}

// readyProjectDir walks up from cwd looking for a .ready/ directory.
// Returns (projectDir, true) if found. This covers both campfire-backed
// projects (which have .campfire/root AND .ready/) and JSONL-only projects
// (which have only .ready/).
func readyProjectDir() (string, bool) {
	// First try via campfire root (campfire-backed projects).
	if _, dir, ok := projectRoot(); ok {
		if _, err := os.Stat(filepath.Join(dir, ".ready")); err == nil {
			return dir, true
		}
		// Campfire exists but .ready/ not yet created — still return the dir so
		// it can be created on first write.
		return dir, true
	}
	// Walk up looking for a .ready/ directory (JSONL-only projects).
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".ready")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// jsonlPath returns the path to .ready/mutations.jsonl for the current project.
// Returns an empty string if no project root is found (not initialized).
func jsonlPath() string {
	dir, ok := readyProjectDir()
	if !ok {
		return ""
	}
	return filepath.Join(dir, ".ready", "mutations.jsonl")
}

// pendingPath returns the path to .ready/pending.jsonl for the current project.
// Returns an empty string if no project root is found.
func pendingPath() string {
	dir, ok := readyProjectDir()
	if !ok {
		return ""
	}
	return filepath.Join(dir, ".ready", "pending.jsonl")
}

// allItemsFromJSONLOrStore returns all items, preferring JSONL when a project
// root exists, falling back to the campfire store when it does not.
func allItemsFromJSONLOrStore(s store.Store) ([]*state.Item, error) {
	if path := jsonlPath(); path != "" {
		// campfireID may be empty for JSONL-only projects; DeriveFromJSONL handles that.
		campfireID, _, _ := projectRoot()
		return resolve.AllItemsFromJSONL(path, campfireID)
	}
	return resolve.AllItems(s)
}

// byIDFromJSONLOrStore resolves an item by ID, preferring JSONL when available.
func byIDFromJSONLOrStore(s store.Store, itemID string) (*state.Item, error) {
	if path := jsonlPath(); path != "" {
		// campfireID may be empty for JSONL-only projects.
		campfireID, _, _ := projectRoot()
		return resolve.ByIDFromJSONL(path, campfireID, itemID)
	}
	return resolve.ByID(s, itemID)
}

// byIDFromJSONLOrStoreExact resolves an item by exact ID only — no prefix
// expansion. Use for security-sensitive operations (e.g. admit) where a prefix
// collision could allow an attacker to substitute a crafted item.
func byIDFromJSONLOrStoreExact(s store.Store, itemID string) (*state.Item, error) {
	if path := jsonlPath(); path != "" {
		campfireID, _, _ := projectRoot()
		return resolve.ByIDFromJSONLExact(path, campfireID, itemID)
	}
	return resolve.ByIDExact(s, itemID)
}

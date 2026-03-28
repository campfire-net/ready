package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/spf13/cobra"
	"github.com/campfire-net/ready/pkg/resolve"
	"github.com/campfire-net/ready/pkg/state"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	jsonOutput bool
	rdHome     string
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
	rootCmd.PersistentFlags().StringVar(&rdHome, "cf-home", "", "campfire home directory (default: ~/.campfire)")
}

// CFHome returns the resolved campfire home directory.
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
	return filepath.Join(home, ".campfire")
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

// jsonlPath returns the path to .ready/mutations.jsonl for the current project.
// Returns an empty string if no project root is found (not initialized).
func jsonlPath() string {
	_, projectDir, ok := projectRoot()
	if !ok {
		return ""
	}
	return filepath.Join(projectDir, ".ready", "mutations.jsonl")
}

// allItemsFromJSONLOrStore returns all items, preferring JSONL when a project
// root exists, falling back to the campfire store when it does not.
func allItemsFromJSONLOrStore(s store.Store) ([]*state.Item, error) {
	if path := jsonlPath(); path != "" {
		_, campfireID, _ := projectRoot()
		return resolve.AllItemsFromJSONL(path, campfireID)
	}
	return resolve.AllItems(s)
}

// byIDFromJSONLOrStore resolves an item by ID, preferring JSONL when available.
func byIDFromJSONLOrStore(s store.Store, itemID string) (*state.Item, error) {
	if path := jsonlPath(); path != "" {
		_, campfireID, _ := projectRoot()
		return resolve.ByIDFromJSONL(path, campfireID, itemID)
	}
	return resolve.ByID(s, itemID)
}

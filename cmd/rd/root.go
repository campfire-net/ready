package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/spf13/cobra"
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

  rd create    create a work item
  rd ready     show items needing attention now
  rd list      list all items
  rd show      show a single item
  rd close     close an item
  rd claim     claim a work item (accept delegation, transition to active)
  rd update    update fields on a work item
  rd delegate  delegate a work item to another party

  Work items live in your campfire. The campfire is the backend.`,
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

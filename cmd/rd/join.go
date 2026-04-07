package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/spf13/cobra"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

// defaultBeaconRoot is the compiled-in default beacon root ID.
// Empty string means no default is compiled in — any first use is a TOFU event.
const defaultBeaconRoot = ""

var joinCmd = &cobra.Command{
	Use:   "join <campfire-id>",
	Short: "Join a remote campfire for cross-project item resolution",
	Long: `Join a remote campfire so that cross-campfire item dependencies can be resolved.

When a work item in this project references a dep in another campfire
(e.g. "acme.frontend.item-abc"), rd needs to be a member of that campfire
to show the dep's status.

TOFU PINNING
  The first time you join using a non-default beacon root, rd warns you and
  pins the beacon root in ~/.campfire/rd.json. On subsequent joins, any
  deviation from the pinned root triggers a warning requiring confirmation.

  To reset the pinned root:
    rd join --reset-beacon-root

EXAMPLES
  rd join <campfire-id>              join using the default beacon root
  rd join <campfire-id> --root <id>  join using a specific beacon root
  rd join --reset-beacon-root        clear the pinned beacon root`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resetRoot, _ := cmd.Flags().GetBool("reset-beacon-root")
		beaconRootFlag, _ := cmd.Flags().GetString("root")
		confirm, _ := cmd.Flags().GetBool("confirm")

		cfg, err := rdconfig.Load(CFHome())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Handle --reset-beacon-root.
		if resetRoot {
			if cfg.BeaconRoot == "" {
				fmt.Println("no beacon root pinned")
				return nil
			}
			prev := cfg.BeaconRoot
			cfg.BeaconRoot = ""
			if err := rdconfig.Save(CFHome(), cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("beacon root pin cleared (was: %s...)\n", prev[:min(12, len(prev))])
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("campfire-id required (use --reset-beacon-root to clear the pinned beacon root)")
		}

		campfireID := args[0]
		beaconRoot := beaconRootFlag

		// TOFU pinning logic.
		if beaconRoot != defaultBeaconRoot && beaconRoot != "" {
			if cfg.BeaconRoot == "" {
				// First use of a non-default beacon root — warn and pin (TOFU).
				fmt.Fprintf(os.Stderr, "warning: first use of non-default beacon root %s...\n", beaconRoot[:min(12, len(beaconRoot))])
				fmt.Fprintf(os.Stderr, "  this root will be pinned (TOFU) in ~/.campfire/rd.json\n")
				fmt.Fprintf(os.Stderr, "  future joins using a different root will require --confirm\n")

				if !confirm {
					if !isInteractive() {
						// Non-interactive: pin silently (agent context).
						fmt.Fprintf(os.Stderr, "  non-interactive: pinning beacon root automatically\n")
					} else {
						fmt.Fprint(os.Stderr, "pin this beacon root? [Y/n] ")
						scanner := bufio.NewScanner(os.Stdin)
						if scanner.Scan() {
							answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
							if answer == "n" || answer == "no" {
								return fmt.Errorf("aborted: beacon root not pinned")
							}
						}
					}
				}
				cfg.BeaconRoot = beaconRoot
				if err := rdconfig.Save(CFHome(), cfg); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not pin beacon root: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "  pinned beacon root: %s...\n", beaconRoot[:min(12, len(beaconRoot))])
				}
			} else if cfg.BeaconRoot != beaconRoot {
				// Deviation from pinned root — warn, require confirmation.
				fmt.Fprintf(os.Stderr, "warning: beacon root mismatch\n")
				fmt.Fprintf(os.Stderr, "  pinned:    %s...\n", cfg.BeaconRoot[:min(12, len(cfg.BeaconRoot))])
				fmt.Fprintf(os.Stderr, "  requested: %s...\n", beaconRoot[:min(12, len(beaconRoot))])
				if !confirm {
					return fmt.Errorf("beacon root does not match pinned root — pass --confirm to proceed or use 'rd join --reset-beacon-root' to re-pin")
				}
			}
		}

		// Join the campfire using the filesystem transport (local dev).
		client, err := requireClient()
		if err != nil {
			return err
		}
		if _, err := client.Join(protocol.JoinRequest{
			CampfireID: campfireID,
			Transport:  protocol.FilesystemTransport{Dir: localCampfireBaseDir()},
		}); err != nil {
			return fmt.Errorf("joining campfire %s: %w", campfireID[:min(12, len(campfireID))], err)
		}

		fmt.Printf("joined campfire %s...\n", campfireID[:min(12, len(campfireID))])
		fmt.Println("  cross-campfire deps referencing this campfire will now resolve")
		return nil
	},
}

// isInteractive reports whether stdin is a terminal.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	joinCmd.Flags().String("root", "", "beacon root campfire ID to use for TOFU pinning")
	joinCmd.Flags().Bool("reset-beacon-root", false, "clear the pinned beacon root")
	joinCmd.Flags().Bool("confirm", false, "confirm beacon root deviation without prompting")
	rootCmd.AddCommand(joinCmd)
}

// Package crossdep resolves cross-campfire item dependencies.
//
// A cross-campfire dep reference has the form "acme.frontend.item-abc" where
// "acme.frontend" is the campfire name path and "item-abc" is the item ID.
// Resolution requires the user to be a member of the target campfire.
//
// Cross-campfire deps are always NON-BLOCKING: if resolution fails (not a
// member, network error, etc.) the item remains actionable and a warning is
// surfaced instead.
package crossdep

import (
	"fmt"
	"strings"

	"github.com/campfire-net/campfire/pkg/naming"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/ready/pkg/state"
)

// ResolvedDep holds the result of resolving a cross-campfire dep reference.
type ResolvedDep struct {
	// Ref is the original cross-campfire reference string.
	Ref string
	// CampfireName is the campfire name path portion.
	CampfireName string
	// ItemID is the item ID portion.
	ItemID string
	// Item is the resolved item, or nil if resolution failed.
	Item *state.Item
	// Warning is set when resolution fails; empty on success.
	Warning string
}

// ResolveDeps attempts to resolve all cross-campfire dep warnings on the item
// by looking up the referenced campfires in the store's membership list.
//
// The aliases parameter provides alias lookups (campfire name → campfire ID).
// When an alias matches, the item is fetched from that campfire's store.
//
// Returns a list of resolved results, one per warning. Items whose campfire
// is not in the membership list are returned with Warning set.
func ResolveDeps(item *state.Item, s store.Store, aliases *naming.AliasStore) []ResolvedDep {
	if len(item.CrossCampfireWarnings) == 0 {
		return nil
	}

	// Build a map of campfire ID → derived items for campfires we're a member of.
	memberships, err := s.ListMemberships()
	memberMap := make(map[string]map[string]*state.Item)
	if err == nil {
		for _, m := range memberships {
			derived, deriveErr := state.DeriveFromStore(s, m.CampfireID)
			if deriveErr == nil {
				memberMap[m.CampfireID] = derived
			}
		}
	}

	var results []ResolvedDep
	for _, warn := range item.CrossCampfireWarnings {
		ref := extractRef(warn)
		parsed := state.ParseCrossCampfireRef(ref)
		if parsed == nil {
			results = append(results, ResolvedDep{
				Ref:     ref,
				Warning: warn,
			})
			continue
		}

		resolved := resolveSingle(parsed, memberMap, aliases)
		results = append(results, resolved)
	}
	return results
}

// resolveSingle attempts to resolve a single cross-campfire ref.
func resolveSingle(parsed *state.CrossCampfireRef, memberMap map[string]map[string]*state.Item, aliases *naming.AliasStore) ResolvedDep {
	dep := ResolvedDep{
		Ref:          parsed.Raw,
		CampfireName: parsed.CampfireName,
		ItemID:       parsed.ItemID,
	}

	// Try to find campfire ID from aliases.
	campfireID := ""
	if aliases != nil {
		if id, err := aliases.Get(parsed.CampfireName); err == nil && id != "" {
			campfireID = id
		}
	}

	if campfireID == "" {
		dep.Warning = fmt.Sprintf("unresolved cross-campfire dep: %s (campfire not in local aliases — not a member or not discovered)", parsed.Raw)
		return dep
	}

	// Check membership.
	items, isMember := memberMap[campfireID]
	if !isMember {
		dep.Warning = fmt.Sprintf("unresolved cross-campfire dep: %s (not a member of campfire %s)", parsed.Raw, shortID(campfireID))
		return dep
	}

	// Look up item.
	item, ok := items[parsed.ItemID]
	if !ok {
		// Try prefix match.
		var matches []*state.Item
		for id, it := range items {
			if strings.HasPrefix(id, parsed.ItemID) {
				matches = append(matches, it)
			}
		}
		if len(matches) == 1 {
			item = matches[0]
			ok = true
		} else if len(matches) > 1 {
			dep.Warning = fmt.Sprintf("ambiguous cross-campfire dep: %s (multiple matches in campfire %s)", parsed.Raw, shortID(campfireID))
			return dep
		}
	}

	if !ok {
		dep.Warning = fmt.Sprintf("unresolved cross-campfire dep: %s (item %q not found in campfire %s)", parsed.Raw, parsed.ItemID, shortID(campfireID))
		return dep
	}

	dep.Item = item
	return dep
}

// extractRef extracts the cross-campfire ref from a warning string.
// Warning format: "unresolved cross-campfire dep: <ref> (...)"
func extractRef(warn string) string {
	const prefix = "unresolved cross-campfire dep: "
	if !strings.HasPrefix(warn, prefix) {
		return warn
	}
	rest := warn[len(prefix):]
	// ref ends at the first space.
	if idx := strings.Index(rest, " "); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// shortID returns the first 12 characters of a campfire ID for display, or
// the full string if shorter than 12 chars.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "..."
}

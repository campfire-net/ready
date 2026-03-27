// Package resolve provides item ID resolution for the rd CLI.
// Item IDs are project-prefixed strings (e.g. "ready-a1b"). Resolution
// looks up a work:create message by scanning the campfire message store.
package resolve

import (
	"fmt"
	"strings"

	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/ready/pkg/state"
)

// ErrNotFound is returned when an item ID cannot be resolved.
type ErrNotFound struct {
	ID string
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("item %q not found", e.ID)
}

// ErrAmbiguous is returned when a prefix matches multiple items.
type ErrAmbiguous struct {
	Prefix   string
	Matches  []string
}

func (e ErrAmbiguous) Error() string {
	return fmt.Sprintf("prefix %q is ambiguous: matches %s", e.Prefix, strings.Join(e.Matches, ", "))
}

// ByID resolves an item by its exact ID or a unique prefix.
// Returns the item and the campfire ID it was found in.
// Searches all campfires the agent is a member of.
func ByID(s store.Store, itemID string) (*state.Item, error) {
	memberships, err := s.ListMemberships()
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}

	// First pass: check for an exact match across all campfires.
	// If an exact match exists, return it immediately without prefix scanning.
	for _, m := range memberships {
		items, err := state.DeriveFromStore(s, m.CampfireID)
		if err != nil {
			continue
		}
		if item, ok := items[itemID]; ok {
			return item, nil
		}
	}

	// Second pass: prefix match (no exact match found).
	var matches []*state.Item
	for _, m := range memberships {
		items, err := state.DeriveFromStore(s, m.CampfireID)
		if err != nil {
			continue
		}
		for id, item := range items {
			if strings.HasPrefix(id, itemID) {
				matches = append(matches, item)
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, ErrNotFound{ID: itemID}
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, ErrAmbiguous{Prefix: itemID, Matches: ids}
	}
}

// AllItems returns all items across all campfires the agent is a member of.
func AllItems(s store.Store) ([]*state.Item, error) {
	memberships, err := s.ListMemberships()
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}

	var all []*state.Item
	seen := map[string]bool{}
	for _, m := range memberships {
		items, err := state.DeriveFromStore(s, m.CampfireID)
		if err != nil {
			continue
		}
		for _, item := range items {
			if !seen[item.ID] {
				seen[item.ID] = true
				all = append(all, item)
			}
		}
	}
	return all, nil
}

// AllItemsInCampfire returns all items derived from the given campfire.
func AllItemsInCampfire(s store.Store, campfireID string) ([]*state.Item, error) {
	items, err := state.DeriveFromStore(s, campfireID)
	if err != nil {
		return nil, err
	}
	var all []*state.Item
	for _, item := range items {
		all = append(all, item)
	}
	return all, nil
}

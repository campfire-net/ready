// Package declarations embeds the work management convention:operation JSON
// declarations and provides functions to post them to a campfire.
package declarations

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/message"
	"github.com/campfire-net/campfire/pkg/store"
)

//go:embed ops/*.json
var opsFS embed.FS

// All returns all convention:operation declaration payloads as raw JSON.
func All() ([][]byte, error) {
	entries, err := fs.ReadDir(opsFS, "ops")
	if err != nil {
		return nil, fmt.Errorf("reading embedded ops: %w", err)
	}
	var payloads [][]byte
	for _, e := range entries {
		data, err := opsFS.ReadFile("ops/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		payloads = append(payloads, data)
	}
	return payloads, nil
}

// PostAll signs and stores all convention:operation declarations in the given
// campfire. Each declaration becomes a signed message tagged "convention:operation".
func PostAll(agentID *identity.Identity, s store.Store, campfireID string) (int, error) {
	payloads, err := All()
	if err != nil {
		return 0, err
	}
	tags := []string{"convention:operation"}
	for i, payload := range payloads {
		msg, err := message.NewMessage(agentID.PrivateKey, agentID.PublicKey, payload, tags, nil)
		if err != nil {
			return i, fmt.Errorf("creating message for declaration %d: %w", i, err)
		}
		rec := store.MessageRecord{
			ID:         msg.ID,
			CampfireID: campfireID,
			Sender:     agentID.PublicKeyHex(),
			Payload:    payload,
			Tags:       tags,
			Timestamp:  msg.Timestamp,
			Signature:  msg.Signature,
			ReceivedAt: store.NowNano(),
		}
		if _, err := s.AddMessage(rec); err != nil {
			return i, fmt.Errorf("storing declaration %d: %w", i, err)
		}
	}
	return len(payloads), nil
}

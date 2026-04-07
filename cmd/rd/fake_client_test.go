package main

// fake_client_test.go — shared fake campfire client implementations for testing
// join/revoke/admit helpers that depend on the campfireReader and campfireAdmitter
// interfaces.

import (
	"encoding/json"
	"fmt"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
)

// fakeReadClient implements campfireReader by returning a pre-configured set
// of messages for any Read call. All Read calls return the same messages
// regardless of campfireID, tags, or sender filter.
//
// The Sender field in ReadRequest is intentionally not filtered here because
// the real filtering logic is in findMembersAdmittedBy's caller (revoke.go) or
// the campfire store. For tests, we control which messages we inject.
type fakeReadClient struct {
	messages []protocol.Message
	err      error // if non-nil, Read returns this error
}

func (f *fakeReadClient) Read(_ protocol.ReadRequest) (*protocol.ReadResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &protocol.ReadResult{Messages: f.messages}, nil
}

// fakeAdmitClient implements campfireAdmitter with configurable behavior.
// Records calls to Admit for verification.
type fakeAdmitClient struct {
	membership     *store.Membership
	membershipErr  error
	admitErr       error
	admitCalls     []protocol.AdmitRequest
}

func (f *fakeAdmitClient) GetMembership(_ string) (*store.Membership, error) {
	if f.membershipErr != nil {
		return nil, f.membershipErr
	}
	return f.membership, nil
}

func (f *fakeAdmitClient) Admit(req protocol.AdmitRequest) error {
	f.admitCalls = append(f.admitCalls, req)
	return f.admitErr
}

// makeGrantMsg builds a protocol.Message with a work:role-grant payload
// targeting the given pubkey with the given role.
func makeGrantMsg(id, pubkey, role string) protocol.Message {
	payload := map[string]string{"pubkey": pubkey, "role": role}
	data, _ := json.Marshal(payload)
	return protocol.Message{
		ID:      id,
		Payload: data,
		Tags:    []string{"work:role-grant"},
	}
}

// makeGrantMsgForTag builds a message with both payload and "work:for:<pubkey>" tag.
func makeGrantMsgForTag(id, pubkey, role string) protocol.Message {
	payload := map[string]string{"pubkey": pubkey, "role": role}
	data, _ := json.Marshal(payload)
	return protocol.Message{
		ID:      id,
		Payload: data,
		Tags:    []string{"work:role-grant", "work:for:" + pubkey},
	}
}

// pubkeyHex returns a valid 64-char hex string by repeating the given 2-char hex byte.
// Panics if hex is not 2 chars. Used to build predictable test pubkeys.
func pubkeyHex(hex2 string) string {
	if len(hex2) != 2 {
		panic(fmt.Sprintf("pubkeyHex: want 2-char hex, got %q", hex2))
	}
	result := make([]byte, 64)
	for i := 0; i < 64; i += 2 {
		result[i] = hex2[0]
		result[i+1] = hex2[1]
	}
	return string(result)
}

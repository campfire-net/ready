package views_test

import (
	"testing"
	"time"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/views"
)

// makeItem builds a minimal state.Item for testing.
func makeItem(id, status, priority, eta, forParty, by string) *state.Item {
	return &state.Item{
		ID:       id,
		Title:    "Test: " + id,
		Status:   status,
		Priority: priority,
		ETA:      eta,
		For:      forParty,
		By:       by,
		Type:     "task",
	}
}

func futureETA(d time.Duration) string {
	return time.Now().Add(d).UTC().Format(time.RFC3339)
}

func pastETA(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

func TestReadyFilter_NotTerminal(t *testing.T) {
	f := views.ReadyFilter()

	// Active item with near ETA should appear.
	item := makeItem("t1", state.StatusActive, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if !f(item) {
		t.Error("expected active item with near ETA to be ready")
	}

	// Done item should not appear.
	item2 := makeItem("t2", state.StatusDone, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(item2) {
		t.Error("expected done item to not be ready")
	}
}

func TestReadyFilter_Blocked(t *testing.T) {
	f := views.ReadyFilter()
	item := makeItem("t1", state.StatusBlocked, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(item) {
		t.Error("expected blocked item to not be ready")
	}
}

func TestReadyFilter_ETAFarFuture(t *testing.T) {
	f := views.ReadyFilter()
	// ETA 10h away — still ready. ETA is for sorting, not filtering.
	// Work due in the future is still workable now.
	item := makeItem("t1", state.StatusInbox, "p2", futureETA(10*time.Hour), "a@b.com", "")
	if !f(item) {
		t.Error("expected item with ETA 10h away to still be ready (ETA is for sorting)")
	}
}

func TestReadyFilter_ScheduledExcluded(t *testing.T) {
	f := views.ReadyFilter()
	// Scheduled items are pending a future date — state isn't accurate yet.
	item := makeItem("t1", state.StatusScheduled, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(item) {
		t.Error("expected scheduled item to not be ready")
	}
}

func TestReadyFilter_ETAWithin4h(t *testing.T) {
	f := views.ReadyFilter()
	item := makeItem("t1", state.StatusInbox, "p1", futureETA(2*time.Hour), "a@b.com", "")
	if !f(item) {
		t.Error("expected item with ETA 2h away to be ready")
	}
}

func TestWorkFilter(t *testing.T) {
	f := views.WorkFilter()

	active := makeItem("t1", state.StatusActive, "p1", "", "a@b.com", "me")
	if !f(active) {
		t.Error("expected active item to appear in work view")
	}

	inbox := makeItem("t2", state.StatusInbox, "p1", "", "a@b.com", "")
	if f(inbox) {
		t.Error("expected inbox item to not appear in work view")
	}
}

func TestPendingFilter(t *testing.T) {
	f := views.PendingFilter()

	for _, status := range []string{state.StatusWaiting, state.StatusScheduled, state.StatusBlocked} {
		item := makeItem("t1", status, "p1", "", "a@b.com", "")
		if !f(item) {
			t.Errorf("expected %s item to appear in pending view", status)
		}
	}

	for _, status := range []string{state.StatusInbox, state.StatusActive, state.StatusDone} {
		item := makeItem("t1", status, "p1", "", "a@b.com", "")
		if f(item) {
			t.Errorf("expected %s item to not appear in pending view", status)
		}
	}
}

func TestOverdueFilter(t *testing.T) {
	f := views.OverdueFilter()

	overdue := makeItem("t1", state.StatusInbox, "p1", pastETA(1*time.Hour), "a@b.com", "")
	if !f(overdue) {
		t.Error("expected past-ETA item to appear in overdue view")
	}

	future := makeItem("t2", state.StatusInbox, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(future) {
		t.Error("expected future-ETA item to not appear in overdue view")
	}

	done := makeItem("t3", state.StatusDone, "p1", pastETA(1*time.Hour), "a@b.com", "")
	if f(done) {
		t.Error("expected done item with past ETA to not appear in overdue view")
	}
}

func TestDelegatedFilter(t *testing.T) {
	f := views.DelegatedFilter("baron@3dl.dev")

	// baron delegated to someone else, active.
	delegated := makeItem("t1", state.StatusActive, "p1", "", "baron@3dl.dev", "agent@3dl.dev")
	if !f(delegated) {
		t.Error("expected delegated item to appear in delegated view")
	}

	// baron doing it himself.
	self := makeItem("t2", state.StatusActive, "p1", "", "baron@3dl.dev", "baron@3dl.dev")
	if f(self) {
		t.Error("expected self-assigned item to not appear in delegated view")
	}

	// Not baron's item.
	other := makeItem("t3", state.StatusActive, "p1", "", "other@3dl.dev", "agent@3dl.dev")
	if f(other) {
		t.Error("expected other's item to not appear in baron's delegated view")
	}
}

func TestMyWorkFilter(t *testing.T) {
	f := views.MyWorkFilter("me@test.com")

	assigned := makeItem("t1", state.StatusActive, "p1", "", "boss@test.com", "me@test.com")
	if !f(assigned) {
		t.Error("expected assigned item to appear in my-work view")
	}

	notMe := makeItem("t2", state.StatusActive, "p1", "", "boss@test.com", "other@test.com")
	if f(notMe) {
		t.Error("expected other's item to not appear in my-work view")
	}

	done := makeItem("t3", state.StatusDone, "p1", "", "boss@test.com", "me@test.com")
	if f(done) {
		t.Error("expected done item to not appear in my-work view")
	}
}

func TestApply(t *testing.T) {
	items := []*state.Item{
		makeItem("t1", state.StatusActive, "p1", "", "a@b.com", ""),
		makeItem("t2", state.StatusDone, "p1", "", "a@b.com", ""),
		makeItem("t3", state.StatusActive, "p2", "", "a@b.com", ""),
	}

	f := views.WorkFilter()
	result := views.Apply(items, f)
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
}

func TestNamed_ReturnsNilForUnknown(t *testing.T) {
	f := views.Named("nonexistent", "")
	if f != nil {
		t.Error("expected nil filter for unknown view name")
	}
}

func TestNamed_AllViewsResolvable(t *testing.T) {
	for _, name := range views.AllNames() {
		f := views.Named(name, "test@test.com")
		if f == nil {
			t.Errorf("expected non-nil filter for view %q", name)
		}
	}
}

func TestFocusFilter_NoGate(t *testing.T) {
	f := views.FocusFilter("")

	// Active item with near ETA and no gate — should appear.
	item := makeItem("t1", state.StatusActive, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if !f(item) {
		t.Error("expected active near-ETA item to appear in focus view with no gate filter")
	}

	// Done item — should not appear.
	done := makeItem("t2", state.StatusDone, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(done) {
		t.Error("expected done item to not appear in focus view")
	}

	// Item with far ETA — still appears (ETA is for sorting, not filtering).
	far := makeItem("t3", state.StatusActive, "p1", futureETA(10*time.Hour), "a@b.com", "")
	if !f(far) {
		t.Error("expected far-ETA active item to appear in focus view (ETA is for sorting)")
	}

	// Scheduled item — should NOT appear (pending a date).
	sched := makeItem("t4", state.StatusScheduled, "p1", futureETA(1*time.Hour), "a@b.com", "")
	if f(sched) {
		t.Error("expected scheduled item to not appear in focus view")
	}
}

func TestFocusFilter_WithGate(t *testing.T) {
	// Add gate to makeItem by constructing directly.
	nearETA := futureETA(1 * time.Hour)

	designItem := &state.Item{
		ID: "t1", Status: state.StatusActive, Priority: "p1",
		ETA: nearETA, For: "a@b.com", Type: "task", Gate: "design",
	}
	noGateItem := &state.Item{
		ID: "t2", Status: state.StatusActive, Priority: "p1",
		ETA: nearETA, For: "a@b.com", Type: "task", Gate: "",
	}
	budgetItem := &state.Item{
		ID: "t3", Status: state.StatusActive, Priority: "p1",
		ETA: nearETA, For: "a@b.com", Type: "task", Gate: "budget",
	}

	f := views.FocusFilter("design")

	if !f(designItem) {
		t.Error("expected design gate item to appear with FocusFilter(design)")
	}
	if f(noGateItem) {
		t.Error("expected no-gate item to not appear with FocusFilter(design)")
	}
	if f(budgetItem) {
		t.Error("expected budget gate item to not appear with FocusFilter(design)")
	}
}

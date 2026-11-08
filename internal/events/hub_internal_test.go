package events

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// The viewers the tests subscribe as.
func alice() Viewer { return Viewer{UserID: 1} }
func bob() Viewer   { return Viewer{UserID: 2} }
func admin() Viewer { return Viewer{UserID: 3, Admin: true} }

// What a fresh stream and a lost one open with.
const (
	heardReady    = "ready:{}"
	heardResync   = "resync:{}"
	heardLibrary1 = "library:1"
)

// drain takes what s holds now, without waiting.
func drain(s *Subscription) []Event {
	var out []Event
	for {
		select {
		case ev := <-s.C():
			out = append(out, ev)
		default:
			return out
		}
	}
}

// kinds lists the kinds and data of evs, as "kind:data".
func kinds(evs []Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = string(ev.Kind) + ":" + string(ev.Data)
	}
	return out
}

func TestHubFiltersByPersonAndRole(t *testing.T) {
	t.Parallel()
	h := New(Options{})
	subs := map[string]*Subscription{"alice": h.Subscribe(alice(), ""), "bob": h.Subscribe(bob(), ""), "admin": h.Subscribe(admin(), "")}
	h.Publish(Library, Everyone(), []byte(`1`), nil)
	h.Publish(Marks, User(alice().UserID), []byte(`2`), nil)
	h.Publish(Settings, Admins(), []byte(`3`), nil)
	h.Publish(Scan, Everyone(), []byte(`4`), []byte(`4+why`))
	want := map[string][]string{
		"alice": {heardReady, heardLibrary1, "marks:2", "scan:4"},
		"bob":   {heardReady, heardLibrary1, "scan:4"},
		"admin": {heardReady, heardLibrary1, "settings:3", "scan:4+why"},
	}
	for name, s := range subs {
		if got := kinds(drain(s)); !slices.Equal(got, want[name]) {
			t.Errorf("%s heard %v, want %v", name, got, want[name])
		}
	}
}

func TestHubKeepsOrderAndNumbersEvents(t *testing.T) {
	t.Parallel()
	h := New(Options{})
	s := h.Subscribe(alice(), "")
	for range 5 {
		h.Publish(Library, Everyone(), []byte(`{}`), nil)
	}
	evs := drain(s)
	ids := make([]string, len(evs))
	for i, ev := range evs {
		ids[i] = ev.ID
	}
	if len(ids) != 6 || !slices.IsSortedFunc(ids[1:], func(a, b string) int { return strings.Compare(pad(a), pad(b)) }) {
		t.Errorf("ids = %v, want ready and five in order", ids)
	}
	if ids[0] != h.id(0) || ids[5] != h.id(5) {
		t.Errorf("ids = %v, want ready at %s and the last at %s", ids, h.id(0), h.id(5))
	}
}

// pad makes ids of one run compare in number order.
func pad(id string) string {
	epoch, n, _ := strings.Cut(id, "-")
	return epoch + strings.Repeat("0", 20-len(n)) + n
}

func TestHubResumesFromTheLastID(t *testing.T) {
	t.Parallel()
	h := New(Options{})
	h.Publish(Library, Everyone(), []byte(`1`), nil)
	h.Publish(Marks, User(bob().UserID), []byte(`bob()'s`), nil)
	h.Publish(Library, Everyone(), []byte(`2`), nil)
	h.Publish(Library, Everyone(), []byte(`3`), nil)
	s := h.Subscribe(alice(), h.id(1))
	if got, want := kinds(drain(s)), []string{heardReady, "library:2", "library:3"}; !slices.Equal(got, want) {
		t.Errorf("resumed with %v, want %v", got, want)
	}
	up := h.Subscribe(alice(), h.id(4))
	if got, want := kinds(drain(up)), []string{heardReady}; !slices.Equal(got, want) {
		t.Errorf("up to date, heard %v, want %v", got, want)
	}
}

func TestHubResyncsWhatItNoLongerHas(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000_000, 0)
	var mu sync.Mutex
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	h := New(Options{Max: 3, Now: clock})
	for range 5 {
		h.Publish(Library, Everyone(), []byte(`{}`), nil)
	}
	other := New(Options{})
	for name, id := range map[string]string{
		"pushed out by count": h.id(1), "another run's": other.id(1), "from the future": h.id(9),
		"not an id": "nonsense", "no number": h.epoch + "-x",
	} {
		if got := kinds(drain(h.Subscribe(alice(), id))); !slices.Equal(got, []string{heardResync}) {
			t.Errorf("%s: heard %v, want resync", name, got)
		}
	}
	if got := kinds(drain(h.Subscribe(alice(), h.id(2)))); len(got) != 4 || got[0] != heardReady {
		t.Errorf("at the edge of what is kept, heard %v, want ready and three", got)
	}
	mu.Lock()
	now = now.Add(defaultKeep + time.Second)
	mu.Unlock()
	if got := kinds(drain(h.Subscribe(alice(), h.id(4)))); !slices.Equal(got, []string{heardResync}) {
		t.Errorf("past the age kept, heard %v, want resync", got)
	}
	if got := kinds(drain(h.Subscribe(alice(), h.id(5)))); !slices.Equal(got, []string{heardReady}) {
		t.Errorf("up to date past the age kept, heard %v, want ready", got)
	}
}

func TestHubDropsASlowSubscriber(t *testing.T) {
	t.Parallel()
	h := New(Options{Queue: 2})
	slow, quick := h.Subscribe(alice(), ""), h.Subscribe(bob(), "")
	for i := range 4 {
		h.Publish(Library, Everyone(), []byte(`{}`), nil)
		if i < 2 {
			drain(quick)
		}
	}
	select {
	case <-slow.Done():
	default:
		t.Fatal("the slow subscriber is still subscribed")
	}
	select {
	case <-quick.Done():
		t.Fatal("the quick subscriber was dropped")
	default:
	}
	if n := h.Subscribers(); n != 1 {
		t.Errorf("subscribers = %d, want 1", n)
	}
	h.Unsubscribe(quick)
	h.Unsubscribe(quick) // Twice is fine.
	if n := h.Subscribers(); n != 0 {
		t.Errorf("subscribers = %d, want 0", n)
	}
}

func TestHubIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	h := New(Options{Max: 50})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				h.Publish(Library, Everyone(), []byte(`{}`), nil)
			}
		})
		wg.Go(func() {
			for range 50 {
				s := h.Subscribe(alice(), h.id(1))
				drain(s)
				h.Unsubscribe(s)
			}
		})
	}
	wg.Wait()
}

// Package events carries what changes on the server to every client that may
// hear it, as server-sent events.
//
// A [Hub] numbers each event, keeps the last few minutes of them, and hands
// them to its subscribers, each filtered by who that subscriber is. An event
// carries ids and versions, never whole objects: a client fetches what it
// needs through the routes, which decide what it may see. The hub never looks
// inside an event's data.
//
// A subscriber that can't keep up is dropped; it reconnects with the id of the
// last event it took, and gets what it missed while the hub still has it, or
// [Resync] when it doesn't.
package events

import (
	"crypto/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kind names what an event is about. It is the SSE event name.
type Kind string

// The kinds of event.
const (
	Library      Kind = "library"       // Albums added, changed or removed: everyone.
	Scan         Kind = "scan"          // A scan's progress: everyone, the failure for admins.
	Fingerprints Kind = "fingerprints"  // The fingerprint pass's progress: admins.
	Loudness     Kind = "loudness"      // The loudness pass's progress: admins.
	Marks        Kind = "marks"         // A person's hearts and stars.
	Prefs        Kind = "prefs"         // A person's preference changed.
	ShuffleRules Kind = "shuffle-rules" // A person's shuffle rules changed.
	Shuffle      Kind = "shuffle"       // One of a person's shuffles moved on.
	Listening    Kind = "listening"     // What a person's devices are playing.
	Lastfm       Kind = "lastfm"        // A person's Last.fm connection changed.
	Settings     Kind = "settings"      // Server settings changed, by key: admins.
	// Resync tells a subscriber it missed events the hub no longer has: it
	// must fetch everything again. The hub sends it; nothing publishes it.
	Resync Kind = "resync"
	// Ready opens every stream, with the id to resume from.
	Ready Kind = "ready"
)

const (
	// defaultKeep is how long the hub keeps events for a client that comes back.
	defaultKeep = 5 * time.Minute
	// defaultMax is the most events it keeps.
	defaultMax = 10_000
	// defaultQueue is how many events wait for one subscriber before it is dropped.
	defaultQueue = 256
)

// Audience says who hears an event.
type Audience struct {
	user   int64 // 0 means not one person.
	admins bool
}

// Everyone is every signed-in person.
func Everyone() Audience { return Audience{} }

// Admins is every administrator.
func Admins() Audience { return Audience{admins: true} }

// User is one person, on all their devices.
func User(id int64) Audience { return Audience{user: id} }

// Viewer is who a subscriber is.
type Viewer struct {
	UserID int64
	Admin  bool
}

func (a Audience) includes(v Viewer) bool {
	switch {
	case a.user != 0:
		return a.user == v.UserID
	case a.admins:
		return v.Admin
	}
	return true
}

// Event is one change, as a subscriber hears it.
type Event struct {
	ID   string // Opaque; a client sends the last one back to resume.
	Kind Kind
	Data []byte // JSON.
}

// published is an event as the hub keeps it.
type published struct {
	seq       uint64
	at        time.Time
	kind      Kind
	to        Audience
	data      []byte
	adminData []byte // What an administrator hears instead, or nil.
}

// Options configures [New]. The zero value is the server's.
type Options struct {
	Keep  time.Duration    // Zero means 5 minutes.
	Max   int              // Zero means 10,000 events.
	Queue int              // Zero means 256 per subscriber.
	Now   func() time.Time // Nil means time.Now.
}

// Hub is the server's events. It is safe for concurrent use.
type Hub struct {
	epoch string // Tells this run's ids from another's.
	opts  Options

	mu      sync.Mutex
	seq     uint64      // The last event's.
	ring    []published // The kept events, oldest first, from head.
	head    int
	n       int
	dropped uint64 // The newest seq no longer kept.
	subs    map[*Subscription]struct{}
}

// New returns a hub with no events.
func New(opts Options) *Hub {
	if opts.Keep <= 0 {
		opts.Keep = defaultKeep
	}
	if opts.Max <= 0 {
		opts.Max = defaultMax
	}
	if opts.Queue <= 0 {
		opts.Queue = defaultQueue
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Hub{
		epoch: strings.ToLower(rand.Text()[:8]), opts: opts,
		ring: make([]published, opts.Max), subs: map[*Subscription]struct{}{},
	}
}

// Publish sends data, JSON, to everyone in to who is subscribed, and keeps it
// for those who come back. Administrators hear adminData instead when it isn't
// nil. It never waits for a subscriber.
func (h *Hub) Publish(kind Kind, to Audience, data, adminData []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	p := published{seq: h.seq, at: h.opts.Now(), kind: kind, to: to, data: data, adminData: adminData}
	h.trim(p.at)
	if h.n == len(h.ring) {
		h.dropOldest()
	}
	h.ring[(h.head+h.n)%len(h.ring)] = p
	h.n++
	for s := range h.subs {
		if ev, ok := h.eventFor(&p, s.viewer); ok {
			s.offer(ev, h)
		}
	}
}

// Subscription is one client's stream. Read C until Done is closed.
type Subscription struct {
	viewer Viewer
	c      chan Event
	done   chan struct{}
	once   sync.Once
}

// C delivers the subscriber's events in order.
func (s *Subscription) C() <-chan Event { return s.c }

// Done is closed when the hub dropped the subscriber for falling behind, or
// it unsubscribed.
func (s *Subscription) Done() <-chan struct{} { return s.done }

// offer queues ev, dropping the subscriber when its queue is full. The caller
// holds the hub's mu.
func (s *Subscription) offer(ev Event, h *Hub) {
	select {
	case s.c <- ev:
	default:
		delete(h.subs, s)
		s.close()
	}
}

func (s *Subscription) close() { s.once.Do(func() { close(s.done) }) }

// Subscribe starts a stream for v. The first event is [Ready] with the
// id to resume from, then, when lastID names an event the hub still has, what
// v missed since it, or else [Resync], which also carries the id to resume
// from. An empty lastID is a fresh start: Ready alone. Call [Hub.Unsubscribe]
// when done.
func (h *Hub) Subscribe(v Viewer, lastID string) *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trim(h.opts.Now())
	backlog, ok := h.since(v, lastID)
	s := &Subscription{viewer: v, c: make(chan Event, h.opts.Queue+len(backlog)+1), done: make(chan struct{})}
	switch {
	case !ok:
		s.c <- Event{ID: h.id(h.seq), Kind: Resync, Data: []byte("{}")}
	default:
		s.c <- Event{ID: h.id(h.seq), Kind: Ready, Data: []byte("{}")}
	}
	for _, ev := range backlog {
		s.c <- ev
	}
	h.subs[s] = struct{}{}
	return s
}

// Unsubscribe ends s.
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, s)
	s.close()
}

// Subscribers counts the streams open now.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// trim drops the events older than Keep. The caller holds mu.
func (h *Hub) trim(now time.Time) {
	for h.n > 0 && now.Sub(h.ring[h.head].at) > h.opts.Keep {
		h.dropOldest()
	}
}

// dropOldest drops the oldest kept event. The caller holds mu.
func (h *Hub) dropOldest() {
	h.dropped = h.ring[h.head].seq
	h.ring[h.head] = published{}
	h.head = (h.head + 1) % len(h.ring)
	h.n--
}

// id is the wire id of seq.
func (h *Hub) id(seq uint64) string { return h.epoch + "-" + strconv.FormatUint(seq, 10) }

// eventFor returns p as v hears it, and whether v hears it at all.
func (h *Hub) eventFor(p *published, v Viewer) (Event, bool) {
	if !p.to.includes(v) {
		return Event{}, false
	}
	data := p.data
	if v.Admin && p.adminData != nil {
		data = p.adminData
	}
	return Event{ID: h.id(p.seq), Kind: p.kind, Data: data}, true
}

// since returns what v missed after lastID, and false when the hub can't tell:
// the id is from another run, from the future, or older than what it kept.
// The caller holds mu.
func (h *Hub) since(v Viewer, lastID string) ([]Event, bool) {
	if lastID == "" {
		return nil, true
	}
	epoch, num, found := strings.Cut(lastID, "-")
	seq, err := strconv.ParseUint(num, 10, 64)
	if !found || err != nil || epoch != h.epoch || seq > h.seq || seq < h.dropped {
		return nil, false
	}
	var out []Event
	for i := range h.n {
		p := &h.ring[(h.head+i)%len(h.ring)]
		if p.seq <= seq {
			continue
		}
		if ev, ok := h.eventFor(p, v); ok {
			out = append(out, ev)
		}
	}
	return out, true
}

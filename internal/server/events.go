package server

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/settings"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// heartbeatEvery is how often an idle stream sends a comment, so a proxy
	// never closes it for silence.
	heartbeatEvery = 25 * time.Second
	// eventWriteTimeout bounds one write to a stream: a client that takes no
	// more is cut off rather than holding a goroutine forever.
	eventWriteTimeout = 30 * time.Second
)

// emptyEvent is the data of an event that says only that something changed.
var emptyEvent = []byte("{}") //nolint:gochecknoglobals // A constant the hub only reads.

// getEvents streams every event the caller may hear, until the caller goes,
// the server stops, or the hub drops the caller for falling behind.
func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	sub := s.events.Subscribe(events.Viewer{UserID: user.ID, Admin: user.Role == store.RoleAdmin}, r.Header.Get("Last-Event-ID"))
	defer s.events.Unsubscribe(sub)
	h := w.Header()
	h.Set("Content-Type", api.EventStreamMediaType)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	heartbeat := time.NewTicker(heartbeatEvery)
	defer heartbeat.Stop()
	for {
		var frame string
		select {
		case <-r.Context().Done():
			return
		case <-s.editContext.Done(): // The server is stopping: let the client come back to the next one.
			return
		case <-sub.Done():
			return
		case ev := <-sub.C():
			frame = eventFrame(&ev)
		case <-heartbeat.C:
			frame = ": ping\n\n"
		}
		if !s.sendFrame(w, rc, frame) {
			return
		}
	}
}

// sendFrame writes one frame and flushes it, within eventWriteTimeout. It
// reports whether the stream may go on.
func (*Server) sendFrame(w http.ResponseWriter, rc *http.ResponseController, frame string) bool {
	_ = rc.SetWriteDeadline(time.Now().Add(eventWriteTimeout)) // Without a deadline to set, a write simply waits.
	if _, err := io.WriteString(w, frame); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// eventFrame is ev as server-sent event lines. Its data is one line of JSON,
// which never holds a newline.
func eventFrame(ev *events.Event) string {
	var b strings.Builder
	b.WriteString("id: ")
	b.WriteString(ev.ID)
	b.WriteString("\nevent: ")
	b.WriteString(string(ev.Kind))
	b.WriteString("\ndata: ")
	b.Write(ev.Data)
	b.WriteString("\n\n")
	return b.String()
}

// publish marshals v and sends it as an event of kind to to. A value that
// can't be marshaled is a programming error, logged and dropped.
func (s *Server) publish(kind events.Kind, to events.Audience, v any) {
	publishJSON(s.events, kind, to, v, nil)
}

// publishJSON marshals v, and admin when it isn't nil, and publishes them.
func publishJSON(hub *events.Hub, kind events.Kind, to events.Audience, v, admin any) {
	data, err := json.Marshal(v)
	if err != nil {
		return // Wire types always marshal.
	}
	var adminData []byte
	if admin != nil {
		if adminData, err = json.Marshal(admin); err != nil {
			return
		}
	}
	hub.Publish(kind, to, data, adminData)
}

// publishMarks tells the person's devices a heart or a rating changed.
func (s *Server) publishMarks(userID int64, change *api.EventMarks) {
	s.publish(events.Marks, events.User(userID), change)
}

// publishLastfm tells the person's devices their Last.fm connection changed.
func (s *Server) publishLastfm(userID int64) { LastfmChanged(s.events)(userID) }

// LibraryEvents turns what the scanner changes into events: it is the
// scanner's [library.Notifier].
type LibraryEvents struct {
	hub *events.Hub
}

// NewLibraryEvents returns the scanner's notifier over hub.
func NewLibraryEvents(hub *events.Hub) *LibraryEvents { return &LibraryEvents{hub: hub} }

// LibraryChanged implements [library.Notifier]: everyone hears which albums.
func (e *LibraryEvents) LibraryChanged(albums []string, all bool) {
	publishJSON(e.hub, events.Library, events.Everyone(), &api.EventLibrary{Albums: append([]string{}, albums...), All: all}, nil)
}

// ScanChanged implements [library.Notifier]: everyone hears a scan's progress,
// as GET /library carries it, and only administrators why it failed.
func (e *LibraryEvents) ScanChanged(p library.Progress) {
	everyone, admins := toAPIScan(p, false), toAPIScan(p, true)
	publishJSON(e.hub, events.Scan, events.Everyone(), &everyone, &admins)
}

// FingerprintsChanged implements [library.Notifier]: administrators hear the
// pass's progress.
func (e *LibraryEvents) FingerprintsChanged(p library.FingerprintProgress) {
	status := toAPIFingerprints(p)
	publishJSON(e.hub, events.Fingerprints, events.Admins(), &status, nil)
}

// LastfmChanged returns what the scrobbler calls when a person's connection or
// outbox changed: that person's devices hear it.
func LastfmChanged(hub *events.Hub) func(userID int64) {
	return func(userID int64) { hub.Publish(events.Lastfm, events.User(userID), emptyEvent, nil) }
}

// ShuffleRulesChanged returns what the shuffle rules' routes call once a
// person's runs or exclusions changed: that person's devices hear it. It is the
// hook for those routes, which plan 039 adds; nothing calls it before them.
func ShuffleRulesChanged(hub *events.Hub) func(userID int64) {
	return func(userID int64) { hub.Publish(events.ShuffleRules, events.User(userID), emptyEvent, nil) }
}

// FoldersChanged returns what the watcher calls when a folder's watch changed:
// administrators hear it as a change to library.roots, which their Server page
// shows it under.
func FoldersChanged(hub *events.Hub) func() {
	return func() {
		publishJSON(hub, events.Settings, events.Admins(), &api.EventSettings{Keys: []string{string(settings.LibraryRoots)}}, nil)
	}
}

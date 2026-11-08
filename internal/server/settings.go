package server

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"maps"
	"mime"
	"net/http"
	"slices"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/settings"
)

// maxSettingsBody caps a PATCH /admin/settings body: every key at its largest
// fits well inside it.
const maxSettingsBody = 512 << 10

// getSettings answers every server setting, a secret only as whether it is set.
func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	body := s.apiSettings(s.settings.Snapshot())
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// patchSettings changes the settings the body names, all of them or none. The
// log line names the keys and never a value.
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	values, err := readSettings(w, r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON object of settings by key")
		return
	}
	snap, err := s.settings.Change(r.Context(), values)
	if ke, ok := errors.AsType[*settings.KeyError](err); ok {
		writeProblem(w, r, http.StatusBadRequest, ke.Error()) // It names the key and why, never the value.
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "change settings", err)
		}
		return
	}
	s.log.InfoContext(r.Context(), "settings changed",
		"requestId", requestID(r.Context()), "keys", slices.Sorted(maps.Keys(values)))
	w.Header().Set("Cache-Control", "no-store")
	body := s.apiSettings(snap)
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// readSettings reads a JSON object of values by key.
func readSettings(w http.ResponseWriter, r *http.Request) (map[string][]byte, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return nil, errors.ErrUnsupported
	}
	var body map[string]jsontext.Value
	if readErr := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxSettingsBody), &body); readErr != nil {
		return nil, readErr //nolint:wrapcheck // The caller only tells the client the body is bad.
	}
	if body == nil { // A JSON null.
		return nil, errors.ErrUnsupported
	}
	values := make(map[string][]byte, len(body))
	for k, v := range body {
		values[k] = v
	}
	return values, nil
}

// apiSettings builds the API's view of snap field by field. A secret's value
// never goes into it.
func (s *Server) apiSettings(snap *settings.Snapshot) api.ServerSettings {
	out := api.ServerSettings{
		Settings: make(map[string]api.Setting, len(settings.Keys())),
		Listen:   s.listen, ListenSource: api.SettingDefault, Version: s.version,
	}
	if s.listenFile {
		out.ListenSource = api.SettingFile
	}
	for _, k := range settings.Keys() {
		src := snap.Source(k)
		entry := api.Setting{
			Source: string(src), From: src.From(k),
			RestartRequired: k.RestartRequired(), PendingRestart: s.settings.PendingRestart(k),
		}
		if k.Secret() {
			set := src != settings.SourceDefault // An empty secret is never stored.
			entry.Set = &set
		} else {
			entry.Value = snap.JSON(k)
		}
		out.Settings[string(k)] = entry
	}
	return out
}

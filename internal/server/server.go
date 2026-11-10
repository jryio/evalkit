// Package server serves the review UI and its JSON API for one suite.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jryio/evalkit/internal/feedback"
	"github.com/jryio/evalkit/internal/suite"
)

// Server serves one suite file. The suite is re-read on every request, so agent
// edits show up without a restart; web assets come from web (embedded or a dir).
type Server struct {
	suitePath string
	fb        *feedback.Dir
	web       fs.FS
}

func New(suitePath string, fb *feedback.Dir, web fs.FS) *Server {
	return &Server{suitePath: suitePath, fb: fb, web: web}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static := http.FileServerFS(s.web)
	mux.Handle("GET /", static)
	mux.Handle("GET /files/", http.StripPrefix("/files/", http.FileServer(http.Dir(filepath.Dir(s.suitePath)))))
	mux.HandleFunc("GET /api/suite", s.getSuite)
	mux.HandleFunc("GET /api/version", s.getVersion)
	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("POST /api/state", s.postState)
	mux.HandleFunc("POST /api/submit", s.postSubmit)
	return noStore(guard(mux))
}

// guard stops other web pages from reaching the loopback server: a foreign Host
// means DNS rebinding, and a POST must be JSON from this page's own origin, which
// a cross-site form or no-cors fetch cannot send.
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopback(r.Host) {
			http.Error(w, "host must be loopback", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
				http.Error(w, "cross-origin request", http.StatusForbidden)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

func loopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// noStore stops the browser from showing a stale page after the suite or kit changes.
// It sets the header at WriteHeader time because http.FileServer clears
// Cache-Control when it serves an error.
func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(noStoreWriter{w}, r)
	})
}

type noStoreWriter struct{ http.ResponseWriter }

func (w noStoreWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(code)
}

func (w noStoreWriter) Write(b []byte) (int, error) {
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	return w.ResponseWriter.Write(b)
}

func (s *Server) load() (suite.Result, error) {
	res, err := suite.Load(s.suitePath)
	res.Version += s.fb.DoneVersion()
	return res, err
}

// getSuite returns the validated suite plus the action results the agent recorded
// with `evalkit done`; the UI merges those into the items.
func (s *Server) getSuite(w http.ResponseWriter, r *http.Request) {
	res, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	done, err := s.fb.DoneAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		suite.Result
		Done map[string]map[string]feedback.Done `json:"done"`
	}{res, done})
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	res, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": res.Version, "sequence": s.fb.Latest()})
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
	b, err := s.fb.State()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (s *Server) postState(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rev, ok := s.saveState(w, b); ok {
		writeJSON(w, http.StatusOK, map[string]int{"rev": rev})
	}
}

// postSubmit saves the state and writes the next snapshot. A stale state writes
// no snapshot. The server keeps running: a send never ends the session.
func (s *Server) postSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State    json.RawMessage `json:"state"`
		Snapshot map[string]any  `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Snapshot == nil {
		http.Error(w, "want {state, snapshot}", http.StatusBadRequest)
		return
	}
	rev, ok := s.saveState(w, body.State)
	if !ok {
		return
	}
	snap, err := s.fb.Submit(body.Snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		feedback.Snapshot
		Rev int `json:"rev"`
	}{snap, rev})
}

// saveState stores the posted state and returns its new revision. On failure it
// writes the response and returns false: 409 with the stored revision when
// another tab saved first, 400 otherwise.
func (s *Server) saveState(w http.ResponseWriter, b []byte) (int, bool) {
	rev, err := s.fb.SaveState(b)
	switch {
	case errors.Is(err, feedback.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]int{"rev": rev})
		return 0, false
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return 0, false
	}
	return rev, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

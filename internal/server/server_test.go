package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jryio/evalkit/internal/feedback"
	"github.com/jryio/evalkit/internal/server"
)

const (
	kitchenSink = "../../examples/kitchen-sink"
	secret      = "outside the suite dir"
)

type testServer struct {
	*httptest.Server
	handler http.Handler
	fb      *feedback.Dir
	dir     string // the suite's directory
}

func copyFile(tb testing.TB, src, dst string) {
	tb.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		tb.Fatalf("read %s: %v", src, err)
	}
	writeFile(tb, dst, string(b))
}

func writeFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}

// startServer serves suite.yaml from a temp dir laid out as root/suite/suite.yaml,
// with root/secret.txt outside the suite dir.
func startServer(tb testing.TB, writeSuite func(dir string)) *testServer {
	tb.Helper()
	root := tb.TempDir()
	dir := filepath.Join(root, "suite")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte(secret), 0o644); err != nil {
		tb.Fatalf("write secret: %v", err)
	}
	writeSuite(dir)
	suitePath := filepath.Join(dir, "suite.yaml")
	fb, err := feedback.Open(feedback.ForSuite(suitePath))
	if err != nil {
		tb.Fatalf("feedback.Open: %v", err)
	}
	h := server.New(suitePath, fb, fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>kit</title>")}}).Handler()
	ts := httptest.NewServer(h)
	tb.Cleanup(ts.Close)
	return &testServer{Server: ts, handler: h, fb: fb, dir: dir}
}

func kitchenSinkServer(tb testing.TB) *testServer {
	tb.Helper()
	return startServer(tb, func(dir string) {
		copyFile(tb, filepath.Join(kitchenSink, "suite.yaml"), filepath.Join(dir, "suite.yaml"))
		copyFile(tb, filepath.Join(kitchenSink, "assets", "evalkit-starter.png"), filepath.Join(dir, "assets", "evalkit-starter.png"))
	})
}

func (s *testServer) do(tb testing.TB, method, path, body string) (*http.Response, []byte) {
	tb.Helper()
	req := s.request(tb, method, path, body)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	return s.send(tb, req)
}

func (s *testServer) request(tb testing.TB, method, path, body string) *http.Request {
	tb.Helper()
	req, err := http.NewRequestWithContext(tb.Context(), method, s.URL+path, strings.NewReader(body))
	if err != nil {
		tb.Fatalf("NewRequest %s %s: %v", method, path, err)
	}
	return req
}

// send performs req and checks that every response, errors included, is no-store.
func (s *testServer) send(tb testing.TB, req *http.Request) (*http.Response, []byte) {
	tb.Helper()
	resp, err := s.Client().Do(req)
	if err != nil {
		tb.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		tb.Fatalf("read %s %s: %v", req.Method, req.URL.Path, err)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		tb.Errorf("%s %s Cache-Control = %q, want no-store", req.Method, req.URL.Path, got)
	}
	return resp, b
}

func decode[T any](tb testing.TB, b []byte) T {
	tb.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		tb.Fatalf("decode %q: %v", b, err)
	}
	return v
}

func wantRev(tb testing.TB, body []byte, want int) {
	tb.Helper()
	if got := decode[map[string]int](tb, body); got["rev"] != want {
		tb.Errorf("response %s, want rev %d", body, want)
	}
}

func wantStatus(tb testing.TB, resp *http.Response, body []byte, want int) {
	tb.Helper()
	if resp.StatusCode != want {
		tb.Fatalf("%s %s status = %d, want %d: %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body)
	}
}

type suiteResponse struct {
	Suite struct {
		Items []map[string]any `json:"items"`
	} `json:"suite"`
	Problems []map[string]any `json:"problems"`
	Version  string           `json:"version"`
	Fatal    bool             `json:"fatal"`
}

func TestGetSuite(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "GET", "/api/suite", "")
	wantStatus(t, resp, body, http.StatusOK)
	got := decode[suiteResponse](t, body)
	if len(got.Suite.Items) == 0 {
		t.Errorf("suite has no items: %s", body)
	}
	if got.Version == "" {
		t.Error("version is empty")
	}
	if got.Fatal || len(got.Problems) != 0 {
		t.Errorf("fatal = %v, problems = %v, want a clean load", got.Fatal, got.Problems)
	}
}

func TestGetSuite_BrokenItem(t *testing.T) {
	t.Parallel()
	s := startServer(t, func(dir string) {
		src := "title: Broken item\nitems:\n  - id: bad\n    title: Bad\n    show:\n      - kind: chartt\n  - id: good\n    title: Good\n"
		if err := os.WriteFile(filepath.Join(dir, "suite.yaml"), []byte(src), 0o644); err != nil {
			t.Fatalf("write suite: %v", err)
		}
	})
	resp, body := s.do(t, "GET", "/api/suite", "")
	wantStatus(t, resp, body, http.StatusOK)
	got := decode[suiteResponse](t, body)
	if len(got.Problems) != 1 || got.Problems[0]["item"] != "bad" {
		t.Errorf("problems = %v, want one for item bad", got.Problems)
	}
	if len(got.Suite.Items) != 1 || got.Suite.Items[0]["id"] != "good" {
		t.Errorf("items = %v, want only good", got.Suite.Items)
	}
}

func TestGetVersion_MatchesSuite(t *testing.T) {
	t.Parallel()
	s := startServer(t, func(dir string) {
		writeFile(t, filepath.Join(dir, "suite.yaml"), "title: Runs\nitems:\n  - { id: a, title: A }\nitems_from: runs/*.jsonl\n")
		writeFile(t, filepath.Join(dir, "runs", "1.jsonl"), `{"id":"b","title":"B"}`+"\n")
	})
	if err := s.fb.RecordDone("a", "post", feedback.Done{Note: "posted"}); err != nil {
		t.Fatalf("RecordDone: %v", err)
	}
	resp, body := s.do(t, "GET", "/api/suite", "")
	wantStatus(t, resp, body, http.StatusOK)
	got := decode[suiteResponse](t, body)
	if len(got.Suite.Items) != 2 {
		t.Fatalf("items = %v, want a and b", got.Suite.Items)
	}
	resp, body = s.do(t, "GET", "/api/version", "")
	wantStatus(t, resp, body, http.StatusOK)
	if v := decode[map[string]any](t, body)["version"]; v != got.Version {
		t.Errorf("/api/version version = %v, want the suite's %q", v, got.Version)
	}
}

func TestState_RoundTrip(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "GET", "/api/state", "")
	wantStatus(t, resp, body, http.StatusOK)
	if strings.TrimSpace(string(body)) != "{}" {
		t.Errorf("GET /api/state = %q, want {}", body)
	}
	const state = `{"items":{"text":{"revision":"abc","answers":{"verdict":"right"}}}}`
	resp, body = s.do(t, "POST", "/api/state", state)
	wantStatus(t, resp, body, http.StatusOK)
	wantRev(t, body, 1)
	resp, body = s.do(t, "GET", "/api/state", "")
	wantStatus(t, resp, body, http.StatusOK)
	const stored = `{"items":{"text":{"revision":"abc","answers":{"verdict":"right"}}},"rev":1}`
	if got, want := decode[map[string]any](t, body), decode[map[string]any](t, []byte(stored)); !equalJSON(got, want) {
		t.Errorf("GET /api/state = %s, want %s", body, stored)
	}
}

func TestState_Revisions(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "POST", "/api/state", `{"n":1}`)
	wantStatus(t, resp, body, http.StatusOK)
	wantRev(t, body, 1)
	resp, body = s.do(t, "POST", "/api/state", `{"rev":1,"n":2}`)
	wantStatus(t, resp, body, http.StatusOK)
	wantRev(t, body, 2)

	resp, body = s.do(t, "POST", "/api/state", `{"rev":1,"n":"stale tab"}`)
	wantStatus(t, resp, body, http.StatusConflict)
	wantRev(t, body, 2)
	resp, body = s.do(t, "GET", "/api/state", "")
	wantStatus(t, resp, body, http.StatusOK)
	if got := decode[map[string]any](t, body); got["n"] != float64(2) {
		t.Errorf("GET /api/state after a conflict = %s, want n=2 kept", body)
	}
}

func TestState_RejectsNonObject(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "POST", "/api/state", `[1]`)
	wantStatus(t, resp, body, http.StatusBadRequest)
}

func TestSubmit(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "POST", "/api/submit", `{"state":{"items":{}},"snapshot":{"suite":"Kitchen sink","items":{}}}`)
	wantStatus(t, resp, body, http.StatusOK)
	got := decode[struct {
		feedback.Snapshot
		Rev int `json:"rev"`
	}](t, body)
	if got.Sequence != 1 || got.Rev != 1 {
		t.Errorf("POST /api/submit = %s, want sequence 1 and rev 1", body)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Errorf("snapshot file: %v", err)
	}
	resp, body = s.do(t, "GET", "/api/suite", "")
	wantStatus(t, resp, body, http.StatusOK)
	resp, body = s.do(t, "GET", "/api/state", "")
	wantStatus(t, resp, body, http.StatusOK)
	if strings.TrimSpace(string(body)) == "{}" {
		t.Errorf("GET /api/state after submit = %q, want the submitted state", body)
	}
}

func TestSubmit_ConflictWritesNoSnapshot(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "POST", "/api/state", `{"items":{}}`)
	wantStatus(t, resp, body, http.StatusOK)
	resp, body = s.do(t, "POST", "/api/submit", `{"state":{"rev":0,"items":{}},"snapshot":{"items":{}}}`)
	wantStatus(t, resp, body, http.StatusConflict)
	wantRev(t, body, 1)
	if n := s.fb.Latest(); n != 0 {
		t.Errorf("Latest() after a conflicting submit = %d, want 0 (no snapshot)", n)
	}
}

func TestSubmit_RejectsMissingSnapshot(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	resp, body := s.do(t, "POST", "/api/submit", `{"state":{}}`)
	wantStatus(t, resp, body, http.StatusBadRequest)
}

func TestFiles_ServesSuiteDir(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	want, err := os.ReadFile(filepath.Join(s.dir, "assets", "evalkit-starter.png"))
	if err != nil {
		t.Fatalf("read asset: %v", err)
	}
	resp, body := s.do(t, "GET", "/files/assets/evalkit-starter.png", "")
	wantStatus(t, resp, body, http.StatusOK)
	if !bytes.Equal(body, want) {
		t.Errorf("body is %d bytes, want the %d-byte asset", len(body), len(want))
	}
}

func TestFiles_NoTraversal(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	for _, target := range []string{
		"/files/../secret.txt",
		"/files/..%2fsecret.txt",
		"/files/%2e%2e/secret.txt",
		"/files/%2e%2e%2fsecret.txt",
		"/files//../secret.txt",
		"/files/assets/../../secret.txt",
	} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", target, nil)
			req.Host = "127.0.0.1:8790"
			s.handler.ServeHTTP(rec, req)
			if rec.Code == http.StatusForbidden {
				t.Fatalf("GET %s = 403 from the request guard; the traversal check never ran", target)
			}
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("GET %s served a file outside the suite dir (status %d)", target, rec.Code)
			}
		})
	}
}

// TestNoStore covers responses not checked elsewhere; do checks Cache-Control on every request.
func TestNoStore(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	tests := []struct {
		name, path, bug string
	}{
		{name: "index", path: "/"},
		{name: "index redirect", path: "/index.html"},
		{name: "api", path: "/api/version"},
		{name: "suite file", path: "/files/suite.yaml"},
		{name: "missing suite file", path: "/files/missing.txt"},
		{name: "missing web file", path: "/missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.bug != "" {
				t.Skip("BUG: " + tc.bug)
			}
			s.do(t, "GET", tc.path, "")
		})
	}
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func TestGetSuite_IncludesRecordedDone(t *testing.T) {
	t.Parallel()
	s := kitchenSinkServer(t)
	if err := s.fb.RecordDone("controls", "merge", feedback.Done{URL: "https://github.com/x/pull/1"}); err != nil {
		t.Fatalf("RecordDone: %v", err)
	}
	_, body := s.do(t, "GET", "/api/suite", "")
	var got struct {
		Done map[string]map[string]feedback.Done `json:"done"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if url := got.Done["controls"]["merge"].URL; url != "https://github.com/x/pull/1" {
		t.Errorf("done.controls.merge.url = %q, want the recorded URL", url)
	}
}

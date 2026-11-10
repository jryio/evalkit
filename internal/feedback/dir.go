// Package feedback owns the feedback directory: review state, numbered snapshots
// of each send, action results and the bookkeeping for waiters.
package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	stateFile = "state.json"
	doneDir   = "done"
	seenFile  = "seen"
)

var (
	snapshotName = regexp.MustCompile(`^sent-(\d+)\.json$`)
	doneName     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ErrConflict means the posted state was based on an older revision than the
// stored one, e.g. because another tab saved in between.
var ErrConflict = errors.New("state changed in another tab")

// Dir is a feedback directory. Its methods are safe for concurrent use.
type Dir struct {
	path string
	mu   sync.Mutex
}

// Open creates the directory if needed.
func Open(path string) (*Dir, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	return &Dir{path: path}, nil
}

// ForSuite returns the default feedback directory: feedback/ next to the suite file.
func ForSuite(suitePath string) string {
	return filepath.Join(filepath.Dir(suitePath), "feedback")
}

func (d *Dir) Path() string { return d.path }

func (d *Dir) file(name string) string { return filepath.Join(d.path, name) }

// State returns the saved review state, or {} when nothing is saved yet.
func (d *Dir) State() ([]byte, error) {
	b, err := os.ReadFile(d.file(stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return []byte("{}"), nil
	}
	return b, err
}

// SaveState stores the review state, a JSON object whose "rev" field names the
// revision it was based on (missing: 0). It returns the new revision. When rev
// is not the stored revision it writes nothing and returns the stored revision
// with ErrConflict.
func (d *Dir) SaveState(b []byte) (int, error) {
	obj, posted, err := decodeState(b)
	if err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	stored, err := d.storedRev()
	if err != nil {
		return 0, err
	}
	if posted != stored {
		return stored, ErrConflict
	}
	next := stored + 1
	obj["rev"] = json.RawMessage(strconv.Itoa(next))
	if err := writeJSON(d.file(stateFile), obj); err != nil {
		return 0, err
	}
	return next, nil
}

func (d *Dir) storedRev() (int, error) {
	b, err := os.ReadFile(d.file(stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, rev, err := decodeState(b)
	if err != nil {
		return 0, fmt.Errorf("stored %s: %w", stateFile, err)
	}
	return rev, nil
}

// decodeState keeps every value as raw JSON so a save round-trips it unchanged.
func decodeState(b []byte) (map[string]json.RawMessage, int, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil || obj == nil {
		return nil, 0, fmt.Errorf("state must be a JSON object, got %.40q", b)
	}
	raw, ok := obj["rev"]
	if !ok {
		return obj, 0, nil
	}
	rev, err := strconv.Atoi(string(raw))
	if err != nil || rev < 0 {
		return nil, 0, fmt.Errorf("state rev must be a non-negative integer, got %s", raw)
	}
	return obj, rev, nil
}

// Snapshot identifies one send.
type Snapshot struct {
	Sequence int    `json:"sequence"`
	Path     string `json:"path"`
}

// Submit writes the next numbered snapshot.
func (d *Dir) Submit(snap map[string]any) (Snapshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	seq := d.latest() + 1
	snap["sequence"] = seq
	snap["sent_at"] = time.Now().UTC().Format(time.RFC3339)
	path := d.SnapshotPath(seq)
	if err := writeJSON(path, snap); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Sequence: seq, Path: path}, nil
}

func (d *Dir) SnapshotPath(seq int) string {
	return d.file(fmt.Sprintf("sent-%02d.json", seq))
}

// ReadSnapshot decodes snapshot seq.
func (d *Dir) ReadSnapshot(seq int) (map[string]any, error) {
	b, err := os.ReadFile(d.SnapshotPath(seq))
	if err != nil {
		return nil, err
	}
	var snap map[string]any
	return snap, json.Unmarshal(b, &snap)
}

// Latest is the newest snapshot number, 0 when nothing was sent.
func (d *Dir) Latest() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.latest()
}

func (d *Dir) latest() int {
	entries, _ := os.ReadDir(d.path)
	n := 0
	for _, e := range entries {
		if m := snapshotName.FindStringSubmatch(e.Name()); m != nil {
			n = max(n, atoi(m[1]))
		}
	}
	return n
}

// Poll blocks until a snapshot newer than after appears on disk. It works across
// processes and server restarts, which is what an agent's waiter needs.
func (d *Dir) Poll(ctx context.Context, after int, every time.Duration) (int, error) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n := d.Latest(); n > after {
			return n, nil
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// Seen is the last snapshot an agent's waiter reported.
func (d *Dir) Seen() int {
	b, _ := os.ReadFile(d.file(seenFile))
	return atoi(strings.TrimSpace(string(b)))
}

func (d *Dir) SetSeen(seq int) error {
	return os.WriteFile(d.file(seenFile), []byte(strconv.Itoa(seq)+"\n"), 0o644)
}

// Done is the result of an action the agent performed after a send.
type Done struct {
	At   string `json:"at,omitempty"`
	URL  string `json:"url,omitempty"`
	Note string `json:"note,omitempty"`
}

// RecordDone stores an action result in done/<item>/<action>.json. Recording it
// again overwrites it.
func (d *Dir) RecordDone(item, action string, rec Done) error {
	for _, name := range []string{item, action} {
		if !doneName.MatchString(name) || strings.Contains(name, "..") {
			return fmt.Errorf("invalid id %q: want letters, digits, '_', '.' and '-', starting with a letter or digit", name)
		}
	}
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339)
	}
	dir := filepath.Join(d.path, doneDir, item)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, action+".json"), rec)
}

// DoneAll returns item id -> action id -> result.
func (d *Dir) DoneAll() (map[string]map[string]Done, error) {
	files, err := d.doneFiles()
	if err != nil {
		return nil, err
	}
	all := map[string]map[string]Done{}
	for _, f := range files {
		b, err := os.ReadFile(f.path)
		if err != nil {
			return nil, err
		}
		var rec Done
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
		if all[f.item] == nil {
			all[f.item] = map[string]Done{}
		}
		all[f.item][f.action] = rec
	}
	return all, nil
}

// DoneVersion changes whenever an action result is added or rewritten, so the UI
// can refresh. It is empty when nothing is recorded.
func (d *Dir) DoneVersion() string {
	files, err := d.doneFiles()
	if err != nil || len(files) == 0 {
		return ""
	}
	var newest time.Time
	for _, f := range files {
		if f.mod.After(newest) {
			newest = f.mod
		}
	}
	return strconv.Itoa(len(files)) + "-" + strconv.FormatInt(newest.UnixNano(), 36)
}

type doneFile struct {
	item, action, path string
	mod                time.Time
}

// doneFiles lists done/<item>/<action>.json, skipping in-flight temp files.
func (d *Dir) doneFiles() ([]doneFile, error) {
	root := d.file(doneDir)
	items, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []doneFile
	for _, it := range items {
		if !it.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, it.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			action, ok := strings.CutSuffix(e.Name(), ".json")
			if !ok || e.IsDir() || !doneName.MatchString(action) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			out = append(out, doneFile{
				item:   it.Name(),
				action: action,
				path:   filepath.Join(root, it.Name(), e.Name()),
				mod:    info.ModTime(),
			})
		}
	}
	return out, nil
}

// writeJSON writes through a unique temp file and a rename, so a reader never
// sees half a file and concurrent writers never share a temp file.
func writeJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

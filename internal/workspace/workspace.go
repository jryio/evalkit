// Package workspace gives each attempt of a benchmark its own copy of the code
// under test: a pool of slots, provisioned once and reset in place between attempts.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Reset policies: when a slot goes back to its provisioned state.
const (
	ResetAttempt = "attempt" // before every attempt
	ResetRun     = "run"     // once, when a run first uses the slot
	ResetNever   = "never"   // never: attempts build on each other
)

// Source is one directory copied into every slot.
type Source struct {
	Path    string   `json:"path"`    // relative to the suite, absolute, or ~/...
	As      string   `json:"as"`      // directory name in the slot; default: the path's base name
	Pin     string   `json:"pin"`     // git ref the slot starts at; default HEAD
	Method  string   `json:"method"`  // auto (default), clone or copy
	Include []string `json:"include"` // untracked paths a clone also brings along
}

// Config is the workspace block of a benchmark.
type Config struct {
	Sources   []Source `json:"sources"`
	Setup     string   `json:"setup"`     // shell command run once per slot after provisioning
	Keep      []string `json:"keep"`      // slot-relative paths a reset leaves alone
	Reset     string   `json:"reset"`     // attempt (default), run or never
	Provision string   `json:"provision"` // shell command that fills an empty slot instead of sources
	Wipe      string   `json:"wipe"`      // shell command that resets a slot instead of the built-in reset
}

// Pool hands out slots. Slots persist under root across runs and are reused
// while the workspace config and its pins are unchanged.
type Pool struct {
	cfg      Config
	suiteDir string
	root     string
	sources  []resolved
	hash     string
	free     chan int

	mu    sync.Mutex
	slots map[int]*Slot
	locks []*os.File // one per slot this process claimed, held until Close
}

// unclaimed stands in the free list for a slot not yet claimed from the root.
const unclaimed = -1

// resolved is a Source with its paths and method worked out.
type resolved struct {
	Source
	abs string
	git bool
	sha string // the pinned commit of a git source
}

// Slot is one isolated copy. Dir is its root; each source sits in Dir/<as>.
type Slot struct {
	Index int
	Dir   string
	pool  *Pool
	fresh bool              // provisioned or reset since its last attempt
	bases map[string]string // source name -> the commit it currently starts from
}

// NewPool resolves the sources and prepares a pool of n slots under root.
func NewPool(ctx context.Context, cfg Config, suiteDir, root string, n int) (*Pool, error) {
	switch cfg.Reset {
	case "":
		cfg.Reset = ResetAttempt
	case ResetAttempt, ResetRun, ResetNever:
	default:
		return nil, fmt.Errorf("workspace.reset %q: want attempt, run or never", cfg.Reset)
	}
	p := &Pool{cfg: cfg, suiteDir: suiteDir, root: root, free: make(chan int, n), slots: map[int]*Slot{}}
	names := map[string]bool{}
	for _, s := range cfg.Sources {
		r, err := resolve(ctx, s, suiteDir)
		if err != nil {
			return nil, err
		}
		if names[r.As] {
			return nil, fmt.Errorf("workspace: two sources are named %q; set as", r.As)
		}
		names[r.As] = true
		p.sources = append(p.sources, r)
	}
	b, err := json.Marshal(struct {
		Config  Config
		Sources []string
	}{cfg, p.fingerprints()})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	p.hash = hex.EncodeToString(sum[:8])
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	for range n {
		p.free <- unclaimed
	}
	return p, nil
}

// claim takes the lowest-numbered slot under the root that no process holds,
// this one included. Two runs of one suite at once get different slots.
func (p *Pool) claim() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < 10_000; i++ {
		if _, mine := p.slots[i]; mine {
			continue
		}
		if f, ok := tryLock(filepath.Join(p.root, strconv.Itoa(i)+".lock")); ok {
			p.locks = append(p.locks, f)
			p.slots[i] = nil
			return i, nil
		}
	}
	return 0, errors.New("no free workspace slot under " + p.root)
}

// Close gives the pool's slots back to other processes. The slots stay on disk.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, f := range p.locks {
		f.Close()
	}
	p.locks = nil
}

func (p *Pool) fingerprints() []string {
	var out []string
	for _, r := range p.sources {
		out = append(out, r.abs+"@"+r.sha)
	}
	return out
}

func resolve(ctx context.Context, s Source, suiteDir string) (resolved, error) {
	r := resolved{Source: s}
	path := s.Path
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return r, err
		}
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(suiteDir, path)
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return r, fmt.Errorf("workspace source %q: not a directory", s.Path)
	}
	r.abs = path
	if r.As == "" {
		r.As = filepath.Base(path)
	}
	isRepo := isGitRoot(ctx, path)
	switch s.Method {
	case "", "auto":
		r.git = isRepo
	case "clone":
		if !isRepo {
			return r, fmt.Errorf("workspace source %q: method clone needs the root of a git repository", s.Path)
		}
		r.git = true
	case "copy":
	default:
		return r, fmt.Errorf("workspace source %q: method %q: want auto, clone or copy", s.Path, s.Method)
	}
	if r.git {
		pin := s.Pin
		if pin == "" {
			pin = "HEAD"
		}
		if r.sha, err = revParse(ctx, path, pin); err != nil {
			return r, fmt.Errorf("workspace source %q: pin %q: %w", s.Path, pin, err)
		}
	}
	return r, nil
}

// Acquire blocks for a free slot and returns it ready for an attempt. pins
// overrides the commit of the named git sources for this attempt.
func (p *Pool) Acquire(ctx context.Context, pins map[string]string) (*Slot, error) {
	var i int
	select {
	case i = <-p.free:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if i == unclaimed {
		claimed, err := p.claim()
		if err != nil {
			p.free <- unclaimed
			return nil, err
		}
		i = claimed
	}
	s, err := p.ready(ctx, i, pins)
	if err != nil {
		p.free <- i
		return nil, err
	}
	return s, nil
}

// Release returns a slot to the pool after an attempt.
func (p *Pool) Release(s *Slot) {
	s.fresh = false
	p.free <- s.Index
}

func (p *Pool) ready(ctx context.Context, i int, pins map[string]string) (*Slot, error) {
	p.mu.Lock()
	s := p.slots[i]
	p.mu.Unlock()
	if s == nil {
		s = &Slot{Index: i, Dir: filepath.Join(p.root, strconv.Itoa(i)), pool: p}
		reused := s.loadState()
		if !reused {
			if err := s.provision(ctx); err != nil {
				return nil, fmt.Errorf("slot %d: %w", i, err)
			}
		} else if p.cfg.Reset != ResetNever {
			if err := s.Reset(ctx, nil); err != nil {
				return nil, fmt.Errorf("slot %d: %w", i, err)
			}
		}
		p.mu.Lock()
		p.slots[i] = s
		p.mu.Unlock()
	}
	wantPin := false
	for name, ref := range pins {
		r, ok := p.source(name)
		if !ok || !r.git {
			return nil, fmt.Errorf("pin for %q: no git source has that name", name)
		}
		sha, err := revParse(ctx, r.abs, ref)
		if err != nil {
			return nil, fmt.Errorf("pin %s=%q: %w", name, ref, err)
		}
		wantPin = wantPin || s.bases[name] != sha
		pins[name] = sha
	}
	if (p.cfg.Reset == ResetAttempt && !s.fresh) || wantPin {
		if err := s.Reset(ctx, pins); err != nil {
			return nil, fmt.Errorf("slot %d: %w", i, err)
		}
	}
	return s, nil
}

func (p *Pool) source(name string) (resolved, bool) {
	for _, r := range p.sources {
		if r.As == name {
			return r, true
		}
	}
	return resolved{}, false
}

// RemoveSlots deletes the slots under root that no running process holds, and
// reports how many it removed and how many are in use.
func RemoveSlots(root string) (removed, busy int, err error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		lock := filepath.Join(root, e.Name()+".lock")
		f, free := tryLock(lock)
		if !free {
			busy++
			continue
		}
		err = errors.Join(os.RemoveAll(filepath.Join(root, e.Name())), os.Remove(filepath.Join(root, e.Name()+".json")))
		f.Close()
		os.Remove(lock)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, busy, err
		}
		removed++
	}
	return removed, busy, nil
}

// Estimate is what provisioning one slot cost.
type Estimate struct {
	Provision time.Duration
	Reset     time.Duration
	SlotBytes int64
	FreeBytes int64 // -1 when the platform does not say
}

// Probe provisions one slot from nothing, dirties it, resets it and reports
// the cost, so a run can be sized before all its slots are made.
func (p *Pool) Probe(ctx context.Context) (Estimate, error) {
	var e Estimate
	i, err := p.claim()
	if err != nil {
		return e, err
	}
	s := &Slot{Index: i, Dir: filepath.Join(p.root, strconv.Itoa(i)), pool: p}
	start := time.Now()
	if err := s.provision(ctx); err != nil {
		return e, err
	}
	p.mu.Lock()
	p.slots[i] = s
	p.mu.Unlock()
	e.Provision = time.Since(start)
	e.SlotBytes = diskUsage(ctx, s.Dir)
	e.FreeBytes = freeBytes(p.root)
	if err := os.WriteFile(filepath.Join(s.Dir, ".evalkit-probe"), []byte("x"), 0o644); err != nil {
		return e, err
	}
	start = time.Now()
	if err := s.Reset(ctx, nil); err != nil {
		return e, err
	}
	e.Reset = time.Since(start)
	if _, err := os.Stat(filepath.Join(s.Dir, ".evalkit-probe")); err == nil {
		return e, errors.New("reset left a stray file in the slot; check workspace.keep and workspace.wipe")
	}
	return e, nil
}

// Path returns where the named source sits in the slot.
func (s *Slot) Path(name string) string { return filepath.Join(s.Dir, name) }

// Sources lists the slot's source names in config order.
func (s *Slot) Sources() []string {
	var out []string
	for _, r := range s.pool.sources {
		out = append(out, r.As)
	}
	return out
}

type slotState struct {
	Hash  string            `json:"hash"`
	Bases map[string]string `json:"bases"`
}

func (s *Slot) statePath() string { return s.Dir + ".json" }

// loadState reports whether the slot on disk was provisioned from this config.
func (s *Slot) loadState() bool {
	b, err := os.ReadFile(s.statePath())
	if err != nil {
		return false
	}
	var st slotState
	if json.Unmarshal(b, &st) != nil || st.Hash != s.pool.hash {
		return false
	}
	if _, err := os.Stat(s.Dir); err != nil {
		return false
	}
	s.bases = st.Bases
	return true
}

func (s *Slot) saveState() error {
	b, err := json.Marshal(slotState{Hash: s.pool.hash, Bases: s.bases})
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath(), b, 0o644)
}

func (s *Slot) provision(ctx context.Context) error {
	p := s.pool
	os.Remove(s.statePath())
	if err := os.RemoveAll(s.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	s.bases = map[string]string{}
	if p.cfg.Provision != "" {
		if err := s.shell(ctx, p.cfg.Provision); err != nil {
			return fmt.Errorf("provision: %w", err)
		}
	}
	for _, r := range p.sources {
		dst := s.Path(r.As)
		if !r.git {
			if err := copyTree(ctx, r.abs, dst); err != nil {
				return fmt.Errorf("copy %s: %w", r.Path, err)
			}
			continue
		}
		if err := cloneAt(ctx, r.abs, dst, r.sha); err != nil {
			return fmt.Errorf("clone %s: %w", r.Path, err)
		}
		s.bases[r.As] = r.sha
		for _, inc := range r.Include {
			from := filepath.Join(r.abs, inc)
			if _, err := os.Stat(from); err != nil {
				return fmt.Errorf("include %s: %w", inc, err)
			}
			to := filepath.Join(dst, inc)
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			if err := copyTree(ctx, from, to); err != nil {
				return fmt.Errorf("include %s: %w", inc, err)
			}
		}
	}
	if p.cfg.Setup != "" {
		if err := s.shell(ctx, p.cfg.Setup); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
	}
	s.fresh = true
	return s.saveState()
}

// Reset returns the slot to its provisioned state, keeping the keep paths.
// pins moves the named git sources to another commit.
func (s *Slot) Reset(ctx context.Context, pins map[string]string) error {
	p := s.pool
	if p.cfg.Wipe != "" {
		if err := s.shell(ctx, p.cfg.Wipe); err != nil {
			return fmt.Errorf("wipe: %w", err)
		}
		s.fresh = true
		return nil
	}
	if p.cfg.Provision != "" {
		return s.provision(ctx)
	}
	known := map[string]bool{}
	for _, r := range p.sources {
		known[r.As] = true
		dst := s.Path(r.As)
		keep := s.keepUnder(r)
		if !r.git {
			if err := syncTree(ctx, r.abs, dst, keep); err != nil {
				return fmt.Errorf("reset %s: %w", r.As, err)
			}
			continue
		}
		sha := r.sha
		if pin, ok := pins[r.As]; ok {
			sha = pin
		}
		if err := resetClone(ctx, dst, sha, keep); err != nil {
			return fmt.Errorf("reset %s: %w", r.As, err)
		}
		s.bases[r.As] = sha
	}
	if err := s.removeStrays(known); err != nil {
		return err
	}
	s.fresh = true
	return s.saveState()
}

// keepUnder returns the keep paths and includes that sit inside source r,
// relative to it.
func (s *Slot) keepUnder(r resolved) []string {
	out := append([]string(nil), r.Include...)
	for _, k := range s.pool.cfg.Keep {
		if rest, ok := strings.CutPrefix(filepath.ToSlash(k), r.As+"/"); ok {
			out = append(out, rest)
		}
	}
	return out
}

// removeStrays deletes what an attempt left at the slot's root beside the sources.
func (s *Slot) removeStrays(known map[string]bool) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	kept := map[string]bool{}
	for _, k := range s.pool.cfg.Keep {
		kept[strings.SplitN(filepath.ToSlash(k), "/", 2)[0]] = true
	}
	for _, e := range entries {
		if known[e.Name()] || kept[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.Dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Diff returns each git source's changes since the commit it started from,
// untracked files included, as unified diffs keyed by source name. It leaves
// out the keep paths and the slot-relative exclude paths.
func (s *Slot) Diff(ctx context.Context, exclude ...string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range s.pool.sources {
		if !r.git {
			continue
		}
		skip := s.keepUnder(r)
		for _, e := range exclude {
			if rest, ok := strings.CutPrefix(filepath.ToSlash(e), r.As+"/"); ok {
				skip = append(skip, rest)
			}
		}
		d, err := diffSince(ctx, s.Path(r.As), s.bases[r.As], skip)
		if err != nil {
			return nil, fmt.Errorf("diff %s: %w", r.As, err)
		}
		out[r.As] = d
	}
	return out, nil
}

// shell runs a config command in the slot.
func (s *Slot) shell(ctx context.Context, command string) error {
	env := append(os.Environ(), "EVALKIT_SLOT="+s.Dir, "EVALKIT_SUITE_DIR="+s.pool.suiteDir)
	_, err := run(ctx, s.Dir, env, "sh", "-c", command)
	return err
}

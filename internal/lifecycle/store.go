// Package lifecycle tracks per-client metadata that AmneziaWG itself does not:
// traffic quotas, expiry dates, and accumulated usage. It is the engine behind
// "give this client 5 days / 50 GB, then cut it off automatically".
//
// The store is the source of truth for lifecycle state; the WireGuard config
// (awg0.conf) remains the source of truth for live peers. The enforcer
// reconciles the two.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Record is the lifecycle metadata for one client.
type Record struct {
	Name       string     `json:"name"`
	PubKey     string     `json:"pub_key"`
	Octet      int        `json:"octet"`
	PeerBlock  string     `json:"peer_block"` // server-conf [Peer] block, for re-enable
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"` // nil = never
	QuotaBytes uint64     `json:"quota_bytes"`          // 0 = unlimited
	UsedBytes  uint64     `json:"used_bytes"`
	SpeedMbit  int        `json:"speed_mbit"` // bandwidth cap in Mbit/s (0 = unlimited)
	LastRx     uint64     `json:"last_rx"`
	LastTx     uint64     `json:"last_tx"`
	Disabled   bool       `json:"disabled"`
	// Samples are daily snapshots of cumulative UsedBytes, used to compute usage
	// over a day/week/month window. Sorted ascending by Date (YYYY-MM-DD).
	Samples []UsageSample `json:"samples,omitempty"`
}

// UsageSample is the cumulative UsedBytes captured at the start of a calendar day.
type UsageSample struct {
	Date string `json:"date"` // "2006-01-02"
	Used uint64 `json:"used"`
}

// RemainingBytes returns quota left (0 if unlimited or exceeded).
func (r Record) RemainingBytes() uint64 {
	if r.QuotaBytes == 0 || r.UsedBytes >= r.QuotaBytes {
		return 0
	}
	return r.QuotaBytes - r.UsedBytes
}

// Store is a concurrency-safe, JSON-file-backed collection of records.
type Store struct {
	mu   sync.Mutex
	path string
	recs map[string]*Record
}

// Open loads the store from path, creating an empty one if the file is absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, recs: map[string]*Record{}}
	if err := s.loadFromDisk(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadFromDisk replaces the in-memory records with the file's current contents.
// Caller must hold the in-process mutex. An absent file yields an empty store.
// The store is rewritten atomically (temp + rename), so a reader always sees a
// complete file, never a torn one.
func (s *Store) loadFromDisk() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.recs = map[string]*Record{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading lifecycle store: %w", err)
	}
	var recs []*Record
	if len(data) > 0 {
		if err := json.Unmarshal(data, &recs); err != nil {
			return fmt.Errorf("parsing lifecycle store: %w", err)
		}
	}
	m := make(map[string]*Record, len(recs))
	for _, r := range recs {
		m[r.Name] = r
	}
	s.recs = m
	return nil
}

// lockPath is the advisory lock file guarding cross-process transactions.
func (s *Store) lockPath() string { return s.path + ".lock" }

// txn runs fn as a load-modify-save transaction that is exclusive across
// processes: it takes the file lock, re-reads the file so fn sees the latest
// committed state (including edits from another process, e.g. the panel daemon
// vs. the `awg-panel client-*` CLI), runs fn, then persists. fn mutates s.recs.
func (s *Store) txn(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := acquireLock(s.lockPath())
	if err != nil {
		return fmt.Errorf("locking lifecycle store: %w", err)
	}
	defer lock.release()
	if err := s.loadFromDisk(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return s.save()
}

// Reload refreshes the in-memory records from disk under the file lock, so a
// long-running process (the daemon) picks up edits made by another process.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := acquireLock(s.lockPath())
	if err != nil {
		return fmt.Errorf("locking lifecycle store: %w", err)
	}
	defer lock.release()
	return s.loadFromDisk()
}

// Mutate applies fn to the named record inside a cross-process transaction and
// persists. fn receives the record as currently stored on disk (so it never
// clobbers fields another process just wrote, e.g. accumulated usage). It
// returns an error if the client is unknown.
func (s *Store) Mutate(name string, fn func(*Record)) error {
	return s.txn(func() error {
		r, ok := s.recs[name]
		if !ok {
			return fmt.Errorf("client %q not found", name)
		}
		fn(r)
		return nil
	})
}

// List returns a stable, copied snapshot of all records.
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.recs))
	for _, r := range s.recs {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a copy of the named record.
func (s *Store) Get(name string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[name]
	if !ok {
		return Record{}, false
	}
	return *r, true
}

// Put inserts or replaces a record and persists. It runs as a cross-process
// transaction, so adding one client never drops records another process added
// concurrently. Note: replacing an existing record overwrites every field, so
// to change only some fields of an existing client use Mutate instead.
func (s *Store) Put(r Record) error {
	return s.txn(func() error {
		cp := r
		s.recs[r.Name] = &cp
		return nil
	})
}

// Delete removes a record and persists.
func (s *Store) Delete(name string) error {
	return s.txn(func() error {
		delete(s.recs, name)
		return nil
	})
}

// UsedOctets returns the host octets reserved by all known records (including
// disabled ones), so address allocation never collides with a client that may
// be re-enabled later.
func (s *Store) UsedOctets() map[int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := make(map[int]bool, len(s.recs))
	for _, r := range s.recs {
		used[r.Octet] = true
	}
	return used
}

// save writes the store to disk atomically. Caller must hold the lock.
func (s *Store) save() error {
	recs := make([]*Record, 0, len(s.recs))
	for _, r := range s.recs {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Name < recs[j].Name })
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing lifecycle store: %w", err)
	}
	return os.Rename(tmp, s.path)
}

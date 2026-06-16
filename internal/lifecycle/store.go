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
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading lifecycle store: %w", err)
	}
	var recs []*Record
	if len(data) > 0 {
		if err := json.Unmarshal(data, &recs); err != nil {
			return nil, fmt.Errorf("parsing lifecycle store: %w", err)
		}
	}
	for _, r := range recs {
		s.recs[r.Name] = r
	}
	return s, nil
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

// Put inserts or replaces a record and persists.
func (s *Store) Put(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := r
	s.recs[r.Name] = &cp
	return s.save()
}

// Delete removes a record and persists.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, name)
	return s.save()
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

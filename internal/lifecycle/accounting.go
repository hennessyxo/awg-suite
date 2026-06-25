package lifecycle

// Transfer is a point-in-time pair of WireGuard byte counters for a peer.
type Transfer struct {
	Rx, Tx uint64
}

// ApplyUsage accumulates traffic for known peers and persists.
//
// WireGuard's counters reset to zero when the interface restarts, so usage is
// tracked as the delta against the last seen values: if the current counter is
// lower than the last (a reset), the current value is treated as the delta.
func (s *Store) ApplyUsage(transfers map[string]Transfer) error {
	return s.txn(func() error {
		byKey := make(map[string]*Record, len(s.recs))
		for _, r := range s.recs {
			byKey[r.PubKey] = r
		}
		for pub, t := range transfers {
			r, ok := byKey[pub]
			if !ok {
				continue
			}
			r.UsedBytes += counterDelta(t.Rx, r.LastRx) + counterDelta(t.Tx, r.LastTx)
			r.LastRx, r.LastTx = t.Rx, t.Tx
		}
		return nil
	})
}

func counterDelta(cur, last uint64) uint64 {
	if cur >= last {
		return cur - last
	}
	return cur // counter reset (interface restart); count from zero
}

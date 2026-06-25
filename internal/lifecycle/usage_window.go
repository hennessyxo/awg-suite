package lifecycle

import "time"

const dateLayout = "2006-01-02"

// maxSamples bounds how many daily snapshots we keep per client (~5 weeks).
const maxSamples = 35

// WindowUsage returns the bytes used over the last `days` calendar days, given
// the daily samples (ascending by Date), the current cumulative usage, and now.
//
// usage = usedNow - baseline, where baseline is the cumulative usage at the start
// of the window: the latest sample dated on/before the window's first day. If no
// sample is that old, the earliest sample is used (usage since tracking began);
// with no samples at all the window is 0.
func WindowUsage(samples []UsageSample, usedNow uint64, now time.Time, days int) uint64 {
	if len(samples) == 0 || days < 1 {
		return 0
	}
	cutoff := now.AddDate(0, 0, -(days - 1)).Format(dateLayout)

	baseline := samples[0].Used // earliest, as a fallback
	for _, s := range samples {
		if s.Date <= cutoff {
			baseline = s.Used
		} else {
			break // samples are sorted ascending; no later one can match
		}
	}
	if usedNow <= baseline {
		return 0
	}
	return usedNow - baseline
}

// Today / Last7d / Last30d report usage over the respective windows.
func (r Record) Today(now time.Time) uint64  { return WindowUsage(r.Samples, r.UsedBytes, now, 1) }
func (r Record) Last7d(now time.Time) uint64 { return WindowUsage(r.Samples, r.UsedBytes, now, 7) }
func (r Record) Last30d(now time.Time) uint64 {
	return WindowUsage(r.Samples, r.UsedBytes, now, 30)
}

// DayTotal is one calendar day's total traffic across all clients.
type DayTotal struct {
	Date  string
	Bytes uint64
}

// DailyTotals returns total traffic (summed across all records) for each of the
// last `days` calendar days, oldest first. A record's traffic for a day is the
// increase in its cumulative usage between that day's snapshot and the next one
// (or the live total for the most recent open day). Days without data read 0.
func DailyTotals(recs []Record, now time.Time, days int) []DayTotal {
	if days < 1 {
		days = 1
	}
	out := make([]DayTotal, days)
	idx := make(map[string]int, days)
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i)).Format(dateLayout)
		out[i] = DayTotal{Date: d}
		idx[d] = i
	}
	for _, r := range recs {
		n := len(r.Samples)
		for i := 0; i < n; i++ {
			next := r.UsedBytes
			if i+1 < n {
				next = r.Samples[i+1].Used
			}
			var delta uint64
			if next > r.Samples[i].Used {
				delta = next - r.Samples[i].Used
			}
			if j, ok := idx[r.Samples[i].Date]; ok {
				out[j].Bytes += delta
			}
		}
	}
	return out
}

// DailyTotals returns the per-day traffic series over the last `days` days.
func (s *Store) DailyTotals(now time.Time, days int) []DayTotal {
	return DailyTotals(s.List(), now, days)
}

// RecordSamples ensures every record has a snapshot for today's date (capturing
// the cumulative UsedBytes at the first reconcile of the day) and prunes old
// samples. Called by the enforcer after usage is applied. Persists once.
func (s *Store) RecordSamples(now time.Time) error {
	today := now.Format(dateLayout)
	return s.txn(func() error {
		changed := false
		for _, r := range s.recs {
			if appendDailySample(r, today) {
				changed = true
			}
		}
		_ = changed // always persist within the transaction; save() is cheap
		return nil
	})
}

// appendDailySample adds today's sample to r if absent and prunes to maxSamples.
// Returns whether r was modified.
func appendDailySample(r *Record, today string) bool {
	if n := len(r.Samples); n > 0 && r.Samples[n-1].Date == today {
		return false // already sampled today
	}
	r.Samples = append(r.Samples, UsageSample{Date: today, Used: r.UsedBytes})
	if len(r.Samples) > maxSamples {
		r.Samples = r.Samples[len(r.Samples)-maxSamples:]
	}
	return true
}

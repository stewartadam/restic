package restic

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// ExpirePolicy configures which snapshots should be automatically removed.
type ExpirePolicy struct {
	Last    int       // keep the last n snapshots
	Hourly  int       // keep the last n hourly snapshots
	Daily   int       // keep the last n daily snapshots
	Weekly  int       // keep the last n weekly snapshots
	Monthly int       // keep the last n monthly snapshots
	Yearly  int       // keep the last n yearly snapshots
	Within  Duration  // keep snapshots made within this duration
	Tags    []TagList // keep all snapshots that include at least one of the tag lists.
}

func (e ExpirePolicy) String() (s string) {
	var keeps []string
	if e.Last > 0 {
		keeps = append(keeps, fmt.Sprintf("%d snapshots", e.Last))
	}
	if e.Hourly > 0 {
		keeps = append(keeps, fmt.Sprintf("%d hourly", e.Hourly))
	}
	if e.Daily > 0 {
		keeps = append(keeps, fmt.Sprintf("%d daily", e.Daily))
	}
	if e.Weekly > 0 {
		keeps = append(keeps, fmt.Sprintf("%d weekly", e.Weekly))
	}
	if e.Monthly > 0 {
		keeps = append(keeps, fmt.Sprintf("%d monthly", e.Monthly))
	}
	if e.Yearly > 0 {
		keeps = append(keeps, fmt.Sprintf("%d yearly", e.Yearly))
	}

	if len(keeps) > 0 {
		s = fmt.Sprintf("keep the last %s snapshots", strings.Join(keeps, ", "))
	}

	if len(e.Tags) > 0 {
		if s != "" {
			s += " and "
		}
		s += fmt.Sprintf("all snapshots with tags %s", e.Tags)
	}

	if !e.Within.Zero() {
		if s != "" {
			s += " and "
		}
		s += fmt.Sprintf("all snapshots within %s of the newest", e.Within)
	}

	return s
}

// Sum returns the maximum number of snapshots to be kept according to this
// policy.
func (e ExpirePolicy) Sum() int {
	return e.Last + e.Hourly + e.Daily + e.Weekly + e.Monthly + e.Yearly
}

// Empty returns true iff no policy has been configured (all values zero).
func (e ExpirePolicy) Empty() bool {
	if len(e.Tags) != 0 {
		return false
	}

	empty := ExpirePolicy{Tags: e.Tags}
	return reflect.DeepEqual(e, empty)
}

// ymdh returns an integer in the form YYYYMMDDHH.
func ymdh(d time.Time, _ int) int {
	return d.Year()*1000000 + int(d.Month())*10000 + d.Day()*100 + d.Hour()
}

// ymd returns an integer in the form YYYYMMDD.
func ymd(d time.Time, _ int) int {
	return d.Year()*10000 + int(d.Month())*100 + d.Day()
}

// yw returns an integer in the form YYYYWW, where WW is the week number.
func yw(d time.Time, _ int) int {
	year, week := d.ISOWeek()
	return year*100 + week
}

// ym returns an integer in the form YYYYMM.
func ym(d time.Time, _ int) int {
	return d.Year()*100 + int(d.Month())
}

// y returns the year of d.
func y(d time.Time, _ int) int {
	return d.Year()
}

// always returns a unique number for d.
func always(d time.Time, nr int) int {
	return nr
}

// findLatestTimestamp returns the time stamp for the newest snapshot.
func findLatestTimestamp(list Snapshots) time.Time {
	if len(list) == 0 {
		panic("list of snapshots is empty")
	}

	var latest time.Time
	for _, sn := range list {
		if sn.Time.After(latest) {
			latest = sn.Time
		}
	}

	return latest
}

// ApplyPolicy returns the snapshots from list that are to be kept and removed
// according to the policy p. list is sorted in the process.
func ApplyPolicy(list Snapshots, p ExpirePolicy) (keep, remove Snapshots) {
	sort.Sort(list)

	if p.Empty() {
		return list, remove
	}

	if len(list) == 0 {
		return list, remove
	}

	var buckets = [6]struct {
		Count  int
		bucker func(d time.Time, nr int) int
		Last   int
	}{
		{p.Last, always, -1},
		{p.Hourly, ymdh, -1},
		{p.Daily, ymd, -1},
		{p.Weekly, yw, -1},
		{p.Monthly, ym, -1},
		{p.Yearly, y, -1},
	}

	latest := findLatestTimestamp(list)

	for nr, cur := range list {
		var keepSnap bool

		// Tags are handled specially as they are not counted.
		for _, l := range p.Tags {
			if cur.HasTags(l) {
				keepSnap = true
			}
		}

		// If the timestamp of the snapshot is within the range, then keep it.
		if !p.Within.Zero() {
			t := latest.AddDate(-p.Within.Years, -p.Within.Months, -p.Within.Days)
			if cur.Time.After(t) {
				keepSnap = true
			}
		}

		// Now update the other buckets and see if they have some counts left.
		for i, b := range buckets {
			if b.Count > 0 {
				val := b.bucker(cur.Time, nr)
				if val != b.Last {
					keepSnap = true
					buckets[i].Last = val
					buckets[i].Count--
				}
			}
		}

		if keepSnap {
			keep = append(keep, cur)
		} else {
			remove = append(remove, cur)
		}
	}

	return keep, remove
}

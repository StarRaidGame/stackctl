// Package sysproc reads per-service CPU and memory usage from Linux /proc,
// aggregated by process group. The supervisor launches each daemon as its own
// process-group leader (Setpgid), so summing every process whose pgrp matches a
// service's leader pid captures its whole `just`→`go run`→binary tree in one go.
package sysproc

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// clkTck is the kernel's clock ticks per second (_SC_CLK_TCK). It is 100 on
// effectively all Linux builds; there is no stdlib sysconf, so we assume it.
const clkTck = 100.0

// Usage is a service's aggregated resource use.
type Usage struct {
	CPUPercent float64 // busy CPU% over the last sampling interval (100 = one core)
	RSSBytes   uint64  // resident memory summed over the process group
}

// Sampler computes CPU% by differencing cumulative CPU time between calls, so the
// first Sample for a group reports 0% CPU (no prior baseline) but real memory.
type Sampler struct {
	prevJiffies map[int]uint64 // pgid → cumulative (utime+stime) at prevAt
	prevAt      time.Time
	pageSize    uint64
}

// NewSampler returns a ready sampler.
func NewSampler() *Sampler {
	return &Sampler{prevJiffies: map[int]uint64{}, pageSize: uint64(os.Getpagesize())}
}

// Sample returns usage per requested process-group id (a service's leader pid).
// Groups with no live processes report zero.
func (s *Sampler) Sample(pgids []int) map[int]Usage {
	want := make(map[int]bool, len(pgids))
	for _, g := range pgids {
		if g > 0 {
			want[g] = true
		}
	}

	jiffies := make(map[int]uint64, len(want))
	rss := make(map[int]uint64, len(want))
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			if _, err := strconv.Atoi(e.Name()); err != nil {
				continue // not a pid dir
			}
			b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
			if err != nil {
				continue // process vanished mid-scan
			}
			pgrp, j, r, ok := parseStat(string(b))
			if !ok || !want[pgrp] {
				continue
			}
			jiffies[pgrp] += j
			rss[pgrp] += r
		}
	}

	now := time.Now()
	wall := now.Sub(s.prevAt).Seconds()
	out := make(map[int]Usage, len(want))
	for g := range want {
		u := Usage{RSSBytes: rss[g] * s.pageSize}
		if prev, ok := s.prevJiffies[g]; ok && wall > 0 {
			if dj := float64(jiffies[g]) - float64(prev); dj > 0 {
				u.CPUPercent = dj / clkTck / wall * 100
			}
		}
		out[g] = u
	}
	s.prevJiffies = jiffies
	s.prevAt = now
	return out
}

// parseStat pulls pgrp, cumulative CPU jiffies (utime+stime), and RSS pages from a
// /proc/<pid>/stat line. comm (field 2) is parenthesised and may contain spaces or
// ')', so we index fields relative to the LAST ')': after it, field 3 (state) is
// the first token, so stat field N is token N-3.
func parseStat(line string) (pgrp int, jiffies, rssPages uint64, ok bool) {
	rp := strings.LastIndexByte(line, ')')
	if rp < 0 || rp+2 >= len(line) {
		return 0, 0, 0, false
	}
	f := strings.Fields(line[rp+2:])
	if len(f) < 22 { // need up to stat field 24 (rss) → token index 21
		return 0, 0, 0, false
	}
	pg, err := strconv.Atoi(f[2]) // field 5: pgrp
	if err != nil {
		return 0, 0, 0, false
	}
	utime, e1 := strconv.ParseUint(f[11], 10, 64) // field 14
	stime, e2 := strconv.ParseUint(f[12], 10, 64) // field 15
	rss, e3 := strconv.ParseUint(f[21], 10, 64)   // field 24
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, 0, 0, false
	}
	return pg, utime + stime, rss, true
}

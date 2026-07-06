package sysproc

import (
	"syscall"
	"testing"
)

func TestParseStat(t *testing.T) {
	// comm deliberately contains spaces and a ')': "(sh -c) weird".
	line := "1234 (sh -c) weird) S 1000 4321 4321 0 -1 4194304 " +
		"100 200 0 0 " + // fields 10-13
		"50 25 " + // 14 utime, 15 stime
		"0 0 20 0 1 0 999 " + // 16-22
		"12345678 " + // 23 vsize
		"512 " + // 24 rss (pages)
		"more fields ignored"
	pgrp, j, rss, ok := parseStat(line)
	if !ok {
		t.Fatal("parseStat: ok=false")
	}
	if pgrp != 4321 {
		t.Fatalf("pgrp = %d, want 4321", pgrp)
	}
	if j != 75 {
		t.Fatalf("jiffies = %d, want 75 (50+25)", j)
	}
	if rss != 512 {
		t.Fatalf("rss pages = %d, want 512", rss)
	}
}

func TestParseStatShortLine(t *testing.T) {
	if _, _, _, ok := parseStat("1 (init) S 0 1 1"); ok {
		t.Fatal("want ok=false for a truncated stat line")
	}
}

func TestSampleSelfHasMemory(t *testing.T) {
	// The test process's own group must report non-zero RSS; first sample is 0% CPU.
	pgid := syscall.Getpgrp()
	s := NewSampler()
	u := s.Sample([]int{pgid})[pgid]
	if u.RSSBytes == 0 {
		t.Fatalf("self group RSS should be > 0, got %d", u.RSSBytes)
	}
	if u.CPUPercent != 0 {
		t.Fatalf("first sample CPU should be 0 (no baseline), got %v", u.CPUPercent)
	}
}

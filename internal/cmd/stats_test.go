package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/metrics"
)

func TestRunStatsNoMetricsDir(t *testing.T) {
	target := t.TempDir()
	out := &bytes.Buffer{}

	if err := runStats(target, out); err != nil {
		t.Fatalf("runStats() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "no metrics recorded yet" {
		t.Errorf("output = %q, want %q", out.String(), "no metrics recorded yet")
	}
}

func TestRunStatsFormatsCounts(t *testing.T) {
	target := t.TempDir()
	dir := metrics.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "handoffs.count"), []byte("12"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compressions.count"), []byte("7"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "future_mechanism.count"), []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	if err := runStats(target, out); err != nil {
		t.Fatalf("runStats() error = %v", err)
	}
	got := out.String()

	for _, want := range []string{"12 handoffs", "7 context compressions", "3 future_mechanism"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	// deterministic order: alphabetical by key (compressions, future_mechanism, handoffs)
	if strings.Index(got, "context compressions") > strings.Index(got, "future_mechanism") ||
		strings.Index(got, "future_mechanism") > strings.Index(got, "handoffs") {
		t.Errorf("output = %q, want alphabetical-by-key order", got)
	}
}

func TestStatsCmdUse(t *testing.T) {
	c := newStatsCmd()
	if c.Use != "stats [path]" {
		t.Errorf("Use = %q, want %q", c.Use, "stats [path]")
	}
}

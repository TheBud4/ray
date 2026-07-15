package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	got := Dir("/proj")
	want := filepath.Join("/proj", ".claude", ".ray-metrics")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func seedCountFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAggregateReadsCountFiles(t *testing.T) {
	dir := t.TempDir()
	seedCountFile(t, dir, "handoffs.count", "12\n")
	seedCountFile(t, dir, "compressions.count", "7")

	got, err := Aggregate(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"handoffs": 12, "compressions": 7}
	if len(got) != len(want) || got["handoffs"] != 12 || got["compressions"] != 7 {
		t.Errorf("Aggregate() = %v, want %v", got, want)
	}
}

func TestAggregateMissingDirReturnsEmpty(t *testing.T) {
	got, err := Aggregate(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Aggregate() error = %v, want nil for a missing dir", err)
	}
	if len(got) != 0 {
		t.Errorf("Aggregate() = %v, want empty map", got)
	}
}

func TestAggregateSkipsMalformedFile(t *testing.T) {
	dir := t.TempDir()
	seedCountFile(t, dir, "handoffs.count", "not-a-number")
	seedCountFile(t, dir, "compressions.count", "3")

	got, err := Aggregate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["handoffs"]; ok {
		t.Errorf("Aggregate() = %v, want the malformed handoffs.count skipped", got)
	}
	if got["compressions"] != 3 {
		t.Errorf("Aggregate()[\"compressions\"] = %d, want 3", got["compressions"])
	}
}

func TestAggregateIgnoresNonCountFiles(t *testing.T) {
	dir := t.TempDir()
	seedCountFile(t, dir, "handoffs.count", "5")
	seedCountFile(t, dir, "README.md", "not a metric")

	got, err := Aggregate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["handoffs"] != 5 {
		t.Errorf("Aggregate() = %v, want only {handoffs: 5}", got)
	}
}

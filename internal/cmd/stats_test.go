package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/metrics"
	"github.com/TheBud4/ray/internal/scaffold"
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

// O contador de handoffs é escrito por um hook em bash e lido por Go, e a chave
// ("handoffs") mora nos dois lados sem nada que os ligue. Aqui o hook roda de
// verdade e o `ray stats` tem de enxergar o que ele escreveu, com o rótulo
// conhecido — renomear o arquivo de contador num dos lados quebra este teste, em
// vez de o stats passar a mostrar zero em silêncio.
func TestRunStatsReadsTheCounterTheSessionHookWrites(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash ausente")
	}
	target := t.TempDir()
	if _, err := scaffold.WriteFiles(scaffold.SystemFiles(), scaffold.Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	// Sem handoff.md o hook não conta atividade; com ele, conta uma injeção.
	if err := os.WriteFile(filepath.Join(target, ".claude", "handoff.md"), []byte("# h\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(target, ".claude", "hooks", "session-start.sh")
	for i := 0; i < 2; i++ {
		cmd := exec.Command("bash", hook)
		cmd.Dir = target
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("session-start.sh: %v\n%s", err, out)
		}
	}

	out := &bytes.Buffer{}
	if err := runStats(target, out); err != nil {
		t.Fatalf("runStats() error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "2 "+statsLabels["handoffs"] {
		t.Errorf("runStats() = %q, want %q: the hook counter and the stats key must agree", got, "2 "+statsLabels["handoffs"])
	}
}

package testenv

import (
	"errors"
	"fmt"
	"testing"
)

type fakeT struct{ skipped, failed string }

func (f *fakeT) Helper()                       {}
func (f *fakeT) Skipf(format string, a ...any) { f.skipped = fmt.Sprintf(format, a...) }
func (f *fakeT) Fatalf(format string, a ...any) {
	f.failed = fmt.Sprintf(format, a...)
}

// Sem a exigência, quem não consegue criar symlink pula — é o caso do
// desenvolvedor no Windows sem privilégio. Com ela (o CI), o mesmo erro falha:
// um pulo silencioso no runner faria a garantia de symlink parecer medida.
func TestSymlinkUnavailableSkipsUnlessRequired(t *testing.T) {
	cause := errors.New("privilege not held")

	t.Setenv(RequireSymlinksEnv, "")
	var lax fakeT
	SymlinkUnavailable(&lax, cause)
	if lax.skipped == "" || lax.failed != "" {
		t.Errorf("without %s: skipped=%q failed=%q, want a skip", RequireSymlinksEnv, lax.skipped, lax.failed)
	}

	t.Setenv(RequireSymlinksEnv, "1")
	var strict fakeT
	SymlinkUnavailable(&strict, cause)
	if strict.failed == "" || strict.skipped != "" {
		t.Errorf("with %s: skipped=%q failed=%q, want a failure", RequireSymlinksEnv, strict.skipped, strict.failed)
	}
}

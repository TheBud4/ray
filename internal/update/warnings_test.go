package update

import (
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

// O motivo de um item mora no Detail da entrada; repeti-lo em Warnings faz a
// tela dizer a mesma coisa duas vezes.
func TestRunDoesNotRepeatAKeptComponentInWarnings(t *testing.T) {
	for name, dry := range map[string]bool{"real run": false, "dry-run": true} {
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "# new upstream")
			writeProfile(t, home.ProfilesDir, testProfile())
			target := t.TempDir()
			writeProfileRecord(t, target, "test")
			installOnDisk(t, target, "# my local edit", "# original")

			sum := updateWith(t, home, target, &runner.FakeRunner{}, Options{NoGlobal: true, DryRun: dry})

			if len(sum.Warnings) != 0 {
				t.Errorf("Warnings = %v, want none: the preserved component is already an entry with its reason", sum.Warnings)
			}
		})
	}
}

func TestRunDoesNotRepeatAFailedToolUpgradeInWarnings(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"uv tool upgrade headroom-ai": {ExitCode: 1, Stderr: "error: network unreachable\n"},
	}}

	sum := updateWith(t, home, target, fr, Options{})

	if len(sum.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: the failed upgrade is already an entry with its reason", sum.Warnings)
	}
}

package update

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

// ---- decideOverwrite (pure) -------------------------------------------

func TestDecideOverwrite(t *testing.T) {
	cases := []struct {
		name                                string
		force, onDiskExists, hasPristine    bool
		onDiskHash, freshHash, pristineHash string
		wantOverwrite                       bool
	}{
		{"force always overwrites", true, true, true, "a", "b", "c", true},
		{"no on-disk content yet", false, false, false, "", "b", "", true},
		{"disk matches pristine", false, true, true, "a", "fresh", "a", true},
		{"disk differs from pristine is a fork", false, true, true, "edited", "fresh", "a", false},
		{"no pristine, disk matches fresh", false, true, false, "fresh", "fresh", "", true},
		{"no pristine, disk differs from fresh is ambiguous", false, true, false, "edited", "fresh", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := decideOverwrite(tc.force, tc.onDiskExists, tc.onDiskHash, tc.freshHash, tc.pristineHash, tc.hasPristine)
			if got != tc.wantOverwrite {
				t.Errorf("decideOverwrite() = %v, want %v", got, tc.wantOverwrite)
			}
		})
	}
}

// ---- Run: fixtures -----------------------------------------------------

func testProfile() *profile.Profile {
	return &profile.Profile{
		Name:         "test",
		Integrations: profile.Integrations{Headroom: true, CodeGraph: true},
		Components:   []profile.Component{{Name: "s", Dest: ".claude/skills"}},
	}
}

func writeProfile(t *testing.T, profilesDir string, p *profile.Profile) {
	t.Helper()
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, p.Name+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newHome(t *testing.T) Home {
	t.Helper()
	base := t.TempDir()
	return Home{
		ProfilesDir:   filepath.Join(base, "profiles"),
		StoreDir:      filepath.Join(base, "store"),
		ComponentsDir: filepath.Join(base, "components"),
	}
}

// seedComponent grava o conteúdo "upstream" do componente s em
// home.ComponentsDir — é dali que `ray update` recopia, nunca da rede.
func seedComponent(t *testing.T, home Home, content string) {
	t.Helper()
	dir := filepath.Join(home.ComponentsDir, "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitClean makes check.Run report a clean tree for `git status --porcelain`.
func cleanGitCheck() *runner.FakeRunner {
	return &runner.FakeRunner{Results: map[string]runner.Result{
		"git status --porcelain": {ExitCode: 0, Stdout: ""},
	}}
}

func dirtyGitCheck() *runner.FakeRunner {
	return &runner.FakeRunner{Results: map[string]runner.Result{
		"git status --porcelain": {ExitCode: 0, Stdout: " M .claude/skills/s/SKILL.md\n"},
	}}
}

const coordS = "s"

// ---- Run: clean-tree guard ---------------------------------------------

func TestRunDirtyTreeAbortsWithoutForce(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	_, err := Run(&runner.FakeRunner{}, dirtyGitCheck(), Options{Target: target}, home)
	if err == nil {
		t.Fatal("Run() = nil error, want error on a dirty tree without --force")
	}
	if strings.Contains(err.Error(), "no profile recorded") {
		t.Fatalf("Run() error = %v, want the dirty-tree guard error, not a profile-resolution error", err)
	}
}

func TestRunDirtyTreeWithForceProceeds(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(&runner.FakeRunner{}, dirtyGitCheck(), Options{Target: target, Force: true}, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil with --force", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}
}

// O guard existe para o diff do update ficar legível — e um dry-run não
// produz diff nenhum. Barrar a simulação empurra a pessoa para o --force, que
// é o oposto do que o guard quer.
func TestRunDirtyTreeAllowsDryRun(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(runner.ExecRunner{DryRun: true}, dirtyGitCheck(),
		Options{Target: target, DryRun: true, Out: &bytes.Buffer{}}, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil — a dry-run cannot dirty anything", err)
	}
	if len(sum.Updated) == 0 {
		t.Error("Summary.Updated is empty, want the dry-run to still report the plan")
	}
}

func TestRunCleanTreeProceeds(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}
}

// ---- Run: profile discovery ---------------------------------------------

func writeProfileRecord(t *testing.T, target, name string) {
	t.Helper()
	dir := filepath.Join(target, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ray-profile"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunReadsProfileFromRecord(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want it to resolve the profile from .claude/.ray-profile", err)
	}
}

func TestRunProfileFlagOverridesRecord(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "some-other-nonexistent-profile")

	_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, Profile: "test"}, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want --profile to override the record", err)
	}
}

func TestRunMissingProfileRecordAndNoOverrideErrors(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err == nil {
		t.Fatal("Run() = nil error, want error when no .claude/.ray-profile and no --profile")
	}
}

// ---- Run: tool upgrades ---------------------------------------------

func TestRunUpgradesTools(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	fr := &runner.FakeRunner{}
	sum, err := Run(fr, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	wantCmds := []string{"uv tool upgrade headroom-ai", "uv tool upgrade graphifyy"}
	for _, want := range wantCmds {
		found := false
		for _, c := range fr.Calls {
			if c.String() == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Calls = %v, want it to include %q", fr.Calls, want)
		}
	}
}

func TestRunNoGlobalSkipsToolUpgrades(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	fr := &runner.FakeRunner{}
	sum, err := Run(fr, cleanGitCheck(), Options{Target: target, NoGlobal: true}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, call := range fr.Calls {
		if s := call.String(); strings.Contains(s, "uv tool upgrade") {
			t.Errorf("global tool upgrade ran despite --no-global: %q", s)
		}
	}
	// O passo project-local segue: --no-global recorta a máquina, não o alvo.
	if len(sum.Updated) == 0 {
		t.Error("Updated is empty, want the project-local content step to still run")
	}
}

// ---- Run: content re-acquisition + fork detection ------------------------

func TestRunOverwritesWhenDiskMatchesPristine(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	pristineDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(pristineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pristineDir, "SKILL.md"), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	pristineHash, err := store.HashTree(pristineDir)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, coordS, pristineHash); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	got, err := os.ReadFile(filepath.Join(pristineDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# new upstream" {
		t.Errorf("SKILL.md = %q, want the fresh upstream content", got)
	}
	found := false
	for _, u := range sum.Updated {
		if u == coordS {
			found = true
		}
	}
	if !found {
		t.Errorf("Updated = %v, want it to include %q", sum.Updated, coordS)
	}

	newPristine, ok := st.PristineHash(target, coordS)
	if !ok {
		t.Fatal("PristineHash() ok = false after overwrite, want it re-recorded")
	}
	wantHash, _ := store.HashTree(pristineDir)
	if newPristine != wantHash {
		t.Errorf("PristineHash() = %q, want %q (the new on-disk hash)", newPristine, wantHash)
	}
}

func TestRunSkipsForkWithoutForce(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# my local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pristine baseline reflects what was originally installed, NOT what's
	// on disk now — this is the fork.
	oldPristine, err := store.HashTree(seedTempFile(t, "# original"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, coordS, oldPristine); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# my local edit" {
		t.Errorf("SKILL.md = %q, want the local edit to survive (fork protected)", got)
	}
	found := false
	for _, s := range sum.Skipped {
		if s == coordS {
			found = true
		}
	}
	if !found {
		t.Errorf("Skipped = %v, want it to include %q", sum.Skipped, coordS)
	}
	if len(sum.Warnings) == 0 {
		t.Error("Warnings is empty, want a fork warning")
	}
}

func TestRunForceOverwritesFork(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# my local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPristine, err := store.HashTree(seedTempFile(t, "# original"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, coordS, oldPristine); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, Force: true}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# new upstream" {
		t.Errorf("SKILL.md = %q, want --force to overwrite the local edit", got)
	}
}

func TestRunNewCloneNoPristineMatchesUpstreamOverwrites(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	// On disk already, but no pristine recorded locally (a fresh clone) — and
	// the content happens to match what home.ComponentsDir has now, so it's
	// not a fork; the degradation path should overwrite (and record
	// pristine).
	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# new upstream"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}
	found := false
	for _, u := range sum.Updated {
		if u == coordS {
			found = true
		}
	}
	if !found {
		t.Errorf("Updated = %v, want it to include %q (matches upstream, not a fork)", sum.Updated, coordS)
	}

	st := store.New(home.StoreDir)
	if _, ok := st.PristineHash(target, coordS); !ok {
		t.Error("PristineHash() ok = false, want it recorded now that we've confirmed it's not a fork")
	}
}

func TestRunNewCloneNoPristineDiffersFromUpstreamSkips(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# something else entirely"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# something else entirely" {
		t.Errorf("SKILL.md = %q, want it untouched (ambiguous fork, no pristine baseline)", got)
	}
	found := false
	for _, s := range sum.Skipped {
		if s == coordS {
			found = true
		}
	}
	if !found {
		t.Errorf("Skipped = %v, want it to include %q", sum.Skipped, coordS)
	}
}

// ---- Run: component not found in the local overlay -----------------------

// Sem rede, "componente não encontrado" substitui o antigo caso de exit code
// de instalador: é a única forma de um componente falhar agora.
func TestRunSkipsComponentNotFoundInComponentsDir(t *testing.T) {
	home := newHome(t)
	// Sem seedComponent: home.ComponentsDir/s não existe.
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	found := false
	for _, s := range sum.Skipped {
		if strings.Contains(s, "not found") {
			found = true
		}
	}
	if !found {
		t.Errorf("Skipped = %v, want an entry naming the missing component", sum.Skipped)
	}
}

// ---- Run: dry-run ---------------------------------------------------

func TestRunDryRunFetchesNothing(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(runner.ExecRunner{DryRun: true}, cleanGitCheck(), Options{Target: target, DryRun: true, Out: os.Stdout}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, ".claude", "skills", "s", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Error("content should not exist after dry-run")
	}
	if len(sum.Updated) == 0 {
		t.Error("Summary.Updated should still report what would be updated in dry-run")
	}
}

// O dry-run tem de aplicar a mesma decisão da execução real quando ela é
// decidível offline — que é o caso normal, com linha-base gravada. Dizer
// "Updated" sobre o que será preservado é pior que não dizer nada: o dry-run
// existe para se confiar nele antes de rodar.
func TestRunDryRunReportsForkAsSkipped(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# my local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPristine, err := store.HashTree(seedTempFile(t, "# original"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, coordS, oldPristine); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(runner.ExecRunner{DryRun: true}, cleanGitCheck(),
		Options{Target: target, DryRun: true, Out: &bytes.Buffer{}}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, u := range sum.Updated {
		if u == coordS {
			t.Errorf("Updated = %v, want %q out of it — the real run preserves a local edit", sum.Updated, coordS)
		}
	}
	found := false
	for _, s := range sum.Skipped {
		if s == coordS {
			found = true
		}
	}
	if !found {
		t.Errorf("Skipped = %v, want it to include %q", sum.Skipped, coordS)
	}
	if len(sum.Warnings) == 0 {
		t.Error("Warnings is empty, want the fork warning in dry-run too")
	}
}

// Sem download, o "upstream" é home.ComponentsDir — uma leitura de disco
// local, livre mesmo em dry-run. Diferente da versão adquirida por rede, o
// dry-run agora decide exatamente como uma execução real decidiria, mesmo
// sem linha-base: não há mais um caso "procedência desconhecida" que só o
// upstream resolveria.
func TestRunDryRunDecidesExactlyLikeRealRunWithoutPristine(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# whatever"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Nenhum SetPristine: mesmo caso que TestRunNewCloneNoPristineDiffersFromUpstreamSkips.

	sum, err := Run(runner.ExecRunner{DryRun: true}, cleanGitCheck(),
		Options{Target: target, DryRun: true, Out: &bytes.Buffer{}}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !slices.Contains(sum.Skipped, coordS) {
		t.Errorf("Skipped = %v, want it to include %q — no pristine, disk differs from the local component", sum.Skipped, coordS)
	}
	if len(sum.Warnings) == 0 {
		t.Error("Warnings is empty, want the same reason a real run would give")
	}
}

// Guarda de não-regressão: decidir offline não pode virar desculpa para buscar.
func TestRunDryRunStillFetchesNothing(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	fr := &runner.FakeRunner{}
	if _, err := Run(fr, cleanGitCheck(), Options{Target: target, DryRun: true, Out: &bytes.Buffer{}}, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, c := range fr.Calls {
		if strings.Contains(c.String(), "skills add") || strings.Contains(c.String(), "clone") {
			t.Errorf("dry-run ran %q, want no acquisition", c.String())
		}
	}
}

// seedTempFile writes content to a fresh temp dir/file and returns the path,
// for hashing a "was originally" baseline that no longer matches the disk.
func seedTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

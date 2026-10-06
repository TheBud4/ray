package update

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

// Um upgrade de ferramenta da máquina não é um componente do projeto: listá-lo
// em "Updated" ao lado das skills dizia que o projeto mudou quando só o uv
// rodou. Ele tem lista própria.
func TestRunReportsToolUpgradesApartFromComponents(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, want := range []string{"uv tool upgrade headroom-ai", "uv tool upgrade graphifyy"} {
		if !slices.Contains(sum.Tools, want) {
			t.Errorf("Tools = %v, want it to include %q", sum.Tools, want)
		}
		if slices.Contains(sum.Updated, want) {
			t.Errorf("Updated = %v, want the tool upgrade %q out of it", sum.Updated, want)
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

func TestRunNewCloneNoPristineMatchesUpstreamRecordsPristine(t *testing.T) {
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
	// O conteúdo já é o do upstream: não há o que recopiar, só o pristino a
	// gravar. Dizer "Updated" aqui afirmaria uma mudança que não houve.
	if !slices.Contains(sum.Unchanged, coordS) || slices.Contains(sum.Updated, coordS) {
		t.Errorf("Unchanged = %v, Updated = %v, want %q only in Unchanged (matches upstream, not a fork)", sum.Unchanged, sum.Updated, coordS)
	}

	st := store.New(home.StoreDir)
	if _, ok := st.PristineHash(target, coordS); !ok {
		t.Error("PristineHash() ok = false, want it recorded now that we've confirmed it's not a fork")
	}
}

// Componente que já é igual ao do upstream não é recopiado nem listado como
// atualizado: re-rodar o `update` sem novidade imprimia "Updated" para tudo e
// regravava os arquivos (mtime novo, watcher e build incremental acordados).
func TestRunIdenticalComponentIsNeitherRecopiedNorReportedUpdated(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# same")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(file, []byte("# same"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	hash, err := store.HashTree(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.New(home.StoreDir).SetPristine(target, coordS, hash); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, NoGlobal: true}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !slices.Contains(sum.Unchanged, coordS) {
		t.Errorf("Unchanged = %v, want it to include %q", sum.Unchanged, coordS)
	}
	if slices.Contains(sum.Updated, coordS) {
		t.Errorf("Updated = %v, want %q out of it: nothing changed", sum.Updated, coordS)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("SKILL.md mtime = %v, want it untouched (%v): the identical component must not be rewritten", info.ModTime(), old)
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

// writeRawProfile grava YAML cru como receita: é assim que uma receita hostil
// chega ao disco (editada à mão ou trazida de fora), sem passar por Validate.
func writeRawProfile(t *testing.T, dir, name, yamlText string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Uma receita com componente que sobe diretórios (ou que aponta para a raiz do
// projeto) é recusada ao carregar, antes de qualquer cópia ou RemoveAll — com e
// sem --force. O alvo e o que há nele continuam intactos.
func TestRunRefusesHostileRecipeBeforeAnyEffect(t *testing.T) {
	recipes := map[string]string{
		"name climbs":   "name: hostile\ncomponents:\n  - name: ..\n    dest: .claude/skills\n",
		"name is root":  "name: hostile\ncomponents:\n  - name: .\n    dest: .\n",
		"dest climbs":   "name: hostile\ncomponents:\n  - name: s\n    dest: ../../elsewhere\n",
		"dest absolute": "name: hostile\ncomponents:\n  - name: s\n    dest: /tmp\n",
	}
	for label, yamlText := range recipes {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/force=%v", label, force), func(t *testing.T) {
				home := newHome(t)
				seedComponent(t, home, "# upstream")
				writeRawProfile(t, home.ProfilesDir, "hostile", yamlText)
				target := t.TempDir()
				sentinel := filepath.Join(target, "keep.txt")
				if err := os.WriteFile(sentinel, []byte("mine"), 0o644); err != nil {
					t.Fatal(err)
				}

				_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Profile: "hostile", Target: target, Force: force, NoGlobal: true}, home)
				if err == nil || !strings.Contains(err.Error(), "invalid profile") {
					t.Fatalf("Run() error = %v, want an invalid profile error", err)
				}
				if got, rerr := os.ReadFile(sentinel); rerr != nil || string(got) != "mine" {
					t.Errorf("target file = %q, %v; want it untouched (force=%v)", got, rerr, force)
				}
				if _, serr := os.Stat(filepath.Join(target, ".claude")); !os.IsNotExist(serr) {
					t.Errorf(".claude was created in the target (stat err = %v)", serr)
				}
			})
		}
	}
}

// Um .claude/.ray-profile forjado num repo clonado não pode escolher uma receita
// de fora de ProfilesDir: a receita de fora é válida e copiaria um componente
// para dentro do projeto se fosse carregada.
func TestRunRefusesForgedProfileRecord(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# upstream")
	if err := os.MkdirAll(home.ProfilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// ../evil.yaml fica ao lado de ProfilesDir, fora dele.
	writeRawProfile(t, filepath.Dir(home.ProfilesDir), "evil", "name: evil\ncomponents:\n  - name: s\n    dest: .claude/skills\n")
	target := t.TempDir()
	writeProfileRecord(t, target, "../evil")

	_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, Force: true, NoGlobal: true}, home)
	if err == nil || !strings.Contains(err.Error(), "single path element") {
		t.Fatalf("Run() error = %v, want an error mentioning a single path element", err)
	}
	if _, serr := os.Stat(filepath.Join(target, ".claude", "skills")); !os.IsNotExist(serr) {
		t.Errorf("a component was copied from the forged recipe (stat err = %v)", serr)
	}
}

// seedEditedComponentWithDanglingSymlink deixa em target uma cópia editada do
// componente s com um symlink pendente dentro — o que faz o hash da pasta
// falhar. A linha-base é a do conteúdo original, então a pasta é um fork.
func seedEditedComponentWithDanglingSymlink(t *testing.T, home Home, target string) string {
	t.Helper()
	skillDir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# my local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "no-such-target"), filepath.Join(skillDir, "dangling")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	oldPristine, err := store.HashTree(seedTempFile(t, "# original"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.New(home.StoreDir).SetPristine(target, coordS, oldPristine); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

// Uma pasta que o hash não consegue ler não é uma pasta inexistente: é algo que
// o ray não gravou. Sem --force, o update a preserva, em vez de a tratar como
// primeira instalação e apagar a edição.
func TestRunPreservesEditedComponentWhoseHashCannotBeComputed(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	skillDir := seedEditedComponentWithDanglingSymlink(t, home, target)

	sum, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, NoGlobal: true}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md")); string(got) != "# my local edit" {
		t.Errorf("SKILL.md = %q, want the local edit to survive", got)
	}
	if _, err := os.Lstat(filepath.Join(skillDir, "dangling")); err != nil {
		t.Errorf("the symlink was removed (%v), want the folder untouched", err)
	}
	if !slices.Contains(sum.Skipped, coordS) {
		t.Errorf("Skipped = %v, want it to include %q", sum.Skipped, coordS)
	}
	if len(sum.Warnings) == 0 {
		t.Error("Warnings is empty, want a warning explaining why it was kept")
	}
}

func TestRunForceOverwritesComponentWhoseHashCannotBeComputed(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	skillDir := seedEditedComponentWithDanglingSymlink(t, home, target)

	if _, err := Run(&runner.FakeRunner{}, cleanGitCheck(), Options{Target: target, Force: true, NoGlobal: true}, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md")); string(got) != "# new upstream" {
		t.Errorf("SKILL.md = %q, want the upstream content after --force", got)
	}
}

func TestRunToolUpgradeFailureCarriesItsReason(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"uv tool upgrade headroom-ai": {ExitCode: 1, Stderr: "error: network unreachable\n"},
	}}

	sum, err := Run(fr, cleanGitCheck(), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !sum.HadFailure || !slices.Contains(sum.Failed, "uv tool upgrade headroom-ai") {
		t.Errorf("HadFailure = %v, Failed = %v; want the upgrade reported as failed", sum.HadFailure, sum.Failed)
	}
	if !slices.ContainsFunc(sum.Warnings, func(w string) bool {
		return strings.Contains(w, "uv tool upgrade headroom-ai") && strings.Contains(w, "exit 1: error: network unreachable")
	}) {
		t.Errorf("Warnings = %v, want the failed command with its exit code and stderr", sum.Warnings)
	}
}

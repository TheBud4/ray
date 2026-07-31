package learn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

func milestones() []profile.Milestone {
	return []profile.Milestone{
		{Goal: "Skeleton compiles", Verify: "true"},
		{Goal: "Tests are green", Verify: "true"},
	}
}

// ---- Current (pure) ------------------------------------------------------

func TestCurrentNonePassed(t *testing.T) {
	m, ok := Current(milestones(), nil)
	if !ok || m.Goal != "Skeleton compiles" {
		t.Fatalf("Current() = (%v, %v), want (Skeleton compiles, true)", m, ok)
	}
}

func TestCurrentSomePassed(t *testing.T) {
	m, ok := Current(milestones(), []string{"Skeleton compiles"})
	if !ok || m.Goal != "Tests are green" {
		t.Fatalf("Current() = (%v, %v), want (Tests are green, true)", m, ok)
	}
}

func TestCurrentAllPassed(t *testing.T) {
	_, ok := Current(milestones(), []string{"Skeleton compiles", "Tests are green"})
	if ok {
		t.Fatal("Current() ok = true, want false when all milestones passed")
	}
}

func TestCurrentNoMilestones(t *testing.T) {
	_, ok := Current(nil, nil)
	if ok {
		t.Fatal("Current() ok = true, want false with no milestones defined")
	}
}

// ---- Check ----------------------------------------------------------------

func TestCheckNothingToCheckWithoutMilestones(t *testing.T) {
	target := t.TempDir()
	_, ok, err := Check(&runner.FakeRunner{}, target, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() ok = true, want false with no milestones")
	}
}

func TestCheckPassRecordsStateAndProgress(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"true": {ExitCode: 0},
	}}

	res, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true")
	}
	if !res.Passed || res.Milestone.Goal != "Skeleton compiles" {
		t.Fatalf("Check() result = %+v, want Passed=true for Skeleton compiles", res)
	}

	passed, err := loadPassed(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(passed) != 1 || passed[0] != "Skeleton compiles" {
		t.Errorf("passed state = %v, want [Skeleton compiles]", passed)
	}

	progressData, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatalf("stat progress: %v", err)
	}
	if !strings.Contains(string(progressData), "1/2") || !strings.Contains(string(progressData), "Tests are green") {
		t.Errorf("progress = %q, want it to show 1/2 passed and the next goal", progressData)
	}
}

// TestCheckProgressCountsIntersectionAfterRenegotiation cobre a correção 1a:
// o numerador do progresso é a interseção entre a lista corrente de
// milestones e o histórico de passed, não len(passed). Receita original A+B
// foi cruzada por inteiro (passed = [A, B]); a sessão renegocia para C+D;
// cruzar C deve mostrar "1/2", nunca "3/2" (len(passed)+1 sobre len([C,D])).
func TestCheckProgressCountsIntersectionAfterRenegotiation(t *testing.T) {
	target := t.TempDir()
	if err := savePassed(target, []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}

	renegotiated := []profile.Milestone{
		{Goal: "C", Verify: "true"},
		{Goal: "D", Verify: "true"},
	}
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 0}}}

	res, ok, err := Check(fr, target, renegotiated)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok || !res.Passed || res.Milestone.Goal != "C" {
		t.Fatalf("Check() = (%+v, %v), want C passar", res, ok)
	}

	progress, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(progress), "1/2") {
		t.Errorf("progress = %q, want \"1/2\" (intersection with the current list, not len(passed)=3 over len(milestones)=2)", progress)
	}
	if strings.Contains(string(progress), "3/2") {
		t.Errorf("progress = %q, contains the \"3/2\" bug (history used as numerator)", progress)
	}
}

// TestCheckReRendersProgressAfterRenegotiationWithoutApproval cobre a
// correção 1b: mesmo quando o check reprova, o progresso é re-renderizado
// contra a lista corrente de milestones — senão "Next:" continuaria
// anunciando o marco da lista velha até o próximo marco cruzar.
func TestCheckReRendersProgressAfterRenegotiationWithoutApproval(t *testing.T) {
	target := t.TempDir()
	old := []profile.Milestone{{Goal: "Old goal", Verify: "true"}}
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 0}}}
	if _, _, err := Check(fr, target, old); err != nil {
		t.Fatal(err)
	}
	progressBefore, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(progressBefore), "All milestones complete") {
		t.Fatalf("progress before = %q, want \"All milestones complete\" (setup)", progressBefore)
	}

	renegotiated := []profile.Milestone{
		{Goal: "New goal", Verify: "false"},
	}
	frFail := &runner.FakeRunner{Results: map[string]runner.Result{"false": {ExitCode: 1, Stderr: "boom"}}}
	if _, _, err := Check(frFail, target, renegotiated); err != nil {
		t.Fatal(err)
	}

	progressAfter, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(progressAfter), "New goal") {
		t.Errorf("progress = %q, want it to mention the renegotiated milestone \"New goal\" even without a pass", progressAfter)
	}
	if strings.Contains(string(progressAfter), "All milestones complete") {
		t.Errorf("progress = %q, still shows the stale state (\"All milestones complete\") after renegotiation", progressAfter)
	}
}

func TestCheckNeverTouchesTheJournal(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}

	// O diário é da IA: um contrato escrito por ela precisa sobreviver a
	// quantas passagens de marco vierem.
	journal := "## Combinado\n- Degrau inicial: 2\n"
	if err := os.WriteFile(headPath(target), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := Check(&runner.FakeRunner{}, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true with milestones defined")
	}

	got, err := os.ReadFile(headPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != journal {
		t.Errorf("Check() rewrote the journal.\n got: %q\nwant: %q", got, journal)
	}

	progress, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatalf("progress was not written: %v", err)
	}
	if !strings.Contains(string(progress), "1/2") {
		t.Errorf("progress = %q, want it to contain \"1/2\"", progress)
	}
}

func TestCheckFailTouchesNothing(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"true": {ExitCode: 1, Stderr: "boom"},
	}}

	res, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true (there was something to check)")
	}
	if res.Passed {
		t.Fatal("Check() Passed = true, want false")
	}
	if !strings.Contains(res.Output, "boom") {
		t.Errorf("Output = %q, want it to include the failure output", res.Output)
	}

	// Reprovação não toca o diário (dono é a IA) nem o estado de máquina de
	// marcos cruzados — mas o progresso pode e deve ser re-renderizado, já
	// que ele reflete a lista corrente de milestones a cada chamada, não só
	// nas aprovações (correção 1b).
	if _, err := os.Stat(headPath(target)); !os.IsNotExist(err) {
		t.Error("journal must not exist after a failed check: it belongs to the assistant, ray never writes it")
	}
	if _, err := os.Stat(PassedPath(target)); !os.IsNotExist(err) {
		t.Error("milestones-passed.yaml must not exist after a failed check: no milestone was crossed")
	}
	progress, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatalf("progress must be re-rendered even on a failed check: %v", err)
	}
	if !strings.Contains(string(progress), "0/2") {
		t.Errorf("progress = %q, want it to contain \"0/2\"", progress)
	}
}

func TestCheckNothingLeftAfterAllPassed(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 0}}}

	if _, _, err := Check(fr, target, milestones()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Check(fr, target, milestones()); err != nil {
		t.Fatal(err)
	}

	_, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() ok = true, want false once all milestones have passed")
	}

	progressData, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(progressData), "All milestones complete") {
		t.Errorf("progress = %q, want \"All milestones complete\"", progressData)
	}
}

func TestCheckRunnerErrorSurfacesWithoutStateChange(t *testing.T) {
	target := t.TempDir()
	res, ok, err := Check(errRunner{}, target, milestones())
	if err == nil {
		t.Fatal("Check() error = nil, want the runner's error surfaced")
	}
	if !ok {
		t.Error("Check() ok = false, want true: there was a milestone to check, it just errored")
	}
	_ = res
	if _, statErr := os.Stat(JournalDir(target)); !os.IsNotExist(statErr) {
		t.Error("journal dir should not exist after a runner error")
	}
}

type errRunner struct{}

func (errRunner) Run(context.Context, runner.Command) (runner.Result, error) {
	return runner.Result{}, os.ErrPermission
}

func TestLoadMilestonesFallsBackToRecipe(t *testing.T) {
	target := t.TempDir()

	got, err := LoadMilestones(target, milestones())
	if err != nil {
		t.Fatalf("LoadMilestones() error = %v", err)
	}
	if len(got) != 2 || got[0].Goal != "Skeleton compiles" {
		t.Errorf("LoadMilestones() = %v, want os marcos da receita", got)
	}
}

func TestLoadMilestonesPrefersLocal(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	local := "milestones:\n  - goal: \"API responde GET /tasks\"\n    verify: \"true\"\n"
	if err := os.WriteFile(LocalMilestonesPath(target), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadMilestones(target, milestones())
	if err != nil {
		t.Fatalf("LoadMilestones() error = %v", err)
	}
	if len(got) != 1 || got[0].Goal != "API responde GET /tasks" {
		t.Errorf("LoadMilestones() = %v, want the locally negotiated milestone", got)
	}
}

func TestLoadMilestonesRejectsBrokenLocalFile(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LocalMilestonesPath(target), []byte("isto: [não é: yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cair calado na receita esconderia o arquivo quebrado do aluno, que foi
	// quem pediu para gravá-lo.
	if _, err := LoadMilestones(target, milestones()); err == nil {
		t.Fatal("LoadMilestones() error = nil, want a parse error")
	}
}

func TestLoadMilestonesRejectsEmptyFile(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LocalMilestonesPath(target), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMilestones(target, milestones())
	if err == nil {
		t.Fatal("LoadMilestones() error = nil, want an error: an empty file must not silently become []")
	}
	if !strings.Contains(err.Error(), LocalMilestonesPath(target)) {
		t.Errorf("error = %q, want it to cite %q", err, LocalMilestonesPath(target))
	}
}

func TestLoadMilestonesRejectsMilestonesKeyWithoutItems(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LocalMilestonesPath(target), []byte("milestones:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMilestones(target, milestones())
	if err == nil {
		t.Fatal("LoadMilestones() error = nil, want an error: milestones key with no items")
	}
	if !strings.Contains(err.Error(), LocalMilestonesPath(target)) {
		t.Errorf("error = %q, want it to cite %q", err, LocalMilestonesPath(target))
	}
}

func TestLoadMilestonesRejectsItemWithoutGoal(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	local := "milestones:\n  - verify: \"true\"\n"
	if err := os.WriteFile(LocalMilestonesPath(target), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMilestones(target, milestones())
	if err == nil {
		t.Fatal("LoadMilestones() error = nil, want an error: an item without goal would become \"\" e contaria como cruzado para sempre")
	}
	if !strings.Contains(err.Error(), LocalMilestonesPath(target)) {
		t.Errorf("error = %q, want it to cite %q", err, LocalMilestonesPath(target))
	}
}

func TestLoadMilestonesRejectsItemWithoutVerify(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	local := "milestones:\n  - goal: \"API responde GET /tasks\"\n"
	if err := os.WriteFile(LocalMilestonesPath(target), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadMilestones(target, milestones())
	if err == nil {
		t.Fatal("LoadMilestones() error = nil, want an error instead of blowing up later at check time")
	}
	if !strings.Contains(err.Error(), LocalMilestonesPath(target)) {
		t.Errorf("error = %q, want it to cite %q", err, LocalMilestonesPath(target))
	}
}

func TestPathHelpers(t *testing.T) {
	target := "/proj"
	if got := JournalDir(target); got != filepath.Join(target, ".claude", ".local") {
		t.Errorf("JournalDir() = %q", got)
	}
	if got := headPath(target); got != filepath.Join(JournalDir(target), "learning-journal.md") {
		t.Errorf("headPath() = %q", got)
	}
	if got := MilestonesProgressPath(target); got != filepath.Join(JournalDir(target), "milestones-progress.md") {
		t.Errorf("MilestonesProgressPath() = %q", got)
	}
	if got := PassedPath(target); got != filepath.Join(JournalDir(target), "milestones-passed.yaml") {
		t.Errorf("PassedPath() = %q", got)
	}
	if got := LocalMilestonesPath(target); got != filepath.Join(JournalDir(target), "milestones.yaml") {
		t.Errorf("LocalMilestonesPath() = %q", got)
	}
}

// O progresso é injetado no início de toda sessão, então ele não pode ficar
// para trás quando o verify não chega a executar (binário ausente, permissão).
// Antes, só a reprovação por exit code re-renderizava: depois de renegociar os
// marcos, um erro de execução deixava o "Next:" anunciando o marco da lista
// anterior até alguma chamada conseguir rodar.
func TestCheckRerendersProgressWhenVerifyCannotRun(t *testing.T) {
	target := t.TempDir()

	antigos := []profile.Milestone{{Goal: "marco antigo", Verify: "true"}}
	if _, _, err := Check(&runner.FakeRunner{}, target, antigos); err != nil {
		t.Fatalf("Check() inicial: %v", err)
	}

	// Renegociação: a lista corrente troca inteira.
	novos := []profile.Milestone{{Goal: "marco novo", Verify: "binario-que-nao-existe"}}
	falha := &runner.FakeRunner{Err: errors.New("exec: binário não encontrado")}
	if _, _, err := Check(falha, target, novos); err == nil {
		t.Fatal("Check() = nil error, want the runner execution error")
	}

	got, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "marco novo") {
		t.Errorf("progress did not follow the renegotiation: %q", got)
	}
	if strings.Contains(string(got), "marco antigo") {
		t.Errorf("progress still announces the milestone from the previous list: %q", got)
	}
}

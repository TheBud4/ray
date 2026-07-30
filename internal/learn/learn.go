// Package learn é a máquina verificável do modo learn (design §9.2–§9.3,
// I6a): roda o `verify` do marco corrente de uma receita e escreve o
// progresso de marcos — o único arquivo do diário que o `ray` escreve. O
// diário de aprendizado em si (head injetado no início de cada sessão) é
// da IA: o `ray` nunca escreve nele. O diário é pessoal: vive em
// <target>/.claude/.local/, nunca vendorizado (I1 já ignora esse diretório
// no .gitignore). O conteúdo de ensino (prompt socrático, escada de dicas)
// é I6b — este pacote só fornece a máquina, não autora currículo.
package learn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// JournalDir é <target>/.claude/.local, onde vive todo estado do diário.
func JournalDir(target string) string {
	return filepath.Join(target, ".claude", ".local")
}

// HeadPath é o diário de aprendizado — dono é a IA, o ray nunca escreve nele.
// O hook de sessão injeta este arquivo junto com o milestones-progress.md.
func HeadPath(target string) string {
	return filepath.Join(JournalDir(target), "learning-journal.md")
}

// MilestonesProgressPath é o progresso renderizado pelo ray — dono é o ray,
// e é o único arquivo do diário que ele escreve. Fica separado do head
// porque head e progresso têm donos diferentes, e dois donos truncando o
// mesmo arquivo foi o que apagava o diário a cada marco.
func MilestonesProgressPath(target string) string {
	return filepath.Join(JournalDir(target), "milestones-progress.md")
}

// PassedPath é o estado de máquina (não o log legível): a lista de goals já
// cruzados.
func PassedPath(target string) string {
	return filepath.Join(JournalDir(target), "milestones-passed.yaml")
}

// LocalMilestonesPath é onde vivem os marcos negociados na sessão. A IA
// escreve, o ray só lê — o inverso do milestones-passed.yaml.
func LocalMilestonesPath(target string) string {
	return filepath.Join(JournalDir(target), "milestones.yaml")
}

type localMilestones struct {
	Milestones []profile.Milestone `yaml:"milestones"`
}

// LoadMilestones resolve de onde vêm os marcos: os negociados na sessão
// ganham dos da receita, porque descrevem o que este aluno combinou construir
// neste projeto. Sem arquivo local, a receita vale. Arquivo local ilegível,
// vazio ou com item inválido é erro, não fallback silencioso — quem escreve
// esse YAML é a IA, não um humano revisando, então vazio ou item incompleto
// são os modos de falha esperados, não exceções.
func LoadMilestones(target string, recipe []profile.Milestone) ([]profile.Milestone, error) {
	path := LocalMilestonesPath(target)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return recipe, nil
		}
		return nil, err
	}
	var local localMilestones
	if err := yaml.Unmarshal(data, &local); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(local.Milestones) == 0 {
		return nil, fmt.Errorf("%s exists but defines no milestones (missing 'milestones:' key, or the key is empty)", path)
	}
	// Reaproveita profile.Profile.Validate — que por sua vez chama
	// Milestone.validate() em cada item — em vez de duplicar a checagem de
	// goal/verify aqui. Milestone.validate não é exportado, mas
	// Profile.Validate já o invoca por dentro; Name é só um valor descartável
	// pra não disparar o checável de nome de receita antes de chegar aos
	// marcos.
	p := profile.Profile{Name: "local milestones", Milestones: local.Milestones}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return local.Milestones, nil
}

type passedState struct {
	Passed []string `yaml:"passed"`
}

func loadPassed(target string) ([]string, error) {
	var s passedState
	data, err := os.ReadFile(PassedPath(target))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Passed, nil
}

func savePassed(target string, passed []string) error {
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(passedState{Passed: passed})
	if err != nil {
		return err
	}
	return os.WriteFile(PassedPath(target), data, 0o644)
}

// Current devolve o primeiro marco de milestones cujo Goal ainda não está em
// passed. ok=false se não há marcos ou todos já passaram.
func Current(milestones []profile.Milestone, passed []string) (profile.Milestone, bool) {
	passedSet := make(map[string]bool, len(passed))
	for _, p := range passed {
		passedSet[p] = true
	}
	for _, m := range milestones {
		if !passedSet[m.Goal] {
			return m, true
		}
	}
	return profile.Milestone{}, false
}

// Result é o desfecho de checar um marco.
type Result struct {
	Milestone profile.Milestone
	Passed    bool
	Output    string // stdout+stderr do verify, útil pra diagnosticar falha
}

// Check roda o verify do marco corrente de target via r. ok=false significa
// que não havia nada a checar (nenhum marco definido, ou todos já
// passaram) — não é erro. Em caso de aprovação, grava o estado de máquina e
// reescreve o progresso de marcos — nunca o diário, que é da IA. Em caso de
// reprovação, nenhum arquivo é tocado — design §9.2 exclui blow-by-blow de
// tentativas do diário; só deltas de entendimento entram lá.
func Check(r runner.Runner, target string, milestones []profile.Milestone) (Result, bool, error) {
	passed, err := loadPassed(target)
	if err != nil {
		return Result{}, false, err
	}

	m, ok := Current(milestones, passed)
	if !ok {
		return Result{}, false, nil
	}

	fields := strings.Fields(m.Verify)
	if len(fields) == 0 {
		return Result{}, true, fmt.Errorf("milestone %q has an empty verify command", m.Goal)
	}
	res, err := r.Run(context.Background(), runner.Command{Name: fields[0], Args: fields[1:], Dir: target})
	if err != nil {
		return Result{Milestone: m}, true, err
	}
	output := res.Stdout + res.Stderr
	if res.ExitCode != 0 {
		return Result{Milestone: m, Passed: false, Output: output}, true, nil
	}

	passed = append(passed, m.Goal)
	if err := savePassed(target, passed); err != nil {
		return Result{}, true, err
	}
	if err := writeMilestones(target, milestones, passed); err != nil {
		return Result{}, true, err
	}

	return Result{Milestone: m, Passed: true, Output: output}, true, nil
}

// writeMilestones reescreve o progresso de marcos — o único arquivo do
// diário que o ray escreve.
func writeMilestones(target string, milestones []profile.Milestone, passed []string) error {
	next, ok := Current(milestones, passed)
	var body string
	if !ok {
		body = "All milestones complete.\n"
	} else {
		body = fmt.Sprintf("Milestones passed: %d/%d\nNext: %s\n", len(passed), len(milestones), next.Goal)
	}
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(MilestonesProgressPath(target), []byte(body), 0o644)
}

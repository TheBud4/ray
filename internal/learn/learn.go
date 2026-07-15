// Package learn é a máquina verificável do modo learn (design §9.2–§9.3,
// I6a): roda o `verify` do marco corrente de uma receita e mantém o diário
// de aprendizado — um head pequeno e sempre reescrito (o único trecho que o
// hook de sessão injeta) mais um log append-only (nunca injetado, só para o
// aluno reler). O diário é pessoal: vive em <target>/.claude/.local/,
// nunca vendorizado (I1 já ignora esse diretório no .gitignore). O
// conteúdo de ensino (prompt socrático, escada de dicas) é I6b — este
// pacote só fornece a máquina, não autora currículo.
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

// HeadPath é o head vivo do diário — sempre reescrito, o único trecho que o
// hook de sessão injeta.
func HeadPath(target string) string {
	return filepath.Join(JournalDir(target), "learning-journal.md")
}

// LogPath é o log append-only — não é injetado, só para o aluno reler.
func LogPath(target string) string {
	return filepath.Join(JournalDir(target), "learning-journal-log.md")
}

// PassedPath é o estado de máquina (não o log legível): a lista de goals já
// cruzados.
func PassedPath(target string) string {
	return filepath.Join(JournalDir(target), "milestones-passed.yaml")
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
// passaram) — não é erro. Em caso de aprovação, grava o estado de máquina,
// acrescenta uma linha ao log e reescreve o head. Em caso de reprovação,
// nenhum arquivo é tocado — design §9.2 exclui blow-by-blow de tentativas do
// diário; só deltas de entendimento entram lá.
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
	if err := appendLog(target, m.Goal); err != nil {
		return Result{}, true, err
	}
	if err := writeHead(target, milestones, passed); err != nil {
		return Result{}, true, err
	}

	return Result{Milestone: m, Passed: true, Output: output}, true, nil
}

func appendLog(target, goal string) error {
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(LogPath(target), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("- Milestone passed: %s\n", goal))
	return err
}

func writeHead(target string, milestones []profile.Milestone, passed []string) error {
	next, ok := Current(milestones, passed)
	var head string
	if !ok {
		head = "All milestones complete.\n"
	} else {
		head = fmt.Sprintf("Milestones passed: %d/%d\nNext: %s\n", len(passed), len(milestones), next.Goal)
	}
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(HeadPath(target), []byte(head), 0o644)
}

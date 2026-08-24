// Package update implementa `ray update` (I3): atualiza ferramentas (latest)
// e recopia cada componente de internal/raypaths.ComponentsDir (nunca da
// rede), protegendo edições por conteúdo (não por git-status) via o hash
// pristino gravado pelo internal/initai em internal/store.
package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

// Home reúne os caminhos de ~/.ray que Run precisa, resolvidos pelo chamador
// (internal/raypaths) — mesmo padrão de initai.Home.
type Home struct {
	ProfilesDir   string
	StoreDir      string
	ComponentsDir string
}

// Options são os parâmetros de `ray update`.
type Options struct {
	// Profile sobrescreve o registro project-local (.claude/.ray-profile).
	// Vazio = ler o registro.
	Profile string
	Target  string
	Force   bool
	// NoGlobal pula os passos que mexem na máquina inteira (upgrade das
	// ferramentas uv), deixando só o que é do projeto-alvo. Mesmo recorte que
	// o flag homônimo de `ray new` e `ray init ai`: atualizar um projeto não
	// deveria ser a única forma de subir a versão global de uma ferramenta.
	NoGlobal bool
	DryRun   bool
	Out      io.Writer
}

// Summary é o resultado de Run.
type Summary struct {
	Updated    []string
	Skipped    []string
	Failed     []string
	Warnings   []string
	HadFailure bool
}

// Run executa `ray update`. r executa ações efetivas (ferramentas, cópia de
// conteúdo) e respeita --dry-run na fiação real; check só consulta o estado
// da árvore git e deve continuar real mesmo sob --dry-run (mesmo raciocínio
// do preflight em `ray init ai`: um guard não deve virar teatro).
func Run(r runner.Runner, check runner.Runner, opts Options, home Home) (Summary, error) {
	var sum Summary
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return Summary{}, err
	}

	// 1. resolve o perfil: --profile ganha; senão lê o registro project-local.
	prof, err := profile.LoadForTarget(home.ProfilesDir, target, opts.Profile)
	if err != nil {
		return Summary{}, err
	}

	// 2. guard de árvore limpa (ortogonal) — mantém o diff do update legível.
	// Se não der para checar (não é repo git, git ausente), segue sem bloquear.
	//
	// O --dry-run passa: o guard protege a legibilidade de um diff, e uma
	// simulação não produz diff nenhum. Barrá-la só empurrava a pessoa para o
	// --force, que é exatamente o que o guard quer evitar que vire hábito.
	if !opts.Force && !opts.DryRun {
		dirty, gerr := isTreeDirty(check, target)
		if gerr == nil && dirty {
			return Summary{}, fmt.Errorf("%s has uncommitted changes; commit/stash or pass --force so the update diff stays legible", target)
		}
	}

	// 3. ferramentas (latest) — só as que a receita liga, e só se o --no-global
	// não tiver recortado a máquina para fora deste update.
	if !opts.NoGlobal {
		for _, cmd := range toolUpgradeCommands(prof.Integrations) {
			if runOne(r, cmd) {
				sum.Updated = append(sum.Updated, cmd.String())
			} else {
				sum.Failed = append(sum.Failed, cmd.String())
			}
		}
	}

	// 4. conteúdo — recópia local (nunca rede), protegida por fork (por
	// componente).
	st := store.New(home.StoreDir)
	for _, c := range prof.Components {
		srcDir := filepath.Join(home.ComponentsDir, c.Name)
		if info, statErr := os.Stat(srcDir); statErr != nil || !info.IsDir() {
			sum.Skipped = append(sum.Skipped, fmt.Sprintf("%s: not found at %s", c.Name, srcDir))
			continue
		}
		onDisk := filepath.Join(target, c.Dest, c.Name)

		freshHash, herr := store.HashTree(srcDir)
		if herr != nil {
			return Summary{}, herr
		}
		onDiskHash, onDiskErr := store.HashTree(onDisk)
		pristineHash, hasPristine := st.PristineHash(target, c.Name)
		overwrite, reason := decideOverwrite(opts.Force, onDiskErr == nil, onDiskHash, freshHash, pristineHash, hasPristine)

		if opts.DryRun {
			if !overwrite {
				fmt.Fprintf(out, "+ preserve %s (edited locally)\n", c.Name)
				sum.Skipped = append(sum.Skipped, c.Name)
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %s", c.Name, reason))
				continue
			}
			fmt.Fprintf(out, "+ re-copy %s -> %s\n", c.Name, onDisk)
			sum.Updated = append(sum.Updated, c.Name)
			continue
		}

		if !overwrite {
			sum.Skipped = append(sum.Skipped, c.Name)
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %s", c.Name, reason))
			continue
		}

		if err := os.RemoveAll(onDisk); err != nil && !os.IsNotExist(err) {
			return Summary{}, err
		}
		if err := store.CopyTree(srcDir, onDisk); err != nil {
			sum.Failed = append(sum.Failed, c.Name)
			continue
		}
		if err := st.SetPristine(target, c.Name, freshHash); err != nil {
			return Summary{}, err
		}
		sum.Updated = append(sum.Updated, c.Name)
	}

	sum.HadFailure = len(sum.Failed) > 0
	return sum, nil
}

// decideOverwrite delega para store.DecideOverwrite. A política mora no
// `store` porque é sobre linha-base pristina, que é o que o `store` guarda —
// e porque o overlay de templates (`scaffold.EnsureTemplates`) precisa da
// mesma decisão sem depender deste pacote.
func decideOverwrite(force, onDiskExists bool, onDiskHash, freshHash, pristineHash string, hasPristine bool) (overwrite bool, reason string) {
	return store.DecideOverwrite(force, onDiskExists, onDiskHash, freshHash, pristineHash, hasPristine)
}

// isTreeDirty roda `git status --porcelain` em target via check. Devolve
// erro se não deu para checar (não é repo git, git ausente) — Run trata isso
// como "não bloqueia" (soft).
func isTreeDirty(check runner.Runner, target string) (bool, error) {
	res, err := check.Run(context.Background(), runner.Command{Name: "git", Args: []string{"status", "--porcelain"}, Dir: target})
	if err != nil {
		return false, err
	}
	if res.ExitCode != 0 {
		return false, fmt.Errorf("git status exited %d", res.ExitCode)
	}
	return strings.TrimSpace(res.Stdout) != "", nil
}

// toolUpgradeCommands espelha economy.Headroom/economy.CodeGraph, trocando
// install por upgrade — só as integrações com uma ferramenta uv global
// entram aqui (as demais são conteúdo, tratado no passo 4).
func toolUpgradeCommands(in profile.Integrations) []runner.Command {
	var cmds []runner.Command
	if in.Headroom {
		cmds = append(cmds, runner.Command{Name: "uv", Args: []string{"tool", "upgrade", "headroom-ai"}})
	}
	if in.CodeGraph {
		cmds = append(cmds, runner.Command{Name: "uv", Args: []string{"tool", "upgrade", "graphifyy"}})
	}
	return cmds
}

// runOne roda c via r e classifica o resultado: err ou ExitCode != 0 → false.
func runOne(r runner.Runner, c runner.Command) bool {
	res, err := r.Run(context.Background(), c)
	if err != nil {
		return false
	}
	return res.ExitCode == 0
}

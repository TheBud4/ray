// Package update implementa `ray update` (I3): atualiza ferramentas (latest)
// e recopia cada componente de internal/raypaths.ComponentsDir (nunca da
// rede), protegendo edições por conteúdo (não por git-status) via o hash
// pristino gravado pelo internal/initai em internal/store.
package update

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/safepath"
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
	Tools      []string
	Updated    []string
	Unchanged  []string
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

	// Um clone pode trazer .claude, o registro do perfil, a linha-base ou o
	// destino de um componente como symlink para fora do projeto. Ler ou gravar
	// por eles (e o --force apaga) alcançaria fora, então recusa antes de ler
	// qualquer coisa do projeto e de qualquer efeito.
	baselinePath := store.ProjectBaseline(target).Path()
	if err := checkInside(target, profile.ProfileRecordPath(target), baselinePath); err != nil {
		return Summary{}, err
	}

	// 1. resolve o perfil: --profile ganha; senão lê o registro project-local.
	prof, err := profile.LoadForTarget(home.ProfilesDir, target, opts.Profile)
	if err != nil {
		return Summary{}, err
	}

	dests := make([]string, 0, len(prof.Components))
	for _, c := range prof.Components {
		dests = append(dests, filepath.Join(target, c.Dest, c.Name))
	}
	if err := checkInside(target, dests...); err != nil {
		return Summary{}, err
	}
	for _, d := range dests {
		if err := checkTreeInside(target, d); err != nil {
			return Summary{}, err
		}
	}

	// A origem dos componentes também é conferida antes de qualquer efeito: um
	// link dentro de um componente que leve para fora dele seria copiado como
	// arquivo comum para dentro do projeto.
	compNames := make([]string, len(prof.Components))
	for i, c := range prof.Components {
		compNames[i] = c.Name
	}
	sources, err := store.ResolveSources(home.ComponentsDir, compNames)
	if err != nil {
		return Summary{}, err
	}

	// A linha-base é o que separa "intocado" de "editado": ilegível, nada abaixo
	// pode decidir sobre sobrescrita, então para antes de qualquer efeito.
	baseline := store.ProjectBaseline(target).WithLegacy(store.New(home.StoreDir))
	if err := baseline.Verify(); err != nil {
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
			if ok, reason := runOne(r, cmd); ok {
				sum.Tools = append(sum.Tools, cmd.String())
			} else {
				sum.Failed = append(sum.Failed, cmd.String())
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("`%s`: %s", cmd.String(), reason))
			}
		}
	}

	// 4. conteúdo — recópia local (nunca rede), protegida por fork (por
	// componente).
	for _, c := range prof.Components {
		srcDir := filepath.Join(home.ComponentsDir, c.Name)
		if info, statErr := os.Stat(srcDir); statErr != nil || !info.IsDir() {
			sum.Skipped = append(sum.Skipped, fmt.Sprintf("%s: not found at %s", c.Name, srcDir))
			continue
		}
		onDisk := filepath.Join(target, c.Dest, c.Name)

		freshHash, herr := store.HashTree(sources[c.Name])
		if herr != nil {
			return Summary{}, herr
		}
		onDiskHash, onDiskExists := store.LocalState(onDisk)
		pristineHash, hasPristine := baseline.PristineHash(c.Name)

		// Já é igual ao do upstream: não há o que recopiar. Só a linha-base
		// pode estar defasada (clone novo, ou edição que coincidiu com o
		// upstream) — e ela é o que o próximo update e o `ray status` leem.
		if onDiskExists && onDiskHash != "" && onDiskHash == freshHash {
			if opts.DryRun {
				fmt.Fprintf(out, "+ unchanged %s\n", c.Name)
			} else if !hasPristine || pristineHash != freshHash {
				if err := baseline.SetPristine(c.Name, freshHash); err != nil {
					return Summary{}, err
				}
			}
			sum.Unchanged = append(sum.Unchanged, c.Name)
			continue
		}

		overwrite, reason := decideOverwrite(opts.Force, onDiskExists, onDiskHash, freshHash, pristineHash, hasPristine)

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
		if err := store.CopyTree(sources[c.Name], onDisk); err != nil {
			sum.Failed = append(sum.Failed, c.Name)
			continue
		}
		if err := baseline.SetPristine(c.Name, freshHash); err != nil {
			return Summary{}, err
		}
		sum.Updated = append(sum.Updated, c.Name)
	}

	// Projeto montado antes de a linha-base ir para o projeto: leva para o arquivo
	// dele o que o store da máquina tem, mesmo que nenhum componente tenha mudado.
	if !opts.DryRun {
		if err := baseline.Promote(); err != nil {
			return Summary{}, err
		}
	}

	sum.HadFailure = len(sum.Failed) > 0
	return sum, nil
}

// checkInside recusa o update se algum dos caminhos, seguindo os symlinks que
// existem no meio dele, sai de target. Um symlink que resolve dentro do
// projeto continua valendo.
func checkInside(target string, paths ...string) error {
	for _, p := range paths {
		if err := safepath.ResolveInside(target, p); err != nil {
			return fmt.Errorf("refusing to update: %w", err)
		}
	}
	return nil
}

// checkTreeInside faz o mesmo para os symlinks que moram dentro de dir: a
// recópia apaga e regrava a árvore inteira, então um arquivo ou subpasta dela
// apontando para fora também é recusado. Um dir que não existe não tem o que
// conferir.
func checkTreeInside(target, dir string) error {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		return checkInside(target, p)
	})
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

// runOne roda c via r e classifica o resultado: err ou ExitCode != 0 → ok
// false, com o motivo (runner.FailureReason) para quem quiser mostrá-lo.
func runOne(r runner.Runner, c runner.Command) (ok bool, reason string) {
	res, err := r.Run(context.Background(), c)
	if err != nil || res.ExitCode != 0 {
		return false, runner.FailureReason(res, err)
	}
	return true, ""
}

// Package update implementa `ray update` (I3): atualiza ferramentas (latest)
// e re-adquire conteúdo pelo Acquirer de cada componente, protegendo edições
// por conteúdo (não por git-status) via o hash pristino gravado pelo
// internal/initai em internal/store.
package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/acquire"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

// Home reúne os caminhos de ~/.ray que Run precisa, resolvidos pelo chamador
// (internal/raypaths) — mesmo padrão de initai.Home.
type Home struct {
	ProfilesDir string
	StoreDir    string
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

	// 4. conteúdo — re-aquisição por ref, protegida por fork (por componente).
	st := store.New(home.StoreDir)
	for _, c := range prof.Components {
		acq, ok := acquire.For(c, r)
		if !ok {
			// Inalcançável por receita carregada desde a recusa de `type: mcp`
			// em profile.Validate. Fica como defesa: se o invariante quebrar, o
			// componente aparece no resumo em vez de sumir — que era o defeito
			// original.
			//
			// O identificador sai dos campos do componente: acq é nil aqui, e
			// acq.Key(c) — a forma usada nos outros skips — seria panic.
			sum.Skipped = append(sum.Skipped, describeUnacquirable(c))
			continue
		}
		coord := acq.Key(c)
		leaf, lerr := acquire.LeafName(c)
		if lerr != nil {
			return Summary{}, lerr
		}
		destRel, derr := acquire.DestRel(c)
		if derr != nil {
			return Summary{}, derr
		}
		onDisk := filepath.Join(target, destRel, leaf)

		if c.Via == profile.ViaGit && c.Ref != "" && c.Ref != "main" {
			sum.Skipped = append(sum.Skipped, coord+" (pinned ref, no-op)")
			continue
		}

		if opts.DryRun {
			// O dry-run decide o que dá para decidir sem rede. Com linha-base
			// gravada o veredito sai de dois hashes locais e é exato — é o
			// caso normal depois do init ai, e é o que impede a simulação de
			// anunciar que vai sobrescrever o que a execução real preserva.
			//
			// Sem linha-base, o ramo de DecideOverwrite que decidiria precisa
			// do upstream, e buscá-lo aqui quebraria o "dry-run não busca
			// nada". Afirmar sem buscar seria inventar: vira aviso.
			onDiskHash, onDiskErr := store.HashTree(onDisk)
			pristineHash, hasPristine := st.PristineHash(target, coord)

			if hasPristine && onDiskErr == nil {
				// freshHash vazio de propósito: com hasPristine, o
				// DecideOverwrite não o consulta. É o que torna a decisão
				// offline, e não descuido.
				overwrite, reason := decideOverwrite(opts.Force, true, onDiskHash, "", pristineHash, true)
				if !overwrite {
					fmt.Fprintf(out, "+ preserve %s (edited locally)\n", coord)
					sum.Skipped = append(sum.Skipped, coord)
					sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %s", coord, reason))
					continue
				}
			} else if onDiskErr == nil {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf(
					"%s: no pristine baseline — whether this is a local edit needs the upstream, which a dry-run does not fetch", coord))
			}

			fmt.Fprintf(out, "+ re-acquire %s -> %s\n", coord, destRel)
			sum.Updated = append(sum.Updated, coord)
			continue
		}

		res, aerr := acq.Acquire(context.Background(), c)
		if aerr != nil {
			sum.Failed = append(sum.Failed, coord)
			continue
		}
		freshDir := filepath.Join(res.Dir, leaf)
		freshHash, herr := store.HashTree(freshDir)
		if herr != nil {
			return Summary{}, herr
		}

		onDiskHash, onDiskErr := store.HashTree(onDisk)
		pristineHash, hasPristine := st.PristineHash(target, coord)

		overwrite, reason := decideOverwrite(opts.Force, onDiskErr == nil, onDiskHash, freshHash, pristineHash, hasPristine)
		if !overwrite {
			sum.Skipped = append(sum.Skipped, coord)
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %s", coord, reason))
			continue
		}

		if err := os.RemoveAll(onDisk); err != nil && !os.IsNotExist(err) {
			return Summary{}, err
		}
		if err := store.CopyTree(freshDir, onDisk); err != nil {
			sum.Failed = append(sum.Failed, coord)
			continue
		}
		if _, err := st.Put(coord, freshDir); err != nil {
			return Summary{}, err
		}
		if err := st.SetPristine(target, coord, freshHash); err != nil {
			return Summary{}, err
		}
		sum.Updated = append(sum.Updated, coord)
	}

	sum.HadFailure = len(sum.Failed) > 0
	return sum, nil
}

// describeUnacquirable nomeia um componente que nenhum adquiridor atende. Não
// usa acquire.Key porque não há adquiridor — é essa a condição.
func describeUnacquirable(c profile.Component) string {
	desc := fmt.Sprintf("no acquirer for via=%s", c.Via)
	if c.Type != "" {
		desc += fmt.Sprintf(" type=%s", c.Type)
	}
	if c.Ref != "" {
		desc += fmt.Sprintf(" (%s)", c.Ref)
	}
	return desc
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

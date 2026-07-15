// Package initai orquestra os 10 passos de `ray init ai` (build guide §8),
// ligando profile, installer, mcp, claudecfg, vault, scaffold e preflight.
package initai

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/acquire"
	"github.com/TheBud4/ray/internal/claudecfg"
	"github.com/TheBud4/ray/internal/installer"
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/vault"
)

// Home reúne os caminhos de ~/.ray usados por Run, resolvidos pelo chamador
// (via internal/raypaths) para manter este pacote livre de os.Getenv e
// testável só com t.TempDir().
type Home struct {
	ProfilesDir  string
	TemplatesDir string
	VaultDir     string
	ConfigPath   string
	StatePath    string
	StoreDir     string
}

// Options são os parâmetros de `ray init ai` (build guide §5, §8).
type Options struct {
	Profile         string
	Target          string
	Mode            string
	Global          bool
	Force           bool
	NoGlobal        bool
	ReinstallGlobal bool
	DryRun          bool
	Out             io.Writer
}

// Summary é o resultado final de Run (build guide §8).
type Summary struct {
	Installed  []string
	Failed     []string
	Created    []string
	Skipped    []string
	Warnings   []string
	HadFailure bool
}

// Run executa os 10 passos de `ray init ai`. r executa comandos de
// componentes/globais (respeita --dry-run na fiação real); l checa
// dependências e deve continuar real mesmo sob --dry-run (mesmo raciocínio
// de `ray doctor`, Fase 7: um gate de validação não deve virar teatro).
func Run(r runner.Runner, l preflight.Looker, opts Options, home Home) (Summary, error) {
	var sum Summary
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	// 1. modo + target + writability.
	if opts.Mode != scaffold.ModeBuild && opts.Mode != scaffold.ModeLearn {
		return Summary{}, fmt.Errorf("invalid --mode %q (want %q or %q)", opts.Mode, scaffold.ModeBuild, scaffold.ModeLearn)
	}
	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return Summary{}, err
	}
	if err := ensureWritableDir(target); err != nil {
		return Summary{}, fmt.Errorf("target %s is not writable: %w", target, err)
	}

	// 2. garante ~/.ray populado.
	if err := profile.EnsureDir(home.ProfilesDir); err != nil {
		return Summary{}, err
	}
	if err := scaffold.EnsureTemplates(home.TemplatesDir); err != nil {
		return Summary{}, err
	}

	// 3. carrega a receita.
	prof, err := profile.Load(filepath.Join(home.ProfilesDir, opts.Profile+".yaml"))
	if err != nil {
		return Summary{}, err
	}

	// 4. preflight — aborta antes de qualquer efeito.
	needPython := prof.Integrations.Headroom || prof.Integrations.CodeGraph
	checks := preflight.Run(l, needPython)
	if missing := preflight.MissingRequired(checks); len(missing) > 0 {
		names := make([]string, len(missing))
		for i, c := range missing {
			names[i] = c.Name
		}
		return Summary{}, fmt.Errorf("missing required dependencies: %s (run `ray doctor`)", strings.Join(names, ", "))
	}

	// 5. docs vault do usuário + resolve o plano de instalação.
	cfg, err := rayconfig.Load(home.ConfigPath)
	if err != nil {
		return Summary{}, err
	}
	docsPath := cfg.UserDocsVaultPath()
	if prof.Integrations.UserDocsVault {
		switch {
		case docsPath == "":
			sum.Warnings = append(sum.Warnings, "user_docs_vault ligado mas não configurado (rode `ray docs set` ou `ray docs init`)")
		default:
			if _, statErr := os.Stat(docsPath); statErr != nil {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("user_docs_vault configurado mas o caminho não existe: %s", docsPath))
			}
		}
	}
	plan, err := installer.Resolve(prof, installer.Options{
		Global:            opts.Global,
		VaultPath:         home.VaultDir,
		UserDocsVaultPath: docsPath,
	})
	if err != nil {
		return Summary{}, err
	}

	// 6. vault de conhecimento da IA.
	if prof.Integrations.KnowledgeVault || prof.Integrations.SecondBrain {
		if opts.DryRun {
			fmt.Fprintf(out, "+ ensure vault at %s\n", home.VaultDir)
		} else if err := vault.Ensure(home.VaultDir); err != nil {
			return Summary{}, err
		}
	}

	// 7a. globais (install-once, rastreados em state.yaml).
	state, err := rayconfig.LoadState(home.StatePath)
	if err != nil {
		return Summary{}, err
	}
	if !opts.NoGlobal {
		for _, g := range plan.Globals {
			if state.HasGlobal(g.Key) && !opts.ReinstallGlobal {
				continue
			}
			allOK := true
			for _, c := range g.Commands {
				if !runOne(r, c) {
					allOK = false
				}
			}
			if allOK {
				sum.Installed = append(sum.Installed, g.Key)
				if !opts.DryRun {
					state.AddGlobal(g.Key)
				}
			} else {
				sum.Failed = append(sum.Failed, g.Key)
			}
		}
		if !opts.DryRun {
			if err := state.Save(home.StatePath); err != nil {
				return Summary{}, err
			}
		}
	}

	// 7b. componentes de conteúdo (I2) — cache-first sobre internal/store:
	// já cacheado restaura sem rede; senão adquire (git/CLI) e popula o
	// cache. Falha isolada não aborta o loop. `--global` só se aplica a
	// `via: skills` (I1 §3.3): instala pessoal/cross-project, não passa
	// pelo store nem é vendorizado no projeto. Roda ANTES de 7c: o
	// `graphify update .` (integração code_graph) precisa achar conteúdo
	// real em `.claude/` para ter o que indexar.
	st := store.New(home.StoreDir)
	for _, c := range prof.Components {
		if opts.Global && c.Via == profile.ViaSkills {
			cmd, err := acquire.GlobalInstallCommand(c)
			if err != nil {
				return Summary{}, err
			}
			cmd.Dir = target
			if opts.DryRun {
				fmt.Fprintf(out, "+ %s\n", cmd.String())
				sum.Installed = append(sum.Installed, cmd.String())
				continue
			}
			if runOne(r, cmd) {
				sum.Installed = append(sum.Installed, cmd.String())
			} else {
				sum.Failed = append(sum.Failed, cmd.String())
			}
			continue
		}

		acq, ok := acquire.For(c, r)
		if !ok {
			continue
		}
		coord := acq.Key(c)
		destRel, err := acquire.DestRel(c)
		if err != nil {
			return Summary{}, err
		}

		if opts.DryRun {
			fmt.Fprintf(out, "+ acquire %s -> %s\n", coord, destRel)
			sum.Installed = append(sum.Installed, coord)
			continue
		}

		srcDir, hit := st.Get(coord)
		if !hit {
			res, acqErr := acq.Acquire(context.Background(), c)
			if acqErr != nil {
				sum.Failed = append(sum.Failed, coord)
				continue
			}
			if !res.HasLicense {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: sem LICENSE detectada na fonte", coord))
			}
			if _, putErr := st.Put(coord, res.Dir); putErr != nil {
				return Summary{}, putErr
			}
			srcDir = res.Dir
		}

		if err := store.CopyTree(srcDir, filepath.Join(target, destRel)); err != nil {
			sum.Failed = append(sum.Failed, coord)
			continue
		}
		sum.Installed = append(sum.Installed, coord)
	}

	// 7c. comandos por-projeto das integrações (ex. `graphify update .`) —
	// roda depois de 7b para achar o conteúdo vendorizado já no disco.
	// Falha isolada não aborta o loop.
	for _, c := range plan.Commands {
		c.Dir = target
		if runOne(r, c) {
			sum.Installed = append(sum.Installed, c.String())
		} else {
			sum.Failed = append(sum.Failed, c.String())
		}
	}

	// 8. servers MCP.
	if err := mcp.WriteServers(target, plan.Servers, opts.DryRun, out); err != nil {
		return Summary{}, err
	}

	// 9. settings.json.
	settings := mergeMaps(prof.Scaffold.Settings, scaffold.HookSettings(opts.Mode))
	if err := claudecfg.MergeSettings(target, settings, opts.DryRun, out); err != nil {
		return Summary{}, err
	}

	// 10. scaffold (orientação + arquivos de sistema).
	files := dedupScaffoldFiles(prof.Scaffold.Files, scaffold.SystemFiles(opts.Mode))
	res, err := scaffold.WriteFiles(files, scaffold.Options{
		Target:       target,
		Data:         scaffold.Data{ProjectName: filepath.Base(target), Stack: prof.Name},
		Force:        opts.Force,
		DryRun:       opts.DryRun,
		Out:          out,
		TemplatesDir: home.TemplatesDir,
	})
	if err != nil {
		return Summary{}, err
	}
	sum.Created = res.Created
	sum.Skipped = res.Skipped

	// 11. .gitignore (I1) — regra-mãe: conteúdo de IA vendorizado é
	// commitável, runtime/segredos nunca são.
	gitignoreData := scaffold.Data{ProjectName: filepath.Base(target), Stack: prof.Name}
	if err := scaffold.MergeGitignore(target, prof.Scaffold.GitignoreStack, gitignoreData, opts.DryRun, out); err != nil {
		return Summary{}, err
	}
	sum.Created = append(sum.Created, ".gitignore")

	sum.HadFailure = len(sum.Failed) > 0
	return sum, nil
}

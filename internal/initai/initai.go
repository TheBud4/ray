// Package initai orquestra os 12 passos de `ray init ai` (ver Run), ligando
// profile, installer, mcp, claudecfg, vault, scaffold e preflight.
package initai

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TheBud4/ray/internal/claudecfg"
	"github.com/TheBud4/ray/internal/installer"
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

// Home reúne os caminhos de ~/.ray usados por Run, resolvidos pelo chamador
// (via internal/raypaths) para manter este pacote livre de os.Getenv e
// testável só com t.TempDir().
type Home struct {
	ProfilesDir   string
	TemplatesDir  string
	ConfigPath    string
	StatePath     string
	StoreDir      string
	ComponentsDir string
}

// Options são os parâmetros de `ray init ai`.
type Options struct {
	Profile         string
	Target          string
	Force           bool
	NoGlobal        bool
	ReinstallGlobal bool
	DryRun          bool
	Out             io.Writer
}

// Summary é o resultado final de Run.
type Summary struct {
	Installed  []string
	Failed     []string
	Created    []string
	Skipped    []string
	Warnings   []string
	HadFailure bool

	// DryRun diz que nada foi escrito: as listas acima são o que seria feito.
	DryRun bool

	// VersionedPaths são os caminhos de topo a passar para `git add`;
	// InGitRepo diz se faz sentido sugerir git. Ver o rodapé em
	// internal/cmd/init_ai.go.
	VersionedPaths []string
	InGitRepo      bool

	// Target é o alvo, em caminho absoluto: o rodapé o usa para dizer onde os
	// próximos passos valem.
	Target string
}

// versionedPaths reduz a lista de arquivos criados aos caminhos de topo que o
// usuário precisa passar para `git add`. Ordenado para a saída ser estável
// entre execuções — mensagem de comando que muda de ordem parece instável.
//
// Colapsar para o topo é o que torna o comando copiável sem edição, e é
// deliberado que não haja `-A` nem `.`: o guard-add.sh, que o próprio ray
// instala, avisa contra `git add` cego.
//
// Normaliza antes de colapsar porque `Created` não é uma lista sanitizada: o
// `profile.Validate` só recusa path vazio, então uma receita customizada pode
// trazer `./x` ou caminho absoluto. Sem normalizar, `./x` virava `.` e era
// descartado — o arquivo sumia do `git add`, que é a falha exata que este
// rodapé existe para impedir.
//
// Caminho fora do target é omitido: o rodapé só pode anunciar o que o `ray`
// escreveu dentro do projeto, e mandar `git add` no que está fora é pior que
// não mencionar.
func versionedPaths(target string, created []string) []string {
	seen := map[string]bool{}
	for _, p := range created {
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(target, p)
			if err != nil {
				continue
			}
			p = rel
		}
		p = filepath.ToSlash(filepath.Clean(p))
		if p == "." || p == ".." || strings.HasPrefix(p, "../") {
			continue
		}
		if i := strings.IndexByte(p, '/'); i > 0 {
			p = p[:i]
		}
		seen[p] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// inGitRepo sobe a árvore procurando .git. Não chama o binário git: é uma
// decisão de exibição, não de correção, e não vale acoplar o resumo a um
// processo externo que pode faltar.
func inGitRepo(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// Run executa os 12 passos de `ray init ai`. r executa comandos de
// componentes/globais (respeita --dry-run na fiação real); l checa
// dependências e deve continuar real mesmo sob --dry-run (mesmo raciocínio
// de `ray doctor`, Fase 7: um gate de validação não deve virar teatro).
func Run(r runner.Runner, l preflight.Looker, opts Options, home Home) (Summary, error) {
	var sum Summary
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	// 1. target (só o caminho absoluto; a pasta é criada no passo 6, depois de
	// tudo que pode recusar a execução).
	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return Summary{}, err
	}

	// A linha-base é o que separa "intocado" de "editado": ilegível, nenhum passo
	// abaixo pode decidir sobre sobrescrita, então para antes de qualquer efeito.
	st := store.New(home.StoreDir)
	if err := st.Verify(); err != nil {
		return Summary{}, err
	}

	// 2. garante ~/.ray populado. Em dry-run nada de ~/.ray é gravado: os
	// perfis de fábrica ausentes são lidos da memória (passo 3) e os templates
	// só são comparados.
	if !opts.DryRun {
		if err := profile.EnsureDir(home.ProfilesDir); err != nil {
			return Summary{}, err
		}
	}
	// O overlay de templates é sincronizado, não só criado: sem isso ele
	// sombreia o embed em silêncio, e atualizar o `ray` deixa de atualizar os
	// templates. A política de "editado?" é a mesma do `ray update`
	// (store.DecideOverwrite), com a mesma saída para --force.
	synced, err := scaffold.EnsureTemplates(home.TemplatesDir, scaffold.EnsureOptions{
		Force:    opts.Force,
		DryRun:   opts.DryRun,
		Pristine: func(rel string) (string, bool) { return st.PristineHash(home.TemplatesDir, rel) },
	})
	if err != nil {
		return Summary{}, err
	}
	for _, s := range synced {
		switch s.Action {
		case scaffold.TemplateCreated, scaffold.TemplateRefreshed:
			if opts.DryRun {
				continue
			}
			// Falha ao gravar a linha-base não derruba o `init ai`: sem
			// pristino, DecideOverwrite já cai na degradação graciosa
			// (disco == embed → atualiza; divergiu → preserva). Abortar aqui
			// trocaria um efeito recuperável por um comando morto.
			if err := st.SetPristine(home.TemplatesDir, s.Rel, s.Hash); err != nil {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("template %s: pristine baseline not recorded (%v)", s.Rel, err))
			}
		case scaffold.TemplateKept:
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("template %s: %s", s.Rel, s.Reason))
		}
	}

	// 3. carrega a receita.
	loadProfile := profile.LoadByName
	if opts.DryRun {
		loadProfile = profile.LoadByNameOrDefault
	}
	prof, err := loadProfile(home.ProfilesDir, opts.Profile)
	if err != nil {
		return Summary{}, err
	}

	// 4. preflight — aborta antes de qualquer efeito. A mensagem sai do
	// preflight, que é dono tanto do que checar quanto do que dizer: montar a
	// string aqui descartava o Hint e o Fix que o Check já carrega.
	needPython := prof.Integrations.Headroom || prof.Integrations.CodeGraph
	checks := preflight.Run(l, needPython)
	if missing := preflight.MissingRequired(checks); len(missing) > 0 {
		return Summary{}, &preflight.MissingRequiredError{
			Missing: missing,
			From:    preflight.FromGate,
		}
	}

	// 5. resolve o plano de instalação.
	plan, err := installer.Resolve(prof)
	if err != nil {
		return Summary{}, err
	}

	// 6. target + writability. Fica depois de tudo que pode recusar sem efeito
	// (receita, preflight, plano) e antes do primeiro efeito: recusar não pode
	// deixar uma pasta nova para trás.
	if err := ensureWritableDir(target, opts.DryRun, out); err != nil {
		return Summary{}, fmt.Errorf("target %s is not writable: %w", target, err)
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
				if ok, reason := runOne(r, c); !ok {
					allOK = false
					sum.Warnings = append(sum.Warnings, fmt.Sprintf("`%s`: %s", c.String(), reason))
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

	// 7b. componentes locais (skills/agents) — copiados de
	// home.ComponentsDir, nunca baixados: o usuário mantém o conteúdo lá, o
	// ray só copia e grava o hash pristino (para `ray update` decidir depois
	// se preserva edição local). Falha isolada não aborta o loop. Roda ANTES
	// de 7c: o `graphify update .` (integração code_graph) precisa achar
	// conteúdo real em `.claude/` para ter o que indexar.
	for _, c := range prof.Components {
		srcDir := filepath.Join(home.ComponentsDir, c.Name)
		if info, statErr := os.Stat(srcDir); statErr != nil || !info.IsDir() {
			sum.Failed = append(sum.Failed, c.Name)
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("component %q not found at %s", c.Name, srcDir))
			continue
		}

		destDir := filepath.Join(target, c.Dest, c.Name)

		// Edição local do usuário é preservada pela mesma política do
		// `ray update`: só sobrescreve se o destino ainda é o pristino. A
		// decisão é só leitura, então vale igual para o dry-run.
		freshHash, err := store.HashTree(srcDir)
		if err != nil {
			return Summary{}, err
		}
		onDiskHash, onDiskExists := store.LocalState(destDir)
		pristineHash, hasPristine := st.PristineHash(target, c.Name)
		overwrite, reason := store.DecideOverwrite(opts.Force, onDiskExists, onDiskHash, freshHash, pristineHash, hasPristine)
		if !overwrite {
			if opts.DryRun {
				fmt.Fprintf(out, "+ preserve %s (edited locally)\n", c.Name)
			}
			sum.Skipped = append(sum.Skipped, c.Name)
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s: %s", c.Name, reason))
			continue
		}

		if opts.DryRun {
			fmt.Fprintf(out, "+ copy %s -> %s\n", c.Name, destDir)
			sum.Installed = append(sum.Installed, c.Name)
			continue
		}

		// Cópia limpa: remove o destino antes, para não sobrar arquivo órfão.
		if err := os.RemoveAll(destDir); err != nil && !os.IsNotExist(err) {
			return Summary{}, err
		}
		if err := store.CopyTree(srcDir, destDir); err != nil {
			sum.Failed = append(sum.Failed, c.Name)
			continue
		}
		leafHash, err := store.HashTree(destDir)
		if err != nil {
			return Summary{}, err
		}
		if err := st.SetPristine(target, c.Name, leafHash); err != nil {
			return Summary{}, err
		}
		sum.Installed = append(sum.Installed, c.Name)
	}

	// 7c. comandos por-projeto das integrações (ex. `graphify update .`) —
	// roda depois de 7b para achar o conteúdo vendorizado já no disco.
	// A falha vira aviso, não Failed: o resultado desses comandos (o índice do
	// graphify) é regenerável, e numa pasta ainda sem código o graphify sai com
	// 1 sem que nada tenha dado errado. Não aborta o loop.
	for _, c := range plan.Commands {
		c.Dir = target
		if ok, reason := runOne(r, c); ok {
			sum.Installed = append(sum.Installed, c.String())
		} else {
			sum.Warnings = append(sum.Warnings, fmt.Sprintf("`%s` failed (%s); run it again once the project has content", c.String(), reason))
		}
	}

	// 8. servers MCP.
	// .mcp.json é vendorizado (está na whitelist do .gitignore) e, quando muda,
	// precisa entrar no Created: é dele que sai o `git add` do rodapé de
	// próximos passos.
	changed, err := writeChanged(filepath.Join(target, ".mcp.json"), opts.DryRun, out, func(w io.Writer) error {
		return mcp.WriteServers(target, plan.Servers, opts.DryRun, w)
	})
	if err != nil {
		return Summary{}, err
	}
	if changed {
		sum.Created = append(sum.Created, ".mcp.json")
	}

	// 9. scaffold (orientação + arquivos de sistema).
	files := dedupScaffoldFiles(prof.Scaffold.Files, scaffold.SystemFiles())
	res, err := scaffold.WriteFiles(files, scaffold.Options{
		Target:       target,
		Data:         scaffold.Data{ProjectName: filepath.Base(target), Stack: stackOf(prof)},
		Force:        opts.Force,
		DryRun:       opts.DryRun,
		Out:          out,
		TemplatesDir: home.TemplatesDir,
	})
	if err != nil {
		return Summary{}, err
	}
	// Acumula em vez de atribuir: o passo 8 já pode ter posto `.mcp.json` aqui,
	// e o 7b já pode ter posto componentes preservados em Skipped; atribuir
	// os descartaria em silêncio. Os passos 11 e 12 já acumulavam.
	sum.Created = append(sum.Created, res.Created...)
	sum.Skipped = append(sum.Skipped, res.Skipped...)

	// 10. settings.json. Vem depois do scaffold porque ele cita os scripts de
	// hook que o scaffold grava: se o scaffold falha, não sobra um settings
	// apontando para hooks inexistentes.
	settings := mergeSettings(prof.Scaffold.Settings, scaffold.HookSettings())
	changed, err = writeChanged(filepath.Join(target, ".claude", "settings.json"), opts.DryRun, out, func(w io.Writer) error {
		return claudecfg.MergeSettings(target, settings, opts.Force, opts.DryRun, w)
	})
	if err != nil {
		return Summary{}, err
	}
	if changed {
		sum.Created = append(sum.Created, ".claude/settings.json")
	}

	// 11. .gitignore (I1) — regra-mãe: conteúdo de IA vendorizado é
	// commitável, runtime/segredos nunca são.
	gitignoreData := scaffold.Data{ProjectName: filepath.Base(target), Stack: stackOf(prof)}
	changed, err = writeChanged(filepath.Join(target, ".gitignore"), opts.DryRun, out, func(w io.Writer) error {
		return scaffold.MergeGitignore(target, prof.Scaffold.GitignoreStack, gitignoreData, opts.DryRun, w)
	})
	if err != nil {
		return Summary{}, err
	}
	if changed {
		sum.Created = append(sum.Created, ".gitignore")
	}
	// Só leitura (vale no dry-run): se o usuário ignora .claude/ inteiro, a
	// lista de exceções que o ray acabou de pôr não tem efeito.
	if data, err := os.ReadFile(filepath.Join(target, ".gitignore")); err == nil && scaffold.GitignoreIgnoresClaudeDir(string(data)) {
		sum.Warnings = append(sum.Warnings, "your .gitignore ignores .claude/, so git will not track the environment ray wrote; remove that line (the block ray adds cannot re-include files under an ignored directory)")
	}

	// 12. registro do perfil (I3) — permite a `ray update` descobrir qual
	// receita re-adquirir sem exigir --profile num clone.
	profileRecord := filepath.Join(target, ".claude", ".ray-profile")
	wantRecord := []byte(prof.Name + "\n")
	if cur, err := os.ReadFile(profileRecord); err != nil || !bytes.Equal(cur, wantRecord) {
		if opts.DryRun {
			fmt.Fprintf(out, "+ write %s (%s)\n", profileRecord, prof.Name)
		} else {
			if err := os.MkdirAll(filepath.Dir(profileRecord), 0o755); err != nil {
				return Summary{}, err
			}
			if err := os.WriteFile(profileRecord, wantRecord, 0o644); err != nil {
				return Summary{}, err
			}
		}
		sum.Created = append(sum.Created, ".claude/.ray-profile")
	}

	sum.Target = target
	sum.DryRun = opts.DryRun
	sum.HadFailure = len(sum.Failed) > 0
	sum.VersionedPaths = versionedPaths(target, sum.Created)
	sum.InGitRepo = inGitRepo(target)
	return sum, nil
}

// stackOf é o valor de {{.Stack}} nos templates: o nome do perfil, que num
// perfil de stack é o nome da stack. O `base` não tem stack, e "base" escrito
// como linguagem/runtime no CLAUDE.md seria informação falsa — o template
// recebe vazio e deixa o campo para quem preenche.
func stackOf(p *profile.Profile) string {
	if p.Name == profile.BaseName {
		return ""
	}
	return p.Name
}

// writeChanged roda step — que grava path ou, em dry-run, imprime o que
// gravaria — e diz se o conteúdo final de path difere do que havia. É o que
// separa "o ray escreveu isto" de "o ray passou por isto": repetir o comando
// não pode listar como criado o que ficou idêntico.
func writeChanged(path string, dryRun bool, out io.Writer, step func(io.Writer) error) (bool, error) {
	before, readErr := os.ReadFile(path)
	existed := readErr == nil

	if dryRun {
		var printed bytes.Buffer
		if err := step(io.MultiWriter(out, &printed)); err != nil {
			return false, err
		}
		return printed.Len() > 0 && (!existed || !bytes.Equal(before, printed.Bytes())), nil
	}

	if err := step(out); err != nil {
		return false, err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	return !existed || !bytes.Equal(before, after), nil
}

// Package acquire materializa componentes de conteúdo (via: skills|aitmpl|
// git) em bytes locais — a peça que fecha o buraco do vendoring do I1: sem
// --copy o `npx skills add` deixa symlink no `.claude/` (link quebrado fora
// da máquina que instalou). GitAcquirer/CliAcquirer sabem *como* buscar;
// quem decide *quando* buscar (cache-first, via internal/store) é
// internal/initai.
package acquire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

// Result é o conteúdo extraído de um componente, pronto para store.Put (pela
// coordenada de Acquirer.Key) e para ser mesclado em <projeto>/DestRel.
type Result struct {
	// Dir é um diretório-contêiner cujo conteúdo deve ser mesclado em
	// DestRel — nunca o "arquivo final" em si, para que múltiplos
	// componentes com o mesmo DestRel (ex. vários agents em .claude/agents)
	// se acumulem em vez de se sobrescreverem.
	Dir string
	// DestRel é sempre um diretório relativo ao projeto (ex.
	// ".claude/skills", ".claude/agents").
	DestRel string
	// Origin é "repo@ref" (git) ou a fonte equivalente (skills/aitmpl) —
	// vira o conteúdo do .ray-origin vendorizado.
	Origin string
	// HasLicense reporta se uma LICENSE foi encontrada e capturada junto do
	// conteúdo. false não bloqueia nada — só vira aviso soft no chamador.
	HasLicense bool
}

// Acquirer sabe materializar um profile.Component em Result.
type Acquirer interface {
	// Key é a coordenada namespaced usada como chave no internal/store —
	// pura, não toca disco nem rede.
	Key(comp profile.Component) string
	Acquire(ctx context.Context, comp profile.Component) (Result, error)
}

// For seleciona o Acquirer certo para comp.Via, ou ok=false se comp não é
// conteúdo adquirível (ex. aitmpl mcp, que é ferramenta — trilha .mcp.json).
func For(comp profile.Component, r runner.Runner) (Acquirer, bool) {
	switch comp.Via {
	case profile.ViaGit:
		return GitAcquirer{Runner: r}, true
	case profile.ViaSkills:
		return CliAcquirer{Runner: r}, true
	case profile.ViaAitmpl:
		if comp.Type == profile.TypeMCP {
			return nil, false
		}
		return CliAcquirer{Runner: r}, true
	default:
		return nil, false
	}
}

// ---- GitAcquirer (via: git) ----------------------------------------------

// GitAcquirer clona repo@ref e extrai comp.Path. MVP (build guide/plano I2):
// só shorthand de GitHub, só o layout padrão skills/<nome>/ — sem submódulos,
// sem outros hosts.
type GitAcquirer struct {
	Runner runner.Runner
}

func (GitAcquirer) Key(comp profile.Component) string {
	return fmt.Sprintf("git:%s@%s#%s", comp.Repo, gitRef(comp), comp.Path)
}

func gitRef(comp profile.Component) string {
	if comp.Ref != "" {
		return comp.Ref
	}
	return "main"
}

// gitFetchCommand é o builder puro (testável sem rede): resolve repo@ref num
// clone raso em destDir.
func gitFetchCommand(comp profile.Component, destDir string) runner.Command {
	return runner.Command{
		Name: "git",
		Args: []string{"clone", "--depth", "1", "--branch", gitRef(comp), "https://github.com/" + comp.Repo + ".git", destDir},
	}
}

func (g GitAcquirer) Acquire(ctx context.Context, comp profile.Component) (Result, error) {
	clone, err := os.MkdirTemp("", "ray-acquire-git-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(clone)

	res, err := g.Runner.Run(ctx, gitFetchCommand(comp, clone))
	if err != nil {
		return Result{}, fmt.Errorf("git clone %s: %w", comp.Repo, err)
	}
	if res.ExitCode != 0 {
		return Result{}, fmt.Errorf("git clone %s: exit %d: %s", comp.Repo, res.ExitCode, res.Stderr)
	}

	srcDir := filepath.Join(clone, filepath.FromSlash(comp.Path))
	if info, statErr := os.Stat(srcDir); statErr != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("path %q not found in %s@%s", comp.Path, comp.Repo, gitRef(comp))
	}

	name, err := LeafName(comp)
	if err != nil {
		return Result{}, err
	}
	container, err := os.MkdirTemp("", "ray-acquire-content-")
	if err != nil {
		return Result{}, err
	}
	finalDir := filepath.Join(container, name)
	if err := store.CopyTree(srcDir, finalDir); err != nil {
		return Result{}, err
	}

	origin := comp.Repo + "@" + gitRef(comp)
	hasLicense, err := captureCompliance(finalDir, clone, origin)
	if err != nil {
		return Result{}, err
	}

	return Result{Dir: container, DestRel: filepath.Join(".claude", "skills"), Origin: origin, HasLicense: hasLicense}, nil
}

// ---- CliAcquirer (via: skills, via: aitmpl agent|command) -----------------

// CliAcquirer embrulha os installers de terceiros (npx skills add,
// claude-code-templates), forçando --copy (senão vendoriza symlink) e
// telemetria off, rodando num projeto temporário isolado.
type CliAcquirer struct {
	Runner runner.Runner
}

func (CliAcquirer) Key(comp profile.Component) string {
	switch comp.Via {
	case profile.ViaSkills:
		return fmt.Sprintf("skills:%s#%s", comp.Source, comp.Skill)
	case profile.ViaAitmpl:
		return fmt.Sprintf("aitmpl:%s:%s", comp.Type, comp.Ref)
	default:
		return ""
	}
}

// cliAcquireCommand é o builder puro: monta o comando npx que instala comp
// em projectDir, e devolve onde o resultado deve aparecer — destRel (sempre
// um diretório-contêiner) + name (subdir, para skills; arquivo .md, para
// aitmpl).
func cliAcquireCommand(comp profile.Component, projectDir string) (cmd runner.Command, destRel, name string, err error) {
	env := map[string]string{"DO_NOT_TRACK": "1", "DISABLE_TELEMETRY": "1"}
	switch comp.Via {
	case profile.ViaSkills:
		args := []string{"skills", "add", comp.Source, "--skill", comp.Skill, "-a", "claude-code", "-y", "--copy"}
		return runner.Command{Name: "npx", Args: args, Dir: projectDir, Env: env}, filepath.Join(".claude", "skills"), comp.Skill, nil
	case profile.ViaAitmpl:
		flag, sub, aerr := aitmplDestSubdir(comp.Type)
		if aerr != nil {
			return runner.Command{}, "", "", aerr
		}
		name, nerr := LeafName(comp)
		if nerr != nil {
			return runner.Command{}, "", "", nerr
		}
		args := []string{"claude-code-templates@latest", flag + "=" + comp.Ref, "--yes"}
		return runner.Command{Name: "npx", Args: args, Dir: projectDir, Env: env}, filepath.Join(".claude", sub), name, nil
	default:
		return runner.Command{}, "", "", fmt.Errorf("via %q is not a CLI acquisition", comp.Via)
	}
}

// LeafName devolve, sem tocar disco/rede, o nome do subdiretório/arquivo que
// o conteúdo de comp ocupa dentro do seu DestRel (ex. ".claude/skills/<nome>")
// — usado pelo `ray update` (I3) para localizar o conteúdo já vendorizado no
// projeto e hasheá-lo antes de decidir se sobrescreve.
func LeafName(comp profile.Component) (string, error) {
	switch comp.Via {
	case profile.ViaGit:
		return filepath.Base(comp.Path), nil
	case profile.ViaSkills:
		return comp.Skill, nil
	case profile.ViaAitmpl:
		return filepath.Base(comp.Ref) + ".md", nil
	default:
		return "", fmt.Errorf("unknown via %q", comp.Via)
	}
}

func aitmplDestSubdir(t string) (flag, sub string, err error) {
	switch t {
	case profile.TypeAgent:
		return "--agent", "agents", nil
	case profile.TypeCommand:
		return "--command", "commands", nil
	default:
		return "", "", fmt.Errorf("unknown aitmpl content type %q", t)
	}
}

func (c CliAcquirer) Acquire(ctx context.Context, comp profile.Component) (Result, error) {
	project, err := os.MkdirTemp("", "ray-acquire-cli-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(project)

	cmd, destRel, name, err := cliAcquireCommand(comp, project)
	if err != nil {
		return Result{}, err
	}

	res, err := c.Runner.Run(ctx, cmd)
	if err != nil {
		return Result{}, fmt.Errorf("acquire %s: %w", c.Key(comp), err)
	}
	if res.ExitCode != 0 {
		return Result{}, fmt.Errorf("acquire %s: exit %d: %s", c.Key(comp), res.ExitCode, res.Stderr)
	}

	acquiredPath := filepath.Join(project, destRel, name)
	if _, statErr := os.Stat(acquiredPath); statErr != nil {
		return Result{}, fmt.Errorf("acquire %s: expected content at %s: %w", c.Key(comp), acquiredPath, statErr)
	}

	container, err := os.MkdirTemp("", "ray-acquire-content-")
	if err != nil {
		return Result{}, err
	}
	finalPath := filepath.Join(container, name)
	if err := store.CopyTree(acquiredPath, finalPath); err != nil {
		return Result{}, err
	}

	origin := comp.Source
	if comp.Via == profile.ViaAitmpl {
		origin = comp.Ref
	}

	var hasLicense bool
	if comp.Via == profile.ViaSkills {
		// Só skills tem layout de diretório — aitmpl (agent/command) é um
		// único arquivo .md e não tem onde guardar .ray-origin/LICENSE sem
		// colidir com outro componente do mesmo DestRel (YAGNI: sem valor de
		// compliance suficiente pra justificar nomear por componente).
		hasLicense, err = captureCompliance(finalPath, project, origin)
		if err != nil {
			return Result{}, err
		}
	}

	return Result{Dir: container, DestRel: destRel, Origin: origin, HasLicense: hasLicense}, nil
}

// captureCompliance escreve .ray-origin em contentDir e, se achar uma
// LICENSE em contentDir ou searchRoot (nessa ordem), copia pra dentro de
// contentDir também. Ausência de LICENSE não é erro — só vira HasLicense
// false (aviso soft no chamador).
func captureCompliance(contentDir, searchRoot, origin string) (bool, error) {
	if err := os.WriteFile(filepath.Join(contentDir, ".ray-origin"), []byte(origin+"\n"), 0o644); err != nil {
		return false, err
	}
	for _, root := range []string{contentDir, searchRoot} {
		for _, name := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt"} {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(contentDir, "LICENSE"), data, 0o644); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// ---- Trilha pessoal/global (--global; I1 design §3.3) ---------------------

// GlobalInstallCommand monta o install "-g" de um componente via: skills
// como conteúdo pessoal cross-project — não passa pelo store, não é
// vendorizado, então não força --copy (symlink em ~/.claude é aceitável:
// atualiza sozinho, não é commitado por ninguém). Só via: skills tem esse
// caminho; aitmpl segue sempre project-local (I1).
func GlobalInstallCommand(comp profile.Component) (runner.Command, error) {
	if comp.Via != profile.ViaSkills {
		return runner.Command{}, fmt.Errorf("via %q has no personal/global install path", comp.Via)
	}
	args := []string{"skills", "add", comp.Source, "--skill", comp.Skill, "-a", "claude-code", "-y", "-g"}
	return runner.Command{Name: "npx", Args: args}, nil
}

// DestRel devolve, sem tocar rede/disco, o diretório-contêiner (relativo ao
// projeto) que comp ocupa — o mesmo valor que Acquire().DestRel. Permite ao
// chamador (initai) restaurar um cache-hit sem precisar rodar Acquire de
// novo só para saber o destino.
func DestRel(comp profile.Component) (string, error) {
	switch comp.Via {
	case profile.ViaSkills, profile.ViaGit:
		return filepath.Join(".claude", "skills"), nil
	case profile.ViaAitmpl:
		_, sub, err := aitmplDestSubdir(comp.Type)
		if err != nil {
			return "", err
		}
		return filepath.Join(".claude", sub), nil
	default:
		return "", fmt.Errorf("unknown via %q", comp.Via)
	}
}

// PreviewCommand devolve, só para exibição (ex. `ray profile show`), o
// comando que a aquisição rodaria para comp. Não executa nada, não resolve
// destRel/paths reais.
func PreviewCommand(comp profile.Component) (runner.Command, error) {
	switch comp.Via {
	case profile.ViaSkills, profile.ViaAitmpl:
		cmd, _, _, err := cliAcquireCommand(comp, "")
		return cmd, err
	case profile.ViaGit:
		return gitFetchCommand(comp, "<clone>"), nil
	default:
		return runner.Command{}, fmt.Errorf("unknown via %q", comp.Via)
	}
}

package initai

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/safepath"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

// fixedDestinations são os destinos dentro do projeto que todo `init ai`
// escreve, qualquer que seja a receita: os arquivos de configuração e os hooks
// de sistema. Caminhos relativos ao projeto, com `/`.
func fixedDestinations() []string {
	dests := []string{
		".mcp.json",
		".claude/settings.json",
		".gitignore",
		".claude/.ray-profile",
		store.BaselineFile,
	}
	for _, f := range scaffold.SystemFiles() {
		dests = append(dests, f.Path)
	}
	return dests
}

// recipeDestinations são os destinos que dependem da receita: o diretório de
// cada componente, cada entrada que já existe dentro dele e cada arquivo de
// scaffold. As entradas existentes entram porque a cópia limpa as regrava, e um
// clone pode trazer qualquer uma como symlink para fora.
func recipeDestinations(target string, p *profile.Profile) []string {
	var dests []string
	for _, c := range p.Components {
		dir := filepath.Join(c.Dest, c.Name)
		dests = append(dests, dir)
		// WalkDir não segue symlink: o link aparece como entrada e é conferido.
		// Erro de leitura (pasta ainda inexistente) não é motivo de recusa aqui.
		_ = filepath.WalkDir(filepath.Join(target, dir), func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if rel, err := filepath.Rel(target, path); err == nil {
				dests = append(dests, rel)
			}
			return nil
		})
	}
	for _, f := range p.Scaffold.Files {
		dests = append(dests, f.Path)
	}
	return dests
}

// checkDestinations recusa a execução se algum destino, depois de seguir os
// symlinks do caminho, sair de target. Um repositório clonado pode trazer
// qualquer um deles como link para fora, e gravar por cima escreveria fora do
// projeto; um link que resolve para dentro continua valendo. Só lê o disco, por
// isso roda antes de qualquer efeito.
func checkDestinations(target string, rels []string) error {
	for _, rel := range rels {
		if err := safepath.ResolveInside(target, filepath.Join(target, rel)); err != nil {
			return fmt.Errorf("refusing to write %s: %w", rel, err)
		}
	}
	return nil
}

// ensureWritableDir garante que dir existe e é gravável, escrevendo e
// removendo um arquivo-probe. O nome é aleatório e a criação é exclusiva
// (os.CreateTemp): um nome fixo poderia vir de um clone como symlink para fora,
// e o probe o seguiria — escrita que nenhum guard de destino enxerga, porque
// roda depois deles e some antes da enumeração.
//
// Em dryRun não faz nem uma coisa nem outra: um dry-run que cria o diretório
// do projeto já executou a parte irreversível antes de o usuário decidir. O
// --dry-run só alcança o runner.ExecRunner, então todo caminho que toca o
// disco fora dele tem de perguntar por ele à mão — imprime o que faria, como
// o ExecRunner faz com processo. O preço é não checar gravabilidade na
// simulação, e isso é o certo: dry-run mostra o plano, não valida o terreno.
func ensureWritableDir(dir string, dryRun bool, out io.Writer) error {
	if dryRun {
		fmt.Fprintf(out, "+ mkdir -p %s\n", dir)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".ray-write-test-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Remove(name)
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

// dedupScaffoldFiles mantém base inteiro e acrescenta de extra só os paths
// ausentes em base — a receita "ganha".
func dedupScaffoldFiles(base, extra []profile.ScaffoldFile) []profile.ScaffoldFile {
	seen := make(map[string]bool, len(base))
	out := make([]profile.ScaffoldFile, len(base))
	copy(out, base)
	for _, f := range base {
		seen[f.Path] = true
	}
	for _, f := range extra {
		if seen[f.Path] {
			continue
		}
		out = append(out, f)
		seen[f.Path] = true
	}
	return out
}

// mergeSettings une os settings da receita aos do ray: `hooks` é unido por
// evento (entradas da receita primeiro, as do ray depois) para que os hooks do
// ray não apaguem os da receita; as demais chaves são uma união rasa em que b
// vence.
func mergeSettings(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if k == "hooks" {
			out[k] = mergeHookEvents(out[k], v)
			continue
		}
		out[k] = v
	}
	return out
}

// mergeHookEvents concatena as entradas de cada evento de a e b, sem alterar
// nenhum dos dois. Se algum não for um mapa de eventos, vale b.
func mergeHookEvents(a, b any) any {
	am, ok := a.(map[string]any)
	bm, ok2 := b.(map[string]any)
	if !ok || !ok2 {
		return b
	}
	out := make(map[string]any, len(am)+len(bm))
	for event, v := range am {
		out[event] = v
	}
	for event, v := range bm {
		prev, pok := out[event].([]any)
		add, aok := v.([]any)
		if !pok || !aok {
			out[event] = v
			continue
		}
		merged := make([]any, 0, len(prev)+len(add))
		merged = append(merged, prev...)
		out[event] = append(merged, add...)
	}
	return out
}

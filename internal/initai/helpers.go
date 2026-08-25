package initai

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// ensureWritableDir garante que dir existe e é gravável, escrevendo e
// removendo um arquivo-probe.
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
	probe := filepath.Join(dir, ".ray-write-test")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		return err
	}
	return os.Remove(probe)
}

// runOne roda c via r e classifica o resultado: err ou ExitCode != 0 → false.
func runOne(r runner.Runner, c runner.Command) bool {
	res, err := r.Run(context.Background(), c)
	if err != nil {
		return false
	}
	return res.ExitCode == 0
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

// mergeMaps é uma união rasa: b sobrescreve chaves de a.
func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

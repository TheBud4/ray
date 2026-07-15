package initai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

// ensureWritableDir garante que dir existe e é gravável, escrevendo e
// removendo um arquivo-probe.
func ensureWritableDir(dir string) error {
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
// ausentes em base — a receita "ganha" (build guide §8).
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

// resolveLevel valida --level contra mode (design §9.1, I6a): só é válido
// com mode == scaffold.ModeLearn (senão erro); vazio em learn vira
// scaffold.LevelIntermediate; qualquer outro valor precisa ser um dos três
// níveis conhecidos.
func resolveLevel(mode, level string) (string, error) {
	if mode != scaffold.ModeLearn {
		if level != "" {
			return "", fmt.Errorf("--level is only valid with --mode %s", scaffold.ModeLearn)
		}
		return "", nil
	}
	if level == "" {
		return scaffold.LevelIntermediate, nil
	}
	switch level {
	case scaffold.LevelBeginner, scaffold.LevelIntermediate, scaffold.LevelAdvanced:
		return level, nil
	default:
		return "", fmt.Errorf("invalid --level %q (want %q, %q or %q)", level, scaffold.LevelBeginner, scaffold.LevelIntermediate, scaffold.LevelAdvanced)
	}
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

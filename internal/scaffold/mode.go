package scaffold

import "github.com/TheBud4/ray/internal/profile"

// Modos suportados por `ray init ai --mode`.
const (
	ModeBuild = "build"
	ModeLearn = "learn"
)

// SystemFiles são os arquivos "de sistema" que o ray sempre escreve, fora da
// receita — garante que todo hook referenciado em settings.json exista no
// disco. No initai (Fase 8), estes se somam a prof.Files (dedup por path,
// receita ganha).
func SystemFiles(mode string) []profile.ScaffoldFile {
	files := []profile.ScaffoldFile{
		{Path: ".claude/hooks/session-start.sh"},
	}
	if mode == ModeLearn {
		files = append(files,
			profile.ScaffoldFile{Path: ".claude/rules/learn.md"},
			profile.ScaffoldFile{Path: ".claude/hooks/guard-code.sh"},
		)
	}
	return files
}

// HookSettings devolve o bloco "hooks" a mesclar em settings.json (via
// claudecfg.MergeSettings): SessionStart sempre injeta o handoff; learn
// adiciona o PreToolUse que bloqueia edição de código fora da allowlist.
func HookSettings(mode string) map[string]any {
	hooks := map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/session-start.sh"},
				},
			},
		},
	}
	if mode == ModeLearn {
		hooks["PreToolUse"] = []any{
			map[string]any{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/guard-code.sh"},
				},
			},
		}
	}
	return map[string]any{"hooks": hooks}
}

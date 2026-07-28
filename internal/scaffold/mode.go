package scaffold

import "github.com/TheBud4/ray/internal/profile"

// Modos suportados por `ray init ai --mode`.
const (
	ModeBuild = "build"
	ModeLearn = "learn"
)

// Níveis suportados por `ray init ai --level` (design §9.1) — só válidos com
// --mode learn. Selecionam a variante do conteúdo de ensino (I6b); I6a só
// valida e rosqueia o valor.
const (
	LevelBeginner     = "beginner"
	LevelIntermediate = "intermediate"
	LevelAdvanced     = "advanced"
)

// SystemFiles são os arquivos "de sistema" que o ray sempre escreve, fora da
// receita — garante que todo hook referenciado em settings.json exista no
// disco. No initai (Fase 8), estes se somam a prof.Files (dedup por path,
// receita ganha).
func SystemFiles(mode string) []profile.ScaffoldFile {
	files := []profile.ScaffoldFile{
		{Path: ".claude/hooks/session-start.sh"},
		{Path: ".claude/hooks/guard-add.sh"},
		{Path: ".claude/hooks/guard-vocab.sh"},
		{Path: ".claude/hooks/guard-plans.sh"},
	}
	if mode == ModeLearn {
		files = append(files,
			profile.ScaffoldFile{Path: ".claude/rules/learn.md"},
			profile.ScaffoldFile{Path: ".claude/hooks/guard-code.sh"},
			profile.ScaffoldFile{Path: ".claude/rules/learning-journal.md"},
		)
	}
	return files
}

// HookSettings devolve o bloco "hooks" a mesclar em settings.json (via
// claudecfg.MergeSettings): SessionStart sempre injeta o handoff; PreToolUse
// sempre traz o guard-add (avisa em `git add` cego) e o guard-plans (avisa
// em plano/design escrito dentro do repositório); PostToolUse sempre traz
// o guard-vocab (avisa em vocabulário de processo vazado para artefato
// entregue); learn soma ao PreToolUse o guard-code, que bloqueia edição de
// código fora da allowlist.
func HookSettings(mode string) map[string]any {
	hooks := map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/session-start.sh"},
				},
			},
		},
		"PreToolUse": []any{
			map[string]any{
				"matcher": "Bash",
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/guard-add.sh"},
				},
			},
			map[string]any{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/guard-plans.sh"},
				},
			},
		},
		"PostToolUse": []any{
			map[string]any{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": "bash .claude/hooks/guard-vocab.sh"},
				},
			},
		},
	}
	if mode == ModeLearn {
		pre, _ := hooks["PreToolUse"].([]any)
		hooks["PreToolUse"] = append(pre, map[string]any{
			"matcher": "Edit|Write|MultiEdit",
			"hooks": []any{
				map[string]any{"type": "command", "command": "bash .claude/hooks/guard-code.sh"},
			},
		})
	}
	return map[string]any{"hooks": hooks}
}

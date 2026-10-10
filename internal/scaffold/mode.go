package scaffold

import (
	"github.com/TheBud4/ray/internal/claudecfg"
	"github.com/TheBud4/ray/internal/profile"
)

// SystemFiles são os arquivos "de sistema" que o ray sempre escreve, fora da
// receita — garante que todo hook referenciado em settings.json exista no
// disco, e que um checkout com conversão de fim de linha não os corrompa. No initai (Fase 8), estes se somam a prof.Files (dedup por path,
// receita ganha).
func SystemFiles() []profile.ScaffoldFile {
	return []profile.ScaffoldFile{
		{Path: ".claude/hooks/session-start.sh"},
		{Path: ".claude/hooks/guard-add.sh"},
		{Path: ".claude/hooks/guard-vocab.sh"},
		{Path: ".claude/hooks/guard-plans.sh"},
		{Path: ".claude/hooks/guard-handoff.sh"},
		{Path: ".claude/.gitattributes"},
	}
}

// HookSettings devolve o bloco "hooks" a mesclar em settings.json (via
// claudecfg.MergeSettings): SessionStart sempre injeta o handoff; PreToolUse
// sempre traz os três guards de aviso — guard-add (avisa em `git add` cego),
// guard-plans (avisa em plano/design escrito dentro do repositório) e
// guard-vocab (avisa em vocabulário de processo vazado para artefato
// entregue). Todos avisam antes da escrita, quando ainda dá para redirecionar.
// PostToolUse sempre traz guard-handoff (avisa quando .claude/handoff.md
// passa do dobro do orçamento) — depois da escrita, porque é o tamanho do
// arquivo no disco que importa, não o trecho que uma chamada carregou.
func HookSettings() map[string]any {
	hooks := map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": hookCommand("session-start.sh")},
				},
			},
		},
		"PreToolUse": []any{
			map[string]any{
				"matcher": "Bash",
				"hooks": []any{
					map[string]any{"type": "command", "command": hookCommand("guard-add.sh")},
				},
			},
			map[string]any{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": hookCommand("guard-plans.sh")},
				},
			},
			map[string]any{
				"matcher": "Edit|Write|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": hookCommand("guard-vocab.sh")},
				},
			},
		},
		"PostToolUse": []any{
			map[string]any{
				"matcher": "Write|Edit|MultiEdit",
				"hooks": []any{
					map[string]any{"type": "command", "command": hookCommand("guard-handoff.sh")},
				},
			},
		},
	}
	return map[string]any{"hooks": hooks}
}

// hookCommand é o comando de settings.json que roda o hook name. O prefixo vem
// do claudecfg, que é quem reconhece, ao mesclar, um hook como "do ray".
func hookCommand(name string) string {
	return claudecfg.RayHookPrefix + name
}

# Todo

> Reescrito a cada nova tarefa — não é histórico. O histórico de decisões e
> aprendizados vive nos commits e em `tasks/lessons.md`.

## Tarefa

Fase 3 do build guide: `internal/installer` + `internal/mcp` (tipo Server) —
receita validada vira um `Plan` de dados (comandos, globais, servers), sem
executar nada. Plano completo em
`/home/thebud4/.claude/plans/snazzy-knitting-squirrel.md`.

## Plano

- [x] `internal/mcp/mcp.go` (tipo Server)
- [x] `internal/installer/installer.go` (Plan, GlobalStep, Options, Resolve, componentCommand, aitmplFlag) — TDD
- [x] `internal/installer/integrations.go` (6 handlers da tabela §6)
- [x] `internal/installer/installer_test.go` (TestComponentCommand + TestResolve*)
- [x] `make ci` verde
- [x] commit único, sem Co-Authored-By

## Revisão

Fase 3 concluída (commit `001c498`). `internal/mcp` ganhou o tipo `Server`
(sem comportamento ainda — o writer de `.mcp.json` é Fase 4).
`internal/installer` transforma `*profile.Profile` num `Plan` puro
(`Commands`/`Globals`/`Servers`), com `componentCommand` mapeando `via:
skills|aitmpl` e `resolveIntegrations` cobrindo as 6 integrações da tabela §6
(headroom, knowledge_vault, user_docs_vault condicional, second_brain,
obsidian_formats, code_graph com 2 comandos globais + comando por-projeto +
server). Mapeamento literal da Fase 3 (sem `--copy`/`DO_NOT_TRACK`, que fica
pro incremento I1/I2 depois de todas as fases prontas). TDD: teste escrito
primeiro (falhou por compilação), depois a implementação, verde de primeira.
`make ci` verde. Próximo passo: Fase 4 (`internal/mcp` ganha o writer de
`.mcp.json` + `internal/claudecfg` para merge de `settings.json`).

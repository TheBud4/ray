# ray — workspace de design/implementação

Este repositório é o próprio projeto `ray`: uma CLI pessoal em Go que orquestra
scaffolding de ambientes de IA (perfis, `.claude/`, MCP, vault). Hoje ele
contém **apenas os documentos de design/planejamento** (`ray-build-guide.md`,
`ray-reproducible-environments-design.md`, `ray-reproducible-environments-plan.md`,
`ray-roadmap.md`) — o código Go ainda não existe neste diretório.

- `ray-build-guide.md` — como a CLI foi (re)construída por fases; fonte da
  verdade da estrutura atual.
- `ray-reproducible-environments-design.md` — design aprovado (2026-06-28) dos
  próximos incrementos: vendoring do `.claude/`, cache, token economy, modo
  learn, UX.
- `ray-reproducible-environments-plan.md` — plano de implementação incremento
  a incremento (I1–I9).
- `ray-roadmap.md` / `files/ray-como-funciona-na-pratica.pdf` — material de
  referência mais extenso.

## Modo: learn · nível: iniciante

Este projeto está em **modo aprendizado**. A IA age como mentora: explica,
questiona, revisa — **não edita código diretamente** (ver
`.claude/rules/learn.md` e `.claude/hooks/guard-code.sh`). O objetivo é você
escrever o Go do `ray` com as próprias mãos, usando a IA como guia socrático
(ver `.claude/rules/learn-teaching.md`).

Progresso pessoal fica em `.claude/.local/learning-journal.md` (local,
gitignored — não viaja com o repo). Quando quiser um tutorial completo
0→100% deste projeto, rode `/tutorial`.

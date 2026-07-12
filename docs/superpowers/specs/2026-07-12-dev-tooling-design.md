# `ray` — Design: Tooling de desenvolvimento (build/qualidade, rastreio de tarefas, agente)

> **Status:** aprovado (brainstorming) · **Data:** 2026-07-12

## Contexto e motivação

O repositório está na Fase 1 de 12 do `ray-build-guide.md` (runner concluído,
`profile` é o próximo passo). O `.claude/` de modo learn (guard-rail que
bloqueava edição direta de código) foi removido deliberadamente — a IA agora
implementa o código diretamente, seguindo o `CLAUDE.md` genérico do projeto
(Plan Node Default, Task Management via `tasks/`, etc.).

Esse `CLAUDE.md` já **referencia** `tasks/todo.md` e `tasks/lessons.md`, mas
esses arquivos não existem. Da mesma forma, o `ray-build-guide.md` §18 já
**especifica** um Makefile (`build/install/test/vet/fmt/fmt-check/ci`), mas ele
nunca foi criado — hoje cada verificação exige rodar `go build`/`go test`/`go
vet`/`gofmt` manualmente. Este design fecha essas duas lacunas e adiciona um
terceiro item, pedido pelo usuário: um comando de agente que resume o estado
do projeto em relação ao plano de fases, evitando o trabalho manual de cruzar
`git log` com o build guide a cada sessão.

**Fora de escopo (decidido explicitamente durante o brainstorming):**
- CI via GitHub Actions — adiado para a Fase 11, como o build guide já previa.
- `golangci-lint` — mantido no mínimo (`go vet` + `gofmt`), sem dependência extra.
- `PROGRESS.md` com checklist de fases — descartado; o build guide + `git log`
  já bastam como fonte de verdade do progresso.
- Qualquer outro comando/skill de agente além do `/proxima-fase` — YAGNI, pode
  ser adicionado depois se a necessidade aparecer.

## Componentes

### 1. `Makefile`

Implementa exatamente o que `ray-build-guide.md` §18 já documenta como
especificação — este design apenas o materializa:

| Target | Comando | Propósito |
|---|---|---|
| `build` | `go build ./...` | compila tudo |
| `install` | `go install .` | instala o binário em `~/go/bin` |
| `test` | `go test ./...` | roda a suíte |
| `vet` | `go vet ./...` | análise estática |
| `fmt` | `gofmt -w .` | formata e grava |
| `fmt-check` | `gofmt -l .` (falha se listar algo) | verifica formatação sem gravar — usado pelo `ci` e, futuramente, pelo GitHub Actions |
| `ci` | `fmt-check vet test` | o "verde" que todo commit deve manter, por fase (build guide §14) |

`fmt-check` precisa falhar (exit ≠ 0) quando `gofmt -l .` lista algum arquivo —
por padrão `gofmt -l` só imprime e sai 0, então o target precisa checar se a
saída é vazia.

### 2. `tasks/`

Dois arquivos, com papéis diferentes (já implícitos no `CLAUDE.md` genérico):

- **`tasks/todo.md`** — o plano da tarefa **corrente**. É reescrito a cada nova
  tarefa (não é histórico cumulativo). Criado agora como um template vazio com
  a estrutura esperada: `## Tarefa`, `## Plano` (checklist), `## Revisão`
  (preenchida ao final, conforme "Document Results" do `CLAUDE.md`).
- **`tasks/lessons.md`** — acumulador **append-only** de correções/padrões
  aprendidos, atualizado após qualquer correção do usuário ("Self-Improvement
  Loop" do `CLAUDE.md`). Criado já com a primeira entrada real e conhecida:
  a política de commit sem trailer `Co-Authored-By`.

### 3. `.claude/commands/proxima-fase.md`

Um único slash-command, sem `.claude/settings.json` nem hooks — Claude Code
descobre comandos em `.claude/commands/*.md` automaticamente, sem configuração
adicional. Ao ser invocado (`/proxima-fase`), instrui o agente a:

1. Ler o plano de fases do `docs/ray-build-guide.md` (§14, Fases 0–12) e os
   incrementos do `docs/ray-reproducible-environments-plan.md` (I1–I9), que se
   aplicam **depois** que o build guide estiver implementado.
2. Checar `git log --oneline` e a árvore atual de `internal/` para inferir o
   que já está feito (mapeando mensagens de commit e pacotes existentes às
   fases/incrementos).
3. Devolver um resumo curto e objetivo: fase atual, o que já está pronto, e o
   próximo passo concreto — **sem começar a implementar**; se o próximo passo
   for não-trivial, sugerir entrar em plan mode (reforça o "Plan Node Default"
   que já está no `CLAUDE.md`, não o substitui).

Não inclui geração de plano nem execução — é só orientação/diagnóstico, o que
mantém o comando pequeno e de baixo risco (não pode "errar" implementando nada).

## Testes

Nenhum destes componentes é código Go executado em produção pelo `ray` — são
tooling de repositório. Verificação é manual/funcional, não testes automatizados:

- `make ci` roda sem erro no estado atual do repo (build+test+vet+fmt já
  passam, conforme verificado nesta sessão).
- `make fmt-check` falha propositalmente se um arquivo mal formatado for
  introduzido (smoke test manual antes de considerar pronto).
- `/proxima-fase`, ao ser invocado, produz um resumo que bate com o estado real
  (Fase 1 concluída, Fase 2 — `profile` — é o próximo passo) — validação por
  inspeção, não por assert automatizado.

## Pronto quando

- `make build`, `make test`, `make vet`, `make fmt-check`, `make ci` existem e
  passam no estado atual do repo.
- `tasks/todo.md` e `tasks/lessons.md` existem com a estrutura descrita.
- `.claude/commands/proxima-fase.md` existe e, ao ser invocado numa sessão do
  Claude Code, resume corretamente o estado do projeto.

**Commit sugerido:** `chore(dev-tooling): Makefile, tasks/ tracking, /proxima-fase command`

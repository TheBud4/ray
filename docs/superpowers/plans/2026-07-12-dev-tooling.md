# Dev Tooling (Makefile, tasks/, /proxima-fase) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the three repo-tooling pieces approved in
`docs/superpowers/specs/2026-07-12-dev-tooling-design.md`: a `Makefile` for
build/quality, a `tasks/` directory for task tracking, and a
`.claude/commands/proxima-fase.md` slash command that summarizes project
phase progress.

**Architecture:** Three independent, non-Go artifacts. No source code changes,
no new packages, no dependencies. Each task produces one self-contained file
(or pair of files) verifiable by manual inspection/execution, per the spec's
"Testes" section (this is repo tooling, not application code — no Go test
files are added).

**Tech Stack:** `make` (POSIX Makefile), Markdown (tasks/lessons templates,
Claude Code slash command).

## Global Constraints

- No `Co-Authored-By` trailer in any commit (per `ray-build-guide.md` and
  `ray-reproducible-environments-plan.md` commit policy).
- No `golangci-lint` or other new dependency — `go vet` + `gofmt` only (spec
  decision).
- No GitHub Actions workflow file — CI wiring is deferred to build guide Fase
  11 (spec decision).
- No `PROGRESS.md` or other new persistent tracking file beyond what's
  specified (spec decision).
- Every commit must leave `go build ./...` and `go test ./...` green (existing
  project-wide rule, `ray-build-guide.md` §14).

---

### Task 1: Makefile

**Files:**
- Create: `Makefile`

**Interfaces:**
- Consumes: nothing (standalone tooling file).
- Produces: `make build|install|test|vet|fmt|fmt-check|ci` targets, usable by
  Task 2/3 verification steps and by any future CI workflow (Fase 11).

- [ ] **Step 1: Write the Makefile**

Create `Makefile` at the repo root with exactly this content:

```makefile
.PHONY: build install test vet fmt fmt-check ci

build:
	go build ./...

install:
	go install .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to be run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

ci: fmt-check vet test
```

- [ ] **Step 2: Verify each target runs clean on the current tree**

Run, in order, and confirm each exits `0` with no unexpected output:

```bash
make build
make vet
make fmt-check
make test
make ci
```

Expected: all five commands exit `0`. `fmt-check` prints nothing (current
tree is already `gofmt`-clean).

- [ ] **Step 3: Verify `fmt-check` actually catches bad formatting**

```bash
printf 'package main\nfunc  main(){}\n' > /tmp/badfmt_scratch.go
cp /tmp/badfmt_scratch.go ./zz_badfmt_scratch.go
make fmt-check; echo "exit code: $?"
rm ./zz_badfmt_scratch.go /tmp/badfmt_scratch.go
```

Expected: `make fmt-check` prints `./zz_badfmt_scratch.go` and the wrapper
echoes `exit code: 1`. After the `rm`, confirm `make fmt-check` is clean again
(exit `0`, `git status` shows no leftover file).

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore(build): add Makefile (build/install/test/vet/fmt/fmt-check/ci)"
```

---

### Task 2: `tasks/` tracking files

**Files:**
- Create: `tasks/todo.md`
- Create: `tasks/lessons.md`

**Interfaces:**
- Consumes: nothing.
- Produces: the two files the project's root `CLAUDE.md` already references
  under "Task Management" (`Plan First` → `tasks/todo.md`; `Capture Lessons`
  → `tasks/lessons.md`). No other task depends on their content, only their
  existence at these exact paths.

- [ ] **Step 1: Create `tasks/todo.md`**

Create `tasks/todo.md` with exactly this content:

```markdown
# Todo

> Reescrito a cada nova tarefa — não é histórico. O histórico de decisões e
> aprendizados vive nos commits e em `tasks/lessons.md`.

## Tarefa

_(nenhuma tarefa em andamento)_

## Plano

- [ ]

## Revisão

_(preenchida ao final da tarefa, resumindo o que foi feito e verificado)_
```

- [ ] **Step 2: Create `tasks/lessons.md`**

Create `tasks/lessons.md` with exactly this content:

```markdown
# Lessons

> Append-only. Cada entrada: o padrão, o porquê, quando se aplica.

## [2026-07-12] Commits sem Co-Authored-By

**Padrão:** nunca incluir o trailer `Co-Authored-By: Claude` (ou qualquer
co-autor de IA) nas mensagens de commit deste repositório.

**Por quê:** política explícita do usuário — `ray-build-guide.md` e
`ray-reproducible-environments-plan.md` já documentam isso como regra de
commit ("SEM trailer `Co-Authored-By`").

**Quando se aplica:** todo commit feito neste repositório, sem exceção.
```

- [ ] **Step 3: Verify structure**

```bash
test -f tasks/todo.md && test -f tasks/lessons.md && echo OK
grep -q "^## Tarefa" tasks/todo.md && echo "todo.md OK"
grep -q "^## \[2026-07-12\]" tasks/lessons.md && echo "lessons.md OK"
```

Expected: `OK`, `todo.md OK`, `lessons.md OK` all print.

- [ ] **Step 4: Commit**

```bash
git add tasks/todo.md tasks/lessons.md
git commit -m "chore(tasks): add tasks/todo.md and tasks/lessons.md tracking files"
```

---

### Task 3: `.claude/commands/proxima-fase.md`

**Files:**
- Create: `.claude/commands/proxima-fase.md`

**Interfaces:**
- Consumes: `docs/ray-build-guide.md` §14 (phase list) and
  `docs/ray-reproducible-environments-plan.md` (increments I1–I9) as its
  reference material — read at invocation time, not at plan time.
- Produces: a Claude Code slash command `/proxima-fase`. Nothing else depends
  on this file; it's a leaf.

- [ ] **Step 1: Create the command file**

Create `.claude/commands/proxima-fase.md` with exactly this content:

```markdown
---
description: Resume o estado do projeto ray em relação ao plano de fases e sugere o próximo passo
---

Você foi invocado para orientar sobre onde o projeto `ray` está no plano de
construção. Siga estes passos, na ordem:

1. Leia `docs/ray-build-guide.md`, seção "## 14. Plano de fases (ordem de
   construção)" — a lista de Fases 0 a 12, cada uma com o que entrega.
2. Leia `docs/ray-reproducible-environments-plan.md` — os incrementos I1 a
   I9, que só passam a fazer sentido depois que o build guide (Fases 0–12)
   estiver implementado.
3. Rode `git log --oneline` e liste os arquivos existentes em `internal/`
   (`find internal -type f`). Cruze as mensagens de commit e os pacotes
   existentes com as fases do build guide para inferir o que já está pronto.
4. Rode `go build ./...` e `go test ./...` para confirmar que o estado atual
   está verde antes de sugerir o próximo passo.
5. Devolva um resumo curto e objetivo, nesta ordem:
   - **Fase atual concluída** (número + nome, ex. "Fase 1 — runner").
   - **O que já está pronto** (lista curta de pacotes/arquivos existentes e
     por que contam como aquela fase).
   - **Próximo passo concreto** (a próxima fase/incremento, o pacote a criar,
     e o que ele precisa fazer, citando a seção correspondente do documento
     fonte).
6. **Não implemente nada.** Se o próximo passo for não-trivial (3+ passos ou
   decisão arquitetural — ver `CLAUDE.md`, "Plan Node Default"), termine
   sugerindo entrar em plan mode para ele. Este comando é só diagnóstico.
```

- [ ] **Step 2: Dry-run the procedure manually**

Since a slash command can't be invoked as an automated test, manually walk
through steps 1–4 of the command's own instructions right now, in this
session:

```bash
git log --oneline
find internal -type f | sort
go build ./... && go test ./...
```

Confirm the output matches what the command should conclude: last commit is
`feat(runner): command execution boundary...`, `internal/` contains
`cmd/*.go` and `runner/*.go` only, build and test are green. Confirm this
maps to "Fase 1 — runner concluída, Fase 2 — profile é o próximo passo" per
`ray-build-guide.md` §14. If the command's instructions would lead to any
other conclusion, fix the wording in Step 1 before proceeding.

- [ ] **Step 3: Commit**

```bash
git add .claude/commands/proxima-fase.md
git commit -m "feat(claude): add /proxima-fase command to summarize build-guide progress"
```

---

## Final verification

- [ ] Run `make ci` one more time after all three commits — must exit `0`.
- [ ] Run `git log --oneline -5` and confirm three new commits exist, none
  with a `Co-Authored-By` trailer.
- [ ] Run `git status` — working tree clean, nothing untracked left over.

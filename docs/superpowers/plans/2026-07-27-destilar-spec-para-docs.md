# `/destilar` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scaffoldar um comando `/destilar NNN` que lê uma spec fechada no cérebro e atualiza o `docs/` do repo com o que ela virou estado, e desfazer o desenho anterior de publicação de spec.

**Architecture:** Tudo é template Markdown renderizado pelo `internal/scaffold`. Nenhuma lógica nova em Go — o `ray` não autora conteúdo, entrega o prompt. As mudanças em Go são de fiação: entradas no mapa `templateFor` (`internal/scaffold/scaffold.go`) e na lista `baseScaffoldFiles` (`internal/profile/defaults.go`), com os testes de espelho correspondentes.

**Tech Stack:** Go 1.25, `text/template`, `embed.FS`, testes com `testing` puro e `t.TempDir()`.

## Global Constraints

- Idioma dos templates e comentários: **português (pt-BR)**. O repo está todo em pt-BR; não trocar.
- Mensagens de commit em **inglês**, Conventional Commits — é o padrão do histórico (`feat(brain):`, `feat(scaffold):`).
- **Nunca** incluir trailer `Co-Authored-By` em commit.
- **Nunca** `git push` sem pedido explícito. Commit sim, push não.
- `git add` sempre seletivo, nunca `git add .`/`-A`.
- Placeholders em template são `…` (reticências unicode), **nunca** `{{ }}` — `{{` colide com o `text/template` do Go e o scaffold não parseia.
- `CLAUDE.md.tmpl` tem orçamento de **300 linhas**. Está em 235; a Task 3 acrescenta ~4.
- Gate a cada commit: `go build ./...`, `go vet ./...`, `go test ./...` — os três verdes, 20 pacotes.
- A vault `MegaBrain` **não é repositório git**. A Task 5 não tem commit.

## File Structure

| Arquivo | Responsabilidade | Task |
|---|---|---|
| `internal/scaffold/templates/claude/commands/destilar.md.tmpl` | **criar** — o prompt do comando | 1 |
| `internal/scaffold/scaffold.go` | fiação: `templateFor` ganha `.claude/commands/destilar.md`, perde `docs/specs/TEMPLATE.md` | 1, 4 |
| `internal/profile/defaults.go` | fiação: `baseScaffoldFiles` idem | 1, 4 |
| `internal/scaffold/scaffold_test.go` | espelho: `wantBasePaths`, teste de conteúdo do comando, remoção do teste da spec | 1, 2, 3, 4 |
| `internal/profile/defaults_test.go` | espelho: `wantPaths` | 1, 4 |
| `internal/scaffold/templates/docs/README.md.tmpl` | **reescrever** o laço: termina em destilação | 2 |
| `internal/scaffold/templates/CLAUDE.md.tmpl` | passo 8 do `<workflow>` aponta para `/destilar` | 3 |
| `internal/scaffold/templates/docs/specs/TEMPLATE.md.tmpl` | **deletar** | 4 |
| `MegaBrain/Sistema/Templates/Template - Feature.md` | corrigir gatilho, absorver as 3 seções que a destilação consome | 5 |

Ordem deliberada: o comando nasce primeiro (Task 1), os documentos passam a referenciá-lo (Tasks 2 e 3), e só então o artefato morto sai (Task 4) — assim nenhum passo intermediário deixa referência pendurada.

---

### Task 1: Criar o comando `/destilar` e fiá-lo no scaffold

**Files:**
- Create: `internal/scaffold/templates/claude/commands/destilar.md.tmpl`
- Modify: `internal/scaffold/scaffold.go` (mapa `templateFor`, ~linha 38)
- Modify: `internal/profile/defaults.go` (slice `paths` em `baseScaffoldFiles`, ~linha 53)
- Test: `internal/scaffold/scaffold_test.go` (`wantBasePaths` ~linha 15; teste novo no fim)
- Test: `internal/profile/defaults_test.go` (`wantPaths` ~linha 53)

**Interfaces:**
- Consumes: `WriteFiles(files []profile.ScaffoldFile, opts Options) (Result, error)` e `Options{Target string, Data Data}`, ambos já existentes em `internal/scaffold`.
- Produces: a chave de path `".claude/commands/destilar.md"`, válida em `templateFor` e em `baseScaffoldFiles`. As Tasks 2 e 3 citam esse path em texto de template.

- [ ] **Step 1: Escrever o teste que falha**

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestDestilarCommandCarriesLoadBearingRules(t *testing.T) {
	target := t.TempDir()

	if _, err := WriteFiles([]profile.ScaffoldFile{{Path: ".claude/commands/destilar.md"}}, Options{
		Target: target,
		Data:   Data{ProjectName: "demo", Stack: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(target, ".claude", "commands", "destilar.md"))
	if err != nil {
		t.Fatal(err)
	}
	txt := string(body)

	// Cada string abaixo é uma regra que, se sumir, muda o que o comando faz.
	// O portão de status impede destilar plano; a regra de evidência é o que
	// impede docs/ de nascer mentindo; "espere OK" protege a decisão travada;
	// "não invente trabalho" impede relatório inflado quando não há nada.
	for _, s := range []string{
		"status",
		"implementada",
		"não vira texto",
		"espere OK",
		"Não invente trabalho",
	} {
		if !strings.Contains(txt, s) {
			t.Errorf("destilar.md perdeu a regra %q", s)
		}
	}
}
```

E acrescente `".claude/commands/destilar.md"` a `wantBasePaths` (`internal/scaffold/scaffold_test.go`, ~linha 15), logo depois de `".claude/commands/revisar.md"`:

```go
var wantBasePaths = []string{
	"CLAUDE.md",
	"SECURITY.md",
	"docs/README.md",
	"docs/architecture.md",
	"docs/conventions.md",
	"docs/specs/TEMPLATE.md",
	".claude/commands/document.md",
	".claude/commands/handoff.md",
	".claude/commands/revisar.md",
	".claude/commands/destilar.md",
	".claude/handoff.md",
}
```

E em `internal/profile/defaults_test.go`, dentro de `TestDefaultsScaffoldFiles`, a mesma entrada em `wantPaths`, na mesma posição:

```go
		".claude/commands/revisar.md",
		".claude/commands/destilar.md",
		".claude/handoff.md",
```

- [ ] **Step 2: Rodar os testes e conferir que falham**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ ./internal/profile/ -run 'TestDestilar|TestWriteFilesCreatesBaseSet|TestDefaultsScaffoldFiles' -v`

Expected: FAIL. Em `internal/scaffold`: `scaffold: no template for path ".claude/commands/destilar.md"`. Em `internal/profile`: `len(Scaffold.Files) = 10, want 11`.

- [ ] **Step 3: Criar o template do comando**

Crie `internal/scaffold/templates/claude/commands/destilar.md.tmpl` com exatamente este conteúdo:

````markdown
---
description: Destila uma spec fechada no cérebro para o docs/ deste repositório
argument-hint: NNN (número da spec)
---

# /destilar $ARGUMENTS — spec fechada vira estado no repo

Lê a spec `$ARGUMENTS` no cérebro e atualiza o `docs/` deste repositório com o
que ela transformou em **estado**. A spec não viaja: ela fica no cérebro para
sempre. O que viaja é o que ela causou.

## Regra de ouro

`docs/` é estado atual. Objetivo, motivação, escopo e o enredo da implementação
são processo e não entram. Se você está prestes a copiar um parágrafo inteiro da
spec, pare: destilar é reescrever o que ficou verdade sobre o sistema, não
transportar texto.

## Passo 1 — ler a spec e checar o portão

Leia a spec pelo MCP `brain`. Se o `status` não for `implementada`, **pare**:
não se destila plano. Diga qual é o status e encerre.

## Passo 2 — extrair os candidatos

Só estas seções produzem estado:

| Seção da spec | Destino |
|---|---|
| Regras de negócio e invariantes | `docs/architecture.md` |
| Contratos | `docs/architecture.md` |
| Estratégia de teste específica | `docs/conventions.md` — só se virou regra permanente |
| Decisões durante a implementação | a decisão travada vai para `<architecture>` do `CLAUDE.md`; o enredo do desvio fica no cérebro |

O resto não entra: Requisitos funcionais já viraram código, Critérios de aceite
já viraram testes `CA-NN`, e Objetivo, Fora de escopo, Dependências e impacto e
Perguntas em aberto são processo.

## Passo 3 — confirmar cada candidato no código

A superfície a inspecionar são os arquivos que a spec nomeia em **Dependências e
impacto**, mais o que `git status` e `git diff` mostrarem por commitar. Se as
duas coisas estiverem vazias, **pergunte** qual é o escopo — não adivinhe.

Para cada candidato, ache a evidência concreta: o símbolo do contrato, o teste
`CA-NN` correspondente, ou o trecho que implementa a invariante.

Candidato sem evidência **não vira texto**. Reporte: "a spec diz X, o código não
mostra". É isso que impede o `docs/` de nascer mentindo — a spec descreve o que
se pretendia, o código descreve o que é.

## Passo 4 — escrever

- `docs/architecture.md` e `docs/conventions.md`: **aplique** e mostre o diff.
  É descrição, e contradizer o que estava lá é o esperado: o estado mudou.
- Decisão travada no `CLAUDE.md`: **proponha o texto e espere OK.** Nunca
  escreva sozinho — a categoria existe justamente para não ser reaberta sem
  perguntar, e um agente que a reescreve calado esvazia a categoria.

Se a spec **contradiz** uma decisão travada que já está registrada, **pare**. Ou
a spec estava errada, ou a decisão precisava ter sido reaberta antes de
implementar. Os dois casos são do usuário, não seus.

Escreva como fato, sem vocabulário de processo: "a navegação usa X", nunca "a
spec 012 decidiu X".

## Passo 5 — reportar

- o que entrou, e em qual arquivo
- o que foi recusado por falta de evidência no código
- a proposta de decisão travada, se houver

Se não havia nada a destilar, diga isso. **Não invente trabalho.**
````

- [ ] **Step 4: Fiar o template no scaffold**

Em `internal/scaffold/scaffold.go`, no mapa `templateFor`, acrescente a linha depois da do `revisar`:

```go
	".claude/commands/revisar.md":       "claude/commands/revisar.md.tmpl",
	".claude/commands/destilar.md":      "claude/commands/destilar.md.tmpl",
```

Em `internal/profile/defaults.go`, no slice `paths` de `baseScaffoldFiles`, na mesma posição:

```go
		".claude/commands/revisar.md",
		".claude/commands/destilar.md",
		".claude/handoff.md",
```

- [ ] **Step 5: Rodar os testes e conferir que passam**

Run: `cd /home/thebud4/www/Projetos/ray && go build ./... && go vet ./... && go test ./...`

Expected: PASS, 20 pacotes `ok`.

- [ ] **Step 6: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/templates/claude/commands/destilar.md.tmpl \
        internal/scaffold/scaffold.go internal/scaffold/scaffold_test.go \
        internal/profile/defaults.go internal/profile/defaults_test.go
git commit -F - <<'EOF'
feat(scaffold): add the /destilar command

Closing a spec leaves its consequences scattered: an invariant that is now true
of the system, a contract the code exposes, a decision nobody may reopen. Step 8
of the workflow says to carry those into docs/, and depends on remembering.

/destilar NNN reads the closed spec from the brain and does it. The spec never
travels - it stays in the brain. Only what it caused moves.

Candidates come from four sections and each one must be confirmed against the
code before it becomes text; a candidate with no evidence in the diff is
reported rather than documented. Descriptions in docs/ are applied directly.
A locked decision is only ever proposed, and a spec that contradicts one stops
the command instead of rewriting it.
EOF
```

---

### Task 2: Reescrever o laço do `docs/README.md`

**Files:**
- Modify: `internal/scaffold/templates/docs/README.md.tmpl` (linhas 21-31, seção "O laço"; e linhas 9-19, a tabela dos três papéis)
- Test: `internal/scaffold/scaffold_test.go` (teste novo no fim)

**Interfaces:**
- Consumes: o path `.claude/commands/destilar.md` produzido pela Task 1.
- Produces: nada consumido por tarefas posteriores.

- [ ] **Step 1: Escrever o teste que falha**

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestDocsReadmeLoopEndsInDistillation(t *testing.T) {
	target := t.TempDir()

	if _, err := WriteFiles([]profile.ScaffoldFile{{Path: "docs/README.md"}}, Options{
		Target: target,
		Data:   Data{ProjectName: "demo", Stack: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(target, "docs", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	txt := string(body)

	if !strings.Contains(txt, "/destilar") {
		t.Error("o laço precisa terminar em /destilar")
	}
	// A spec fica no cérebro. Se o README voltar a mandar publicá-la em
	// docs/specs/, o laço contradiz a regra de roteamento.
	if strings.Contains(txt, "docs/specs/") {
		t.Error("docs/README.md não pode mandar publicar spec em docs/specs/")
	}
}
```

- [ ] **Step 2: Rodar o teste e conferir que falha**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ -run TestDocsReadmeLoopEndsInDistillation -v`

Expected: FAIL, duas mensagens — `o laço precisa terminar em /destilar` e `docs/README.md não pode mandar publicar spec em docs/specs/`.

- [ ] **Step 3: Reescrever as duas seções**

Em `internal/scaffold/templates/docs/README.md.tmpl`, substitua a tabela "Os três papéis" e a seção "O laço" por:

````markdown
## Os dois papéis

| Arquivo | Muda com que frequência | Quem lê |
|---|---|---|
| `CLAUDE.md` (raiz) | Quase nunca — só em decisão de arquitetura | A IA, em todo turno |
| `architecture.md` · `conventions.md` | A cada spec que muda o sistema | Quem chega no projeto |

As specs **não moram aqui**. Elas nascem, amadurecem e ficam no cérebro — são
processo. Esta pasta é estado: o que alguém que clonasse o repo amanhã
precisaria ler para entender o sistema como ele é hoje.

## O laço

```
1. A spec nasce no cérebro, a partir do template de spec de lá.
2. Preencher. Deixar "Perguntas em aberto" honesta — é aqui que a spec ganha ou perde.
3. Revisar juntos. A spec só vira status: aprovada quando "Perguntas em aberto" está VAZIA.
4. Abrir o turno: "implemente a spec NNN". O agente lê a spec direto do cérebro,
   pelo MCP brain — ela não precisa estar no repo.
5. Ciclo RED-GREEN-REFACTOR, um CA por vez (workflow no CLAUDE.md).
6. Fechar: status: implementada + "Decisões durante a implementação" com o que divergiu.
7. Destilar com /destilar NNN: o que a spec virou estado entra em docs/; o resto
   fica no cérebro.
8. Revisar antes do commit com /revisar NNN.
```
````

Na seção final "Os outros arquivos", troque a linha de fecho para deixar explícito
o vínculo com a destilação:

```markdown
Ambos descrevem estado, não intenção. Se um deles ficar defasado do código, ele
virou ficção — corrija junto com a mudança que o defasou, ou deixe o `/destilar`
fazer isso ao fechar a spec.
```

- [ ] **Step 4: Rodar os testes e conferir que passam**

Run: `cd /home/thebud4/www/Projetos/ray && go build ./... && go vet ./... && go test ./...`

Expected: PASS, 20 pacotes `ok`.

- [ ] **Step 5: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/templates/docs/README.md.tmpl internal/scaffold/scaffold_test.go
git commit -F - <<'EOF'
docs(scaffold): the loop ends in distillation, not publication

The loop described a fourth step that published the approved spec into
docs/specs/. That step existed because the agent needed the file inside the
repo, and the brain MCP integration removed the need. It also contradicted the
routing rule this same folder states: docs/ is current state, and a spec is a
plan or a record.

The loop now runs from the brain end to end and closes with /destilar NNN.
EOF
```

---

### Task 3: Apontar o passo 8 do workflow para o `/destilar`

**Files:**
- Modify: `internal/scaffold/templates/CLAUDE.md.tmpl` (passo 8 do `<workflow>`, ~linhas 129-133)
- Test: `internal/scaffold/scaffold_test.go` (teste novo no fim)

**Interfaces:**
- Consumes: o path `.claude/commands/destilar.md` produzido pela Task 1.
- Produces: nada consumido por tarefas posteriores.

- [ ] **Step 1: Escrever o teste que falha**

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestWorkflowStep8DelegatesToDestilar(t *testing.T) {
	target := t.TempDir()

	if _, err := WriteFiles([]profile.ScaffoldFile{{Path: "CLAUDE.md"}}, Options{
		Target: target,
		Data:   Data{ProjectName: "demo", Stack: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(target, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	txt := string(body)

	if !strings.Contains(txt, "/destilar") {
		t.Error("o passo de fechamento da spec precisa delegar ao /destilar")
	}
	// Orçamento do cabeçalho: o arquivo entra em contexto em todo turno.
	if n := strings.Count(txt, "\n"); n > 300 {
		t.Errorf("CLAUDE.md = %d linhas, orçamento é 300", n)
	}
}
```

- [ ] **Step 2: Rodar o teste e conferir que falha**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ -run TestWorkflowStep8DelegatesToDestilar -v`

Expected: FAIL com `o passo de fechamento da spec precisa delegar ao /destilar`. A asserção de orçamento passa (o arquivo tem ~235 linhas).

- [ ] **Step 3: Reescrever o passo 8**

Em `internal/scaffold/templates/CLAUDE.md.tmpl`, substitua o passo 8 do `<workflow>`. Texto atual:

```
8. **Fechar a spec** (ao terminar todos os CAs, não a cada CA) — marque
   `status: implementada` e preencha "Decisões durante a implementação" com o que
   divergiu do planejado. Se a spec mudou o estado do sistema, atualize `docs/`
   (`architecture.md`, `conventions.md`) no mesmo ciclo; se mudou uma decisão
   travada ou a <stack>, atualize a seção aqui. Sem esse passo a spec não está
   fechada, mesmo com os gates verdes.
```

Texto novo:

```
8. **Fechar a spec** (ao terminar todos os CAs, não a cada CA) — marque
   `status: implementada` e preencha "Decisões durante a implementação" com o que
   divergiu do planejado. Depois rode `/destilar NNN`: ele leva para `docs/` o que
   a spec virou estado e propõe o que mudou de decisão travada. Sem comando
   disponível, faça à mão o mesmo trabalho. Sem esse passo a spec não está
   fechada, mesmo com os gates verdes.
```

- [ ] **Step 4: Rodar os testes e conferir que passam**

Run: `cd /home/thebud4/www/Projetos/ray && go build ./... && go vet ./... && go test ./...`

Expected: PASS, 20 pacotes `ok`.

- [ ] **Step 5: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/templates/CLAUDE.md.tmpl internal/scaffold/scaffold_test.go
git commit -F - <<'EOF'
feat(scaffold): closing a spec now delegates to /destilar

Step 8 told the agent to carry the spec's consequences into docs/ by hand,
which is exactly what the new command does. Two instructions for one job, and
the manual one is the one the agent meets first.

The step now names the command and keeps the manual route as the fallback for
when no command is available.
EOF
```

---

### Task 4: Remover `docs/specs/TEMPLATE.md` do scaffold

**Files:**
- Delete: `internal/scaffold/templates/docs/specs/TEMPLATE.md.tmpl`
- Modify: `internal/scaffold/scaffold.go` (remover a entrada do `templateFor`, ~linha 38)
- Modify: `internal/profile/defaults.go` (remover do slice `paths`, ~linha 59)
- Test: `internal/scaffold/scaffold_test.go` (remover de `wantBasePaths`; apagar `TestSpecTemplateKeepsLoadBearingSections`, ~linhas 403-438)
- Test: `internal/profile/defaults_test.go` (remover de `wantPaths`, ~linha 59)

**Interfaces:**
- Consumes: nada.
- Produces: nada. Esta task só remove.

- [ ] **Step 1: Escrever o teste que falha**

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestSpecTemplateIsNotScaffolded(t *testing.T) {
	// A spec vive no cérebro; o template dela também. Scaffoldar um
	// TEMPLATE.md no repo cria uma segunda fonte que diverge da primeira.
	if _, ok := templateFor["docs/specs/TEMPLATE.md"]; ok {
		t.Error("docs/specs/TEMPLATE.md não deve mais ser scaffoldado")
	}
}
```

- [ ] **Step 2: Rodar o teste e conferir que falha**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ -run TestSpecTemplateIsNotScaffolded -v`

Expected: FAIL com `docs/specs/TEMPLATE.md não deve mais ser scaffoldado`.

- [ ] **Step 3: Remover as cinco referências e o arquivo**

Em `internal/scaffold/scaffold.go`, apague do mapa `templateFor` a linha:

```go
	"docs/specs/TEMPLATE.md":            "docs/specs/TEMPLATE.md.tmpl",
```

Em `internal/profile/defaults.go`, apague do slice `paths` a linha:

```go
		"docs/specs/TEMPLATE.md",
```

Em `internal/profile/defaults_test.go`, apague de `wantPaths` a linha:

```go
		"docs/specs/TEMPLATE.md",
```

Em `internal/scaffold/scaffold_test.go`, apague de `wantBasePaths` a linha:

```go
	"docs/specs/TEMPLATE.md",
```

e apague a função `TestSpecTemplateKeepsLoadBearingSections` inteira (de `func TestSpecTemplateKeepsLoadBearingSections(t *testing.T) {` até a chave de fechamento). Ela testava o artefato que está saindo.

Apague o template:

```bash
cd /home/thebud4/www/Projetos/ray
git rm internal/scaffold/templates/docs/specs/TEMPLATE.md.tmpl
```

- [ ] **Step 4: Rodar os testes e conferir que passam**

Run: `cd /home/thebud4/www/Projetos/ray && go build ./... && go vet ./... && go test ./...`

Expected: PASS, 20 pacotes `ok`.

- [ ] **Step 5: Conferir que não sobrou referência**

Run: `cd /home/thebud4/www/Projetos/ray && grep -rn "docs/specs" --include="*.go" --include="*.tmpl" internal/`

Expected: saída vazia.

- [ ] **Step 6: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/scaffold.go internal/scaffold/scaffold_test.go \
        internal/profile/defaults.go internal/profile/defaults_test.go \
        internal/scaffold/templates/docs/specs/TEMPLATE.md.tmpl
git commit -F - <<'EOF'
feat(scaffold): stop scaffolding a spec template into the repo

Specs live in the brain, and so does the template they are written from.
Scaffolding a second copy into every repo created a source that would drift
from the one people actually use, for an artefact that no longer travels.
EOF
```

---

### Task 5: Corrigir o template de spec na vault

**Files:**
- Modify: `/home/thebud4/www/MegaBrain/Sistema/Templates/Template - Feature.md`

**Interfaces:**
- Consumes: o mapeamento seção→destino que o `/destilar` implementa (Task 1). As seções acrescentadas aqui são exatamente as que o comando lê.
- Produces: nada em código.

**Nota:** `MegaBrain` **não é repositório git** — não há commit nesta task. Se o Obsidian estiver aberto, prefira `vault_write` pelo MCP `obsidian` para o app reindexar na hora; se estiver fechado, escreva pelo filesystem.

- [ ] **Step 1: Conferir o estado atual**

Run: `cat "/home/thebud4/www/MegaBrain/Sistema/Templates/Template - Feature.md"`

Expected: o callout `> [!info] Spec` contém a frase `aprovada` é o gatilho de publicação para o `docs/` do repositório — que é a afirmação falsa a corrigir. E o arquivo **não** tem as seções `## Contratos`, `## Regras de negócio e invariantes` nem `## Decisões durante a implementação`.

- [ ] **Step 2: Corrigir o callout**

Substitua a linha do callout que fala em publicação por:

```markdown
> [!info] Spec
> `status`: `rascunho` → `aprovada` → `implementada` (ou `descartada`). Uma spec só vira `aprovada` quando **Perguntas em aberto** estiver vazia. A spec **não é publicada no repositório** — ela fica aqui. Ao fechar, `/destilar NNN` leva para o `docs/` do repo só o que ela virou estado.
```

- [ ] **Step 3: Acrescentar as três seções que a destilação consome**

Acrescente ao template, depois de `## Fora de escopo`, na ordem abaixo. Os textos de orientação são curtos de propósito — a regra da vault é que seção que nasce vazia não entra, e estas nascem com instrução:

````markdown
## Regras de negócio e invariantes

O que precisa ser verdade **sempre**, não só no fluxo acima. É a seção que vira
`docs/architecture.md` na destilação — escreva como afirmação sobre o sistema.

- …

## Contratos

Cole o real: tipos, JSON, schema, assinatura, evento. Descrição em prosa vira
alucinação. Também vai para `docs/architecture.md`.

```…
…
```

## Decisões durante a implementação

Preencher **depois**, ao fechar a spec. Só o que divergiu do planejado e por quê.
A decisão que ficou travada vai para o `CLAUDE.md` do repo; o enredo do desvio
fica aqui.

- …
````

- [ ] **Step 4: Conferir que o template casa com o que o comando lê**

Run:
```bash
for s in "## Regras de negócio e invariantes" "## Contratos" "## Decisões durante a implementação" "/destilar"; do
  printf "%-45s " "$s"
  grep -cF "$s" "/home/thebud4/www/MegaBrain/Sistema/Templates/Template - Feature.md"
done
```

Expected: cada linha termina em `1`.

- [ ] **Step 5: Conferir que a afirmação falsa sumiu**

Run: `grep -c "gatilho de publicação" "/home/thebud4/www/MegaBrain/Sistema/Templates/Template - Feature.md"`

Expected: `0`.

---

## Verificação final

- [ ] `cd /home/thebud4/www/Projetos/ray && go build ./... && go vet ./... && go test ./...` — 20 pacotes `ok`
- [ ] `git log --oneline -4` mostra os quatro commits das Tasks 1-4
- [ ] `git log -4 --format=%B | grep -ci co-authored` devolve `0`
- [ ] Nenhum push foi feito — o usuário pede quando quiser
- [ ] `wc -l internal/scaffold/templates/CLAUDE.md.tmpl` ≤ 300

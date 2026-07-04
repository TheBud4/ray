# `ray` — Plano de Implementação: Ambientes de IA que viajam com o repo

> **Origem:** `ray-reproducible-environments-design.md` (aprovado 2026-06-28).
> **Base:** `ray-build-guide.md` (a CLI já reconstruída por fases). Este plano
> assume a base existente e descreve os **incrementos** que realizam o design.
> **Regra de ouro:** cada incremento deixa `go build ./...` e `go test ./...`
> verdes; commit por incremento; TDD onde há lógica; **sem** co-autor de IA.
> **Política de commits:** sem trailer `Co-Authored-By`.

---

## 0. Visão geral e ordem

Os incrementos seguem a dependência natural (dados → efeitos → fiação → UX):

```
I1  Vendoring do .claude/ (.gitignore + project-local + origem/licença)  [base]
I2  Aquisição + cache (Acquirer git/CLI + internal/store)               dep. I1
I3  ray update (re-aquisição + fork por conteúdo)                        dep. I1,I2
I4  Token Economy (internal/economy + mecanismos)                       independente
I5  ray stats (métricas-proxy)                                          dep. I4
I6a Learn: máquina verificável (nível/marcos/diário-runtime)            dep. scaffold
I6b Learn: conteúdo de ensino (prompt socrático/escada) — iterável      dep. I6a
I7  /tutorial (comando de geração in-session)                           dep. I6b
I8  UX: first-run, fix inline, rodapé, ray status                       dep. I1,I4
I9  Docs, README, validação manual                                     por último
```

Dependências: I1 → {I2,I3,I8} · I2 → I3 · I4 → {I5,I8} · I6a → {I6b,I7} · tudo → I9.
I4 e I6a podem começar em paralelo a I1–I3.

> **v2 (2026-07-01).** I2 deixa de ser "só cache" e passa a modelar a
> **aquisição por fonte** (`Acquirer`: `GitAcquirer`/`CliAcquirer`); I3 troca a
> salvaguarda por-git-status por **detecção de fork por conteúdo** (hash
> pristino); I1 ganha **captura de origem+licença**; I6 é **fatiado** em máquina
> verificável (I6a, testável) e conteúdo de ensino (I6b, iterável, sem gate de
> release). O `--level` é input de geração; o diário é local/gitignored.

Cada incremento abaixo lista: **objetivo**, **mudanças por pacote**, **testes**,
**pronto quando**, e o **commit** sugerido.

---

## I1 — Vendoring do `.claude/` (a base do design)

**Objetivo.** Parar de tratar o conteúdo de IA como efêmero: ele instala
project-local e é **commitado**; o `.gitignore` separa dependência capturada de
saída gerada. (Design §2, §3, §5.)

**Mudanças por pacote.**
- `internal/scaffold` — gerar/atualizar o `.gitignore` com a **regra-mãe**:
  - **NÃO ignorar** (whitelist): `.claude/skills/`, `.claude/agents/`,
    `.claude/commands/`, `.claude/settings.json`, `.mcp.json`, `docs/`, e os
    artefatos de compliance por componente (`**/.ray-origin`, `**/LICENSE`
    vendorizados — §5.3 do design).
  - **Ignorar** (runtime/segredos/pessoal): `graphify-out/`,
    `.claude/.ray-metrics/` (ver I5), `.claude/.local/` (diário do learn —
    pessoal, I6a), `.env`, `*.local`, e o bloco por stack (`node_modules/`,
    `.next/`, `.dart_tool/`, `/<bin>`, `build/`).
  - Estrutura: template base + bloco por perfil (campo opcional na receita, como
    o build guide §15.1 prevê). `.sh`/handoff inalterados.
- `internal/installer` — componentes de conteúdo (`via: skills`/`aitmpl`)
  resolvem **project-local por padrão**; `--global` deixa de ser o caminho dos
  componentes de projeto e passa a ser reservado para conteúdo **pessoal**
  cross-project (Design §3.3). Ajustar `componentCommand`/`Options` conforme.
- `internal/initai` — o resumo (`Summary`) já lista `Created`; garantir que o
  conteúdo project-local apareça como material a commitar (gancho p/ I8 rodapé).

**Testes.**
- `scaffold`: `.gitignore` gerado whitelista o conteúdo de IA e blacklista
  runtime; idempotente; bloco por stack correto por perfil.
- `installer`: componente de conteúdo gera comando project-local; `--global` só
  marca conteúdo pessoal (table-driven, sem rede).

**Pronto quando.** `ray init ai` num `t.TempDir()` produz `.claude/` com conteúdo
e um `.gitignore` que deixaria esse conteúdo ser commitado, mas ignora
`graphify-out/` e segredos.

**Commit.** `feat(scaffold,installer): vendor .claude content (commit it; gitignore separates runtime)`

---

## I2 — Aquisição por fonte + cache (`internal/acquire` + `internal/store`)

**Objetivo.** Aquisição **por fonte** (git direto onde dá, CLI onde ajuda) sobre
um cache content-addressed agnóstico ao adquiridor; `init ai` rápido e offline;
cache-first puro. (Design §5.1, §5.2, §13.)

**Mudanças por pacote.**
- **Novo `internal/acquire`** — o contrato e as duas implementações:
  - `type Acquirer interface { Acquire(ctx, comp) (dir, prov string, err error);
    Key(comp) string }`.
  - `GitAcquirer` (`via: git`): resolve `repo@ref#path` via `runner` (git/tarball),
    extrai o subdir; **pinado pelo `ref`**; captura `LICENSE`+`.ray-origin` (§5.3).
    MVP: GitHub shorthand + layout padrão `skills/<nome>/` (YAGNI no resto).
  - `CliAcquirer` (`via: skills|aitmpl`): embrulha os installers **forçando
    `--copy`** (senão vendoriza symlink!) e **desligando telemetria**
    (`DO_NOT_TRACK=1`/`DISABLE_TELEMETRY=1`); captura por localização
    determinística; busca a `LICENSE` do repo de origem uma vez.
- **Novo `internal/store`** — content-addressed em `~/.ray/store/<hash>/`,
  **agnóstico ao adquiridor** (só guarda bytes):
  - `Put(coord, dir) (hash, error)` — hasheia (`sha256` sobre `(rel-path,
    conteúdo)` ordenado); índice `coordenada → hash` com coord **namespaced**
    (`git:…` / `skills:…` / `aitmpl:…`).
  - `Get(coord) (dir, ok)`, `Has(coord) bool`. Só FS, sem rede.
- `internal/raypaths` — `StoreDir()`.
- `internal/profile` — `Component` ganha `via: git` com `Repo/Ref/Path`;
  `Validate` cobre a nova trilha.
- `internal/initai` — laço de conteúdo: **cache-first** (`store.Get` → restaura);
  senão `Acquirer.Acquire` (via `runner`) e `store.Put`. **MCP não passa pelo
  store** (é ferramenta — trilha `.mcp.json`).

**Testes.**
- `store`: `Put`/`Get` round-trip; hash estável; dedup (mesma coord = uma
  entrada; git e CLI com mesmos bytes = uma entrada); coord ausente → `ok=false`.
- `acquire`: `GitAcquirer` resolve `repo@ref#path` (FakeRunner) e captura
  origem/licença; `CliAcquirer` **inclui `--copy`** e `DO_NOT_TRACK` no comando
  (regressão explícita — o symlink-default já mordeu); sem-licença → `prov`
  marcado desconhecido + aviso.
- `initai` (fumaça): 1ª run popula; 2ª com cache quente **não** re-adquire.

**Pronto quando.** Um componente `via: git` pinado num `ref` materializa bytes
determinísticos + `LICENSE`/`.ray-origin`; o mesmo conteúdo em dois projetos
baixa uma vez; a 2ª é offline; nenhum symlink aparece no `.claude/`.

**Commit.** `feat(acquire,store): per-source acquisition (git/CLI, --copy, no-telemetry) over content-addressed cache`

---

## I3 — `ray update` (re-aquisição + detecção de fork por conteúdo)

**Objetivo.** Atualizar ferramentas (latest) e **re-adquirir** conteúdo pelo
`Acquirer` de cada componente, **protegendo edições por conteúdo** (não por
git-status). (Design §6, §13.)

**Mudanças por pacote.**
- **Novo `cmd/update.go`** (+ lógica em pacote próprio):
  - ferramentas: `uv tool upgrade` (headroom, graphify) + MCP via npx — `runner`.
  - conteúdo: re-`Acquire` respeitando o `ref` (pinado = no-op; `main` =
    re-resolve HEAD). Atualiza `store` e reescreve `.claude/`.
  - **detecção de fork por conteúdo:** o `store` guarda o **hash pristino** por
    projeto×componente (metadado local, nada no projeto). Antes de sobrescrever,
    compara o hash do que está no disco com o pristino: `iguais` → sobrescreve;
    `diferem` → **você editou** → pula com aviso, exige `--force`. Protege até
    edições **commitadas**.
  - **degradação graciosa** (clone novo, sem pristino local): re-adquire o `ref`
    pinado e compara; se `main` (ambíguo), cai no antigo "avisa + mostra diff".
  - **guard de árvore limpa** (ortogonal): exige working tree limpo (ou `--force`)
    para o diff ficar legível. Update em massa é **por componente**.
- `internal/store` — API de pristino (`PristineHash(proj, coord)` / `SetPristine`).

**Testes.**
- `update`: com FakeRunner, roda os upgrades esperados; disco == pristino →
  sobrescreve; disco != pristino → pula sem `--force`, prossegue com `--force`;
  clone-novo sem pristino → re-deriva do `ref` pinado; respeita `--dry-run`.

**Pronto quando.** `ray update` sobrescreve o pristino, mas **pula** um
componente que você editou (mesmo commitado) até `--force`.

**Commit.** `feat(update): re-acquire per ref; content-based fork detection (protects committed edits)`

---

## I4 — Token Economy (`internal/economy` + mecanismos)

**Objetivo.** Promover `headroom`/`code_graph` (flags soltas) + handoff
(embutido) a **mecanismos de uma categoria** com contrato comum. (Design §8.1,
§8.2.)

**Mudanças por pacote.**
- **Novo `internal/economy`**:
  - `type Mechanism struct { Name, Kind string; Install []runner.Command;
    Server *mcp.Server; MetricKey string }` (Design §8.2).
  - `Mechanisms(prof) []Mechanism` — deriva a lista a partir das integrações
    ligadas (graphify→code-graph, headroom→context-compression) + handoff
    (sempre, kind `builtin`/`hook`).
- `internal/installer` — `Resolve` passa a montar os steps de economia **a
  partir** de `economy.Mechanisms` (em vez de `if Headroom { … }` espalhado),
  mantendo a tabela §6 do build guide como a implementação concreta de cada
  mecanismo.
- `internal/profile` — `Integrations.Headroom`/`CodeGraph` continuam no YAML
  (compat), mas a lógica passa a ler via `economy`. (Refactor sem mudar o YAML.)

**Testes.**
- `economy`: perfil com headroom+graphify → 3 mecanismos (incl. handoff) com os
  campos corretos; perfil sem nenhum → só o handoff.
- `installer`: os comandos/servers gerados continuam idênticos à tabela §6
  (regressão — o refactor não muda a saída).

**Pronto quando.** `installer` produz exatamente os mesmos comandos de antes,
agora derivados de `economy.Mechanisms`.

**Commit.** `refactor(economy): model token-saving as pluggable mechanisms`

---

## I5 — `ray stats` (métricas-proxy leves)

**Objetivo.** Tornar a economia visível e provável lendo proxies de atividade
que já existem. Só proxies (tokens como gancho futuro). (Design §8.3, §13.)

**Mudanças por pacote.**
- `internal/scaffold` — os mecanismos que emitem métrica escrevem num local
  conhecido: `.claude/.ray-metrics/` (ignorado no git — ver I1). O hook de
  handoff incrementa um contador; documentar onde os MCP servers logam.
- **Novo `cmd/stats.go`** + leitura em `internal/economy` (ou `internal/metrics`):
  - agrega por `MetricKey`: consultas ao grafo, compressões, handoffs injetados.
  - **honestidade na UI:** reporta atividade; tokens só se houver telemetria
    acessível, rotulados *estimativa*. Sem telemetria → omite tokens. (Gancho
    `--estimate-tokens` experimental, desligado por padrão.)
- `internal/scaffold` hook de `session-start` — registrar a injeção de handoff
  como evento contável.

**Testes.**
- `stats`: dado um diretório-fixture com `.claude/.ray-metrics/` populado,
  agrega e formata as contagens corretas; sem o diretório, mostra "sem dados
  ainda"; nunca imprime tokens sem telemetria.

**Pronto quando.** `ray stats` num projeto com métricas-fixture imprime as
contagens de atividade; sem dados, mensagem clara.

**Commit.** `feat(stats): aggregate proxy activity metrics (honest, no fake tokens)`

---

## I6a — Learn: máquina verificável (nível, marcos, diário-runtime)

**Objetivo.** A parte **testável, no domínio do `ray`**: validação de `--level`,
marcos verificáveis, e a máquina do diário (head/log + local/gitignored). O
conteúdo de ensino (prompt socrático/escada) sai daqui para o **I6b** — que é
iterável e não trava um release. (Design §9.1–§9.3, revisão v2.)

**Mudanças por pacote.**
- `internal/scaffold/templates` — `rules/learning-journal.md.tmpl`: estrutura do
  diário (**head vivo** + **log append-only**), o que capturar (deltas de
  entendimento) e o que não. **O diário é pessoal → local** (`.claude/.local/`
  gitignored, ou `~/.ray/learn/<projeto>`); só o head é injetado no
  `session-start`. (O prompt de ensino em si é conteúdo do **I6b**.)
- `internal/scaffold/mode.go` — `--level` (default `intermediário`) entra no
  `Data` do render (usado pelo I6b); o `session-start` injeta **só o head** do
  diário, do local pessoal.
- `internal/cmd/init_ai.go` (+ `new.go`) — nova flag `--level
  iniciante|intermediário|avançado`, **válida só com `--mode learn`** (erro/aviso
  caso contrário); valor assado no scaffold (Design §9.1).
- `internal/profile` — campo **opcional** `Milestones []Milestone{Goal, Verify}`
  na receita (Design §9.3); validação leve (goal+verify não-vazios quando
  presentes).
- **Novo `internal/learn`** (ou em `runfile`) — rodar o `verify` de um marco via
  `runner`, rastrear o marco corrente e registrar a passagem no diário; a
  checagem de compreensão é instrução do prompt (não código), mas o *registro*
  no diário é mediado por MCP/escrita do vault.

**Testes.**
- `scaffold`: overlay learn escreve o prompt de ensino e o template do diário; a
  variante muda com `--level`; o guard de código continua bloqueando
  `lib/main.dart` e liberando `docs/x.md` (regressão).
- `profile`: receita com `milestones` válida; marco sem `verify` → erro.
- `learn`: `verify` de um marco roda via FakeRunner; passagem registrada.
- `cmd`: `--level` sem `--mode learn` → erro/aviso; com learn, valor propagado.

**Pronto quando.** `ray init ai --mode learn --level iniciante` produz um
`.claude/` com o prompt de ensino da variante certa, o template do diário, e
(se a receita tiver) marcos rodáveis.

**Commit.** `feat(learn): level validation, verifiable milestones, journal machine (head/log, local)`

---

## I6b — Learn: conteúdo de ensino (iterável, sem gate de release)

**Objetivo.** O **conteúdo** que o `ray` scaffolda mas não consegue verificar por
código: o prompt de ensino socrático, a **escada de dicas** (6 degraus; aluno
puxa, IA escala se travar, degrau 6 gateado por confirmação) e a checagem de
compreensão nos marcos. Itera sem release — trocar o texto é editar template, não
recompilar lógica. (Design §9.1.)

**Mudanças por pacote.**
- `internal/scaffold/templates` — `rules/learn-teaching.md.tmpl` com o protocolo
  da escada e a checagem; **variante por `--level`** (via `{{.Level}}` ou três
  arquivos + `templateFor`). O `--level` (I6a) apenas **seleciona a variante** —
  é input de geração, não estado persistido; o que viaja é o conteúdo gerado.
- `internal/scaffold/mode.go` — incluir o template no overlay learn.

**Testes.**
- `scaffold`: overlay learn escreve o prompt de ensino; a variante muda com
  `--level`; o guard de código continua bloqueando `lib/main.dart` e liberando
  `docs/x.md` (regressão).

**Pronto quando.** `ray init ai --mode learn --level iniciante` produz o prompt
da variante certa; o texto é editável sem mexer em Go.

**Commit.** `feat(learn): socratic teaching prompt (hint-ladder, level variants) as scaffolded content`

---

## I7 — `/tutorial` (geração de tutorial in-session)

**Objetivo.** Comando que dispara a IA a gerar um tutorial 0→100% do projeto,
guiado por prompt scaffoldado; saída em `docs/`. O `ray` entrega o prompt; a IA
gera. (Design §9.4, §9.5.)

**Mudanças por pacote.**
- `internal/scaffold/templates` — `commands/tutorial.md.tmpl` (análogo ao
  `commands/document.md` existente): o **prompt específico** que orienta a IA a
  produzir fundamentos → fases → "pronto quando…", **calibrado pelo nível**
  (§9.1), com instrução de salvar em `docs/` e referenciar os `milestones`
  (§9.3) como checkpoints quando existirem.
- `internal/scaffold/mode.go` — incluir `commands/tutorial.md` no overlay learn.

**Testes.**
- `scaffold`: overlay learn escreve `commands/tutorial.md`; conteúdo reflete o
  nível; presença condicionada ao modo learn.

**Pronto quando.** Num projeto learn, existe `.claude/commands/tutorial.md` com o
prompt; rodá-lo é trabalho da IA (fora do escopo de teste do `ray`).

**Commit.** `feat(learn): /tutorial command scaffolds the in-session tutorial-generation prompt`

---

## I8 — UX e primeira experiência

**Objetivo.** First-run caloroso, fix de deps inline, rodapé "próximos passos",
`ray status`. (Design §10.)

**Mudanças por pacote.**
- `internal/cmd/root.go` — `RunE` do root sem subcomando: tela de "começando"
  (status de deps via `preflight` silencioso + próximo comando sugerido + alerta
  se faltar required). `ray --help` inalterado. (§10.1)
- `internal/cmd/init_ai.go` + `internal/preflight` — quando faltam required e há
  **TTY interativo**, oferecer *"Instalar agora? [S/n]"* e rodar os `Check.Fix`;
  sem TTY ou com `--yes`, comportamento não-interativo. Reusa `preflight` (fonte
  única). (§10.2)
- `internal/initai` — `Summary` ganha **rodapé "Próximos passos"**: dentro de um
  repo git, instrui `git add .claude && git commit` + `claude`; fora de repo,
  omite o git. (§10.3) — **fecha o loop do vendoring** (I1).
- **Novo `cmd/status.go`** — lê `.claude/` + `.mcp.json` + versões de ferramentas
  + **estado git do `.claude/`** (limpo/sujo/untracked = drift). Mostra
  skills/agents/commands, MCP servers, mecanismos de economia ativos (§8.5 →
  reusa `economy`). (§10.4)

**Testes.**
- `cmd` (fumaça, com `RAY_HOME` em `t.TempDir()` + FakeRunner):
  - root sem args imprime a tela de começando (não o help cru);
  - `init ai` com dep faltando: `--yes` instala sem prompt; sem TTY aborta com
    dica;
  - rodapé aparece só dentro de repo git (simular presença de `.git`);
  - `status` lê um `.claude/` fixture e reporta presença + estado git.

**Pronto quando.** A jornada "instalei → `ray` → `ray new go app`" orienta em
cada passo; `ray status` mostra o ambiente e o drift.

**Commit.** `feat(cmd): warm first-run, inline dep-fix, next-steps footer, ray status`

---

## I9 — Docs, README e validação manual

**Objetivo.** Documentar o novo modelo e validar ponta-a-ponta.

**Mudanças.**
- `README.md` — seção sobre **vendoring** (commitar `.claude/`), `--mode learn
  --level`, `ray status`/`ray stats`/`ray update`, e `ray completion`.
- Atualizar o `ray-build-guide.md` nas seções listadas no Design §11 (§7, §15.1,
  §5/§6, §16, §9, §4.1, §13) — para o guia refletir o design.
- **Validação manual** (`RAY_HOME=$(mktemp -d)`): `ray` (first-run), `ray doctor`,
  `ray new go /tmp/x` → inspecionar `.claude/` commitável + `.gitignore` (git
  status: conteúdo de IA *staged*, `graphify-out/` ignorado); `ray init ai
  --mode learn --level iniciante` → conferir prompt de ensino + diário +
  `/tutorial`; 2º projeto reusa o cache offline; `ray status`/`ray stats`;
  `ray update` (drift recusado sem `--force`).

**Pronto quando.** Tudo verde, README atualizado, validação manual ok.

**Commit.** `docs: reproducible AI environments (vendoring, learn levels, stats/update)` ·
`chore: align build guide with the reproducible-environments design`

---

## Apêndice — invariantes a não reaprender (do build guide + deste design)

- Pacote PyPI do grafo: **`graphifyy`** (CLI `graphify`); MCP `graphify-mcp`;
  registrar `--platform claude`. headroom: extra `[mcp]`; server `headroom mcp`.
- `.claude/handoff.md` é intocável por `--force`. O **diário** (I6) segue a mesma
  filosofia: head vivo injetado, log fora do contexto.
- `.claude/.ray-metrics/` é **runtime** → sempre no `.gitignore`, mesmo estando
  sob `.claude/` (cujo conteúdo, no resto, é commitado).
- Conteúdo de IA é **project-local e commitado**; `--global` = só conteúdo
  pessoal cross-project.
- `init ai` é **cache-first** (offline); trazer o latest é **explícito** via
  `ray update` (que protege drift não-commitado).
- `ray` **não autora conteúdo**: a geração de tutorial é da **IA in-session**; o
  `ray` só entrega o prompt scaffoldado.
- Métricas são **proxies honestos**; nunca imprimir tokens **nem causalidade**
  sem medição real.
- **Aquisição por fonte** (v2): `via: git` → `GitAcquirer` (pinado por `ref`,
  preferido p/ fontes oficiais); `via: skills|aitmpl` → `CliAcquirer` **com
  `--copy` e telemetria off** (symlink-default vendoriza link quebrado!). O
  `store` é **agnóstico** ao adquiridor.
- **MCP nunca entra no `store`** — é ferramenta (latest + config no `.mcp.json`).
- `ray update` protege por **hash de conteúdo pristino** (não por git-status);
  protege edições commitadas; degrada com graça em clone novo.
- Todo conteúdo vendorizado carrega **`.ray-origin` (`repo@ref`) + LICENSE**
  (commitados = compliance); fonte sem licença → aviso soft, sem bloqueio.
- Learn: `--level` é **input de geração**; **diário é local/gitignored**; só o
  conteúdo de ensino é commitado.

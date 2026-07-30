# `ray` — Guia de Implementação (construção do zero, à mão)

> **Propósito deste documento.** Reconstruir o `ray` inteiro, manualmente, sem
> "vibecoding". É um retrato fiel do que o projeto já era (capturado do código,
> não da memória) **mais** os pontos que faltavam documentar e as melhorias a
> incorporar nesta segunda volta. Auto-contido: dá pra apagar o repositório
> antigo e construir só com isto.
>
> **Stack:** Go 1.25 · Cobra · `gopkg.in/yaml.v3` · módulo `github.com/TheBud4/ray`.
> **Política de commits:** SEM trailer `Co-Authored-By` (nenhum co-autor de IA).
>
> **Alinhamento com o design v2 (2026-07-01).** As seções afetadas pelo modelo de
> ambientes reprodutíveis estão marcadas ⟶v2 (aquisição por fonte, `--copy`,
> store agnóstico, update por fork-de-conteúdo, origem/licença).

---

## 1. Objetivo do produto

`ray` é uma CLI pessoal em Go que (a) cria projetos novos de um stack e (b)
monta, com um comando, um ambiente de desenvolvimento com IA (Claude Code)
**econômico em tokens** e **rico em ferramentas**.

A feature âncora é **`ray init ai`**, que estabelece num diretório:

1. **Economia de tokens** — `headroom` (compressão de contexto) como MCP + grafo
   de código consultável (`graphify`), pra IA não reler arquivos.
2. **Ferramentas que ampliam a IA** — skills, agentes, comandos e MCPs curados
   por stack, instalados a partir de ecossistemas abertos.
3. **Praticidade** — scaffold em camadas (memória, conhecimento, guard-rails,
   delegação), docs-base, regras que guiam/restringem a IA, e handoff de
   continuidade entre sessões.

**Princípio de design central:** o `ray` **não embute conteúdo de componentes**.
Ele **orquestra installers externos** (`npx skills`, `npx claude-code-templates`,
`uv tool install`). A curadoria vive nas **receitas** (`~/.ray/profiles/*.yaml`),
não no binário.

---

## 2. Decisões técnicas

- **Cobra** dá a árvore de subcomandos, flags, `--help`, `version` e `completion`.
- **Orquestrador guiado por receita**: cada "fonte de instalação" é mapeada em
  comandos por uma função pura; executar é só a fronteira `runner`. Sem engine
  de plugins em runtime (YAGNI).
- **Uma única fronteira de processos externos**: o pacote `runner`. Todo
  `npx`/`uv`/`git`/`exec` passa por ele → testes usam um `FakeRunner` e nunca
  tocam rede.
- **Idempotência em todo lugar**: profiles/templates default só são escritos se
  faltarem; `.mcp.json`/`settings.json` fazem merge sem destruir; globais
  "install-once" são rastreados em `state.yaml`.
- **Não-sobrescrita** de arquivos de scaffold (exige `--force`); `.claude/handoff.md`
  é a única exceção (sempre gerida pela IA, nunca tocada nem pelo `--force`).

---

## 3. Arquitetura e fronteiras de pacotes

```
ray/
├── main.go                       # chama cmd.Execute()
├── go.mod / go.sum
├── Makefile                      # build/test/vet/fmt/ci
├── .github/workflows/ci.yml      # roda `make ci`
├── .gitignore
├── README.md
└── internal/
    ├── cmd/          # Cobra: root, init, init_ai, new, run, profile, vault, docs, doctor
    ├── runner/       # ÚNICA fronteira de exec. ExecRunner (real, dry-run) + FakeRunner
    ├── profile/      # struct da receita + load/validate + defaults + list/add/remove
    ├── installer/    # receita → Plan{Commands, Globals, Servers} (puro, não executa)
    ├── mcp/          # Server + merge idempotente de .mcp.json
    ├── claudecfg/    # merge de .claude/settings.json
    ├── scaffold/     # renderiza orientation files (go:embed templates) + modos
    ├── vault/        # cria/garante ~/.ray/vault (Obsidian-compatível)
    ├── preflight/    # checagens de deps (fonte única p/ doctor + init ai), c/ Fix
    ├── initai/       # orquestra o fluxo init ai ponta-a-ponta
    ├── runfile/      # aliases do `ray run` (ray.yaml + ~/.ray/commands.yaml)
    ├── rayconfig/    # ~/.ray/config.yaml (Config) + ~/.ray/state.yaml (State)
    ├── raypaths/     # resolve ~/.ray (RAY_HOME override)
    └── openutil/     # abrir caminho no app default (xdg-open/open)
```

**Regras de fronteira (cada peça testável isolada):**
- `installer` não sabe de CLI nem executa — devolve **dados** (`Plan`).
- `profile` só carrega/valida — não executa.
- `runner` é o único que toca processos (`os/exec`).
- `scaffold` só escreve arquivos — não chama processos.
- `cmd`/`initai` orquestram: load receita → resolve plan → runner → mcp/settings → scaffold → resumo.

---

## 4. Modelo de dados

### 4.1 Profile (receita) — `internal/profile`
```go
type Profile struct {
    Name         string
    Description  string
    Integrations Integrations
    Components   []Component
    Scaffold     Scaffold
    Create       []string // templates p/ `ray new` (rodados no novo dir); {{.Name}}
}
type Integrations struct { // YAML keys entre parênteses
    Headroom  bool // headroom
    Brain     bool // brain
    CodeGraph bool // code_graph
}
type Component struct {
    Via    string // "skills" | "aitmpl"
    Skill, Source string // via: skills
    Type, Ref     string // via: aitmpl (type: agent|command|mcp)
}
type Scaffold struct {
    Files    []ScaffoldFile      // {Path, Template?}
    Settings map[string]any      // model, effortLevel, ...
}
```
**Validação** (`Validate`): `name` obrigatório; `via ∈ {skills, aitmpl}`;
`skills` exige `skill`+`source`; `aitmpl` exige `type ∈ {agent,command,mcp}`+`ref`;
todo `ScaffoldFile.Path` não-vazio. Erro claro no primeiro problema.

**Helpers do pacote:** `Load(path)`, `List(dir)→[]Entry`, `EnsureDir(dir)`
(escreve defaults faltantes, nunca sobrescreve), `Starter(name)` (perfil mínimo
válido p/ `profile add`), `WriteNew(dir,p)`, `Remove(dir,name)`.

### 4.2 Config + State — `internal/rayconfig`
- **Config** (`~/.ray/config.yaml`): `brain: <path>`. Override por env
  `RAY_BRAIN`. Helpers `Load/Save/SetBrain/BrainPath`.
- **State** (`~/.ray/state.yaml`): `installed_globals: [keys]`. Rastreia globais
  já instalados. Helpers `LoadState/Save/HasGlobal/AddGlobal`.

### 4.3 Runfile (aliases) — `internal/runfile`
```yaml
commands:
  test: { description: run tests, steps: ["go test ./..."] }
```
`Load(workdir)` mescla **global** (`~/.ray/commands.yaml`) + **projeto**
(`ray.yaml`, achado subindo a árvore a partir de `workdir`). Projeto sobrescreve
global por nome. `Resolved{Name, Description, Steps, BaseDir, Source}` —
`BaseDir` é o dir do `ray.yaml` (projeto) ou o `workdir` (global).

### 4.4 Paths — `internal/raypaths`
`Home()` = `$RAY_HOME` ou `~/.ray`. `ProfilesDir/TemplatesDir/VaultDir` = subpastas.

---

## 5. Árvore de comandos (CLI)

```
ray
├── new <perfil> <nome>     # cria projeto do stack (create + git init) + init ai
├── run [alias]             # executa aliases; sem alias ou --list lista (com origem)
├── init ai                 # monta o ambiente de IA numa pasta existente
├── profile  list | show <n> | add <n> | edit <n> | remove <n> | path
├── vault    init | status | open | path        # ~/.ray/vault (vault da IA)
├── docs     init <path> | set <path> | open | path   # vault central de docs do usuário
└── doctor   [--fix]        # checa deps; --fix instala o que o ray consegue
```
Cobra fornece `version`, `help`, `completion` de graça.

### Flags globais (persistentes, root)
`--verbose`, `--dry-run`.

### `ray init ai` — flags
`--profile` (obrigatório) · `[path]` posicional (default `.`) ·
`--mode build|learn` (default `build`) · `--global` (`-g`, skills globais) ·
`--force` (regenera scaffold) · `--no-global` · `--reinstall-global`.

### `ray new <perfil> <nome>` — flags
Reaproveita as do `init ai` + `--no-git` (pula `git init`). Fluxo: valida alvo
vazio/inexistente → `MkdirAll` → roda `create` (templates `{{.Name}}`) no novo
dir → `git init -q` (salvo `--no-git`) → chama `initai.Run`. Falha num passo de
`create` **aborta** (não montar IA sobre projeto meio-criado).

---

## 6. Integrações — mapeamento receita → comando real

Tabela **exata** ao código (`installer.Resolve` + `integrations.go`). `Plan` tem
três listas: `Commands` (por-projeto, sempre rodam), `Globals` (install-once,
rastreados por `Key` em `state.yaml`), `Servers` (entradas de `.mcp.json`).

| Item da receita | Tipo | Comando / Server gerado |
|---|---|---|
| `via: git` ⟶v2 | Acquirer | `GitAcquirer`: tarball/clone de `<repo>@<ref>`, extrai `<path>`, copia p/ `.claude/` + captura `LICENSE`/`.ray-origin` (preferido p/ fontes oficiais) |
| `via: skills` | Command | `DO_NOT_TRACK=1 npx skills add <source> --skill <skill> -a claude-code --copy -y` (+`-g` se `--global`) ⟶v2: **`--copy` e telemetria off** |
| `via: aitmpl, type: agent\|command\|mcp` | Command | `npx claude-code-templates@latest --<type>=<ref> --yes` (copia arquivos p/ `.claude/agents\|commands/`) |
| `headroom` | Global `headroom` + Server | install: `uv tool install headroom-ai[mcp]` · server `headroom` → `headroom mcp` |
| `brain` | Server (condicional) | só se `ray brain set` configurou path válido: `brain` → `npx -y @modelcontextprotocol/server-filesystem <path>` |
| `code_graph` | Global `code_graph` + Command + Server | global: `uv tool install graphifyy` **e** `graphify install --platform claude` · por-projeto: `graphify update .` (constrói o grafo, tree-sitter, sem LLM) · server `graphify` → `graphify-mcp` (stdio, lê `graphify-out/graph.json`) |

**Notas que valem ouro (já mordidas uma vez):**
- ⟶v2 **`npx skills add` faz *symlink* por padrão**, não cópia — o método
  recomendado aponta cada agente para uma cópia canônica única. Commitar symlink
  **não** viaja com o repo (o conteúdo fica fora). O vendoring **exige `--copy`**.
- ⟶v2 **telemetria ligada por padrão** no `skills add` (envia nome+arquivos da
  skill): desligar sempre com `DO_NOT_TRACK=1`/`DISABLE_TELEMETRY=1`.
- ⟶v2 `via: git` é o caminho preferido p/ repositórios oficiais: pinado por `ref`,
  sem exposição a symlink/telemetria/flag-drift de terceiros.
- O pacote PyPI é `graphifyy` (dois "y"); o binário é `graphify`.
- `headroom-ai[mcp]` precisa do extra `[mcp]` pra expor `headroom mcp`.
- Globais só viram `state.AddGlobal(key)` se **todos** os comandos do step
  saírem 0; `--no-global` pula tudo; `--reinstall-global` ignora o state.
- Servers são **sempre** registrados (por-projeto), mesmo que o global já tenha
  sido instalado antes.

---

## 7. Scaffold em camadas (Agent Development Kit)

Árvore produzida na pasta-alvo (modo `build`, default):

```
<alvo>/
├── CLAUDE.md                 # a base estável: 12 seções XML (ver abaixo)
├── SECURITY.md               # [MUST]/[SHOULD], regras p/ código gerado por IA, checklist de PR
├── .mcp.json                 # headroom + brain? + graphify + componentes
├── docs/                     # o ESTADO ATUAL do projeto (versionado)
│   ├── README.md             # os dois papéis + o laço spec-driven
│   └── architecture.md  conventions.md
└── .claude/
    ├── settings.json         # model, effortLevel, hooks
    ├── handoff.md            # estado vivo (gerido pela IA; NUNCA tocado por --force)
    ├── commands/{destilar,document,handoff,revisar}.md
    ├── hooks/session-start.sh
    ├── agents/  skills/       # populados pelos installers
```

**`CLAUDE.md` é a base estável** (o "00-projeto" do fluxo spec-driven), em 12
seções XML e **nesta ordem**, que é load-bearing e travada por teste:

```
<role> <context> <stack> <documentation_sources> <architecture> <test_strategy>
<conventions> <quality_gates> <workflow> <agent_behavior> <output_format> <edge_cases>
```

`<workflow>` fica perto do fim de propósito — instrução de processo no topo é
atropelada pelo volume da spec colada no turno. `<edge_cases>` fecha o documento:
é a cláusula que impede invenção quando a premissa falha.

Não há mais `.claude/rules/*` no modo `build`: o que era regra solta virou seção
do `CLAUDE.md`. `rules/` sobrou só para o que é opt-in por modo (`learn`) — não
por custo de carregamento (os dois entram no contexto sozinhos, ver abaixo), mas
porque regra que só vale num modo não deve poluir o documento base do outro.

#### Quem carrega `.claude/rules/` (dependência externa, não código do ray)

**O `ray` nunca carrega essas regras — quem carrega é o Claude Code.** Nenhum
arquivo gerado aponta para elas: o `CLAUDE.md.tmpl` não tem seção nem `@import`,
o `session-start.sh` injeta só handoff/diário/marcos, e o `HookSettings` só
registra hooks. Isso é correto, e não um esquecimento: o Claude Code descobre
`.claude/rules/**/*.md` recursivamente e, conforme a documentação oficial,
*"rules without `paths` frontmatter are loaded at launch with the same priority
as `.claude/CLAUDE.md`"*. Os três templates de learn não têm frontmatter, logo
carregam incondicionalmente, toda sessão.

Verificado empiricamente em **Claude Code 2.1.220**, com ferramentas desabilitadas
para excluir leitura por tool call: um canário em `.claude/rules/` volta na
resposta; o mesmo canário em `.claude/naoregras/` não volta; três arquivos de
regra voltam os três; e um scaffold real de `--mode learn` responde que não pode
editar código *citando* `.claude/rules/learn.md`. Para reverificar depois de
mexer nos templates, o caminho barato é o hook `InstructionsLoaded`, que loga
quais arquivos de instrução carregaram e por quê.

Duas consequências que valem estar escritas:

- **Injetar as regras pelo `session-start.sh` seria erro**, não alternativa:
  carregaria em duplicidade e pagaria contexto toda sessão por algo que já vem
  de graça — contra o argumento de custo que o próprio template do diário faz.
- **A regra não substitui o hook.** A doc é explícita: *"Claude treats them as
  context, not enforced configuration. To block an action regardless of what
  Claude decides, use a PreToolUse hook instead."* É exatamente por isso que
  `learn.md` (instrução) e `guard-code.sh` (bloqueio) coexistem — um não torna
  o outro redundante.

Fragilidades conhecidas, ambas fora do caminho padrão: `claudeMdExcludes` pode
excluir o diretório por glob, e `--setting-sources` sem `project` pula as regras
de projeto. Alavanca ainda não usada: o frontmatter `paths` escopa uma regra por
glob, carregando-a só quando a IA toca arquivos que casam — a saída natural se as
regras de learn crescerem.

A regra que sustenta o conjunto: **o que está no `CLAUDE.md` nunca se repete numa
spec.** E todo critério de aceite vira um teste nomeado `CA-NN:`, de modo que
`grep -r "CA-03" test/` responde "isso está implementado?" em um segundo.

**Templates** (`internal/scaffold/templates/*.tmpl`, `go:embed`), placeholders
`{{.ProjectName}}` e `{{.Stack}}`. `EnsureTemplates(~/.ray/templates)` copia os
embutidos como overlay editável; `render` prefere o overlay, cai pro embed.
Mapa `templateFor` liga cada path → arquivo `.tmpl`. Arquivos `.sh` saem `0755`.

**Arquivos "de sistema" (sempre escritos pelo ray, fora da receita):**
`scaffold.SystemFiles(mode)` garante `.claude/hooks/session-start.sh`,
`guard-add.sh`, `guard-vocab.sh` e `guard-plans.sh` (e no `learn`:
`rules/learn.md`, `rules/learn-teaching.md`, `rules/learning-journal.md` +
`hooks/guard-code.sh`). Isso garante que todo hook
referenciado em `settings.json` exista no disco. No `initai`, os arquivos da
receita + os de sistema são deduplicados por path (receita ganha).

### Modos — `--mode build | learn` (overlay ortogonal ao perfil)
- **build** (default): IA implementa normalmente; nenhuma restrição.
- **learn** (mentora/revisora): a IA ensina e revisa mas **não toca em código**.
  Overlay adiciona:
  1. **Bloqueio duro** — hook `PreToolUse` (matcher `Edit|Write|MultiEdit`) →
     `.claude/hooks/guard-code.sh`. Libera só se o path casa a allowlist de docs
     (`*.md`, `docs/*`, `.claude/*` — no `case` do bash o `*` casa `/`, então é
     recursivo); senão **nega** a ação com mensagem. Sem
     `jq`, nega tudo — hook que bloqueia falha fechado. É um **freio de
     reflexo, não um sandbox**: guarda `Edit`/`Write`/`MultiEdit` e não guarda
     `Bash`, então um `bash -c 'cat > x.go'` passa. O limite é deliberado —
     fechar exigiria guardar `Bash`, superfície grande e cheia de falso
     positivo.
  2. **Regra** `.claude/rules/learn.md`.
  3. **Prompt de ensino** `.claude/rules/learn-teaching.md`: contrato negociado
     na primeira sessão, escada de 4 degraus, e a regra de que fatos são
     respondidos direto.
  4. **Diário** `.claude/rules/learning-journal.md` — o diário é da IA e vive
     em `.claude/.local/`; o `ray` só escreve o progresso de marcos.
  5. Viés de agentes p/ review/exploração.

Contrato dos arquivos de `.claude/.local/` (I6a/I6b — nenhum é vendorizado):

| Arquivo | Escrito por | Lido por |
|---|---|---|
| `learning-journal.md` | só a IA | `session-start.sh` (injeta) |
| `milestones-progress.md` | só o `ray` (`ray learn check`) | `session-start.sh` (injeta) |
| `milestones.yaml` | a IA (marcos negociados na sessão) | o `ray` (`LoadMilestones`); ganha da receita quando existe |
| `milestones-passed.yaml` | o `ray` | o `ray` — estado de máquina, não é injetado |

`scaffold.HookSettings(mode)` devolve o bloco `hooks` p/ mesclar no
`settings.json`: `SessionStart` (sempre, injeta o handoff) + `PreToolUse` (só no
learn). As escritas no vault passam por **ferramenta MCP**, não por `Edit/Write`,
então o guard **não** as bloqueia — em `learn` a IA ainda anota aprendizados.

### Handoff entre sessões
`.claude/handoff.md` (estado vivo) + `.claude/rules/handoff.md` (diretiva: antes
de encerrar/limpar, sobrescrever o handoff) + `session-start.sh` (no
`SessionStart`, injeta o handoff na nova sessão).

---

## 8. Fluxo do `ray init ai` (passos exatos — `initai.Run`)

1. Validar `--mode`; resolver `target = Abs(path)`; `ensureWritableDir` (MkdirAll
   + escreve/remove um probe `.ray-write-test`).
2. Resolver `~/.ray` (profiles, templates, vault); `profile.EnsureDir` +
   `scaffold.EnsureTemplates`.
3. `profile.Load(profiles/<profile>.yaml)`.
4. **Preflight** (aborta se faltar required): `needPython = Headroom || CodeGraph`.
   `preflight.MissingRequired` → erro `missing required dependencies ... (run \`ray doctor\`)`.
5. Resolver o cérebro (warning se `brain` ligado mas não configurado ou com
   caminho inválido — nesse caso o server **não** é registrado) e
   `installer.Resolve(prof, Options{Global, BrainPath})`.
7a. **Globais** (`runGlobals`): pula os já em `state.yaml` (salvo `--reinstall-global`);
    `--no-global` pula todos; grava `state` só em execução real.
7b. **Componentes por-projeto**: cada `c.Dir = target`, roda e acumula (falha
    isolada **não** aborta).
8. `mcp.WriteServers(target, plan.Servers, dryRun, out)` — merge idempotente.
9. `claudecfg.MergeSettings(target, merge(prof.Settings, HookSettings(mode)), …)`.
10. `scaffold.WriteFiles(dedup(prof.Files + SystemFiles(mode)), …)` →
    acumula `Created`/`Skipped`.

**Resumo final + exit code:** `Summary{Installed, Failed, Created, Skipped,
Warnings, HadFailure}`. Exit ≠ 0 se `HadFailure` (uso em scripts/CI). `runOne`
classifica: `err` → Failed; `ExitCode≠0` → Failed; senão Installed.

---

## 9. Layout de `~/.ray` (runtime, override por `$RAY_HOME`)

```
~/.ray/
├── profiles/*.yaml      # receitas editáveis (defaults na 1ª run)
├── templates/*.tmpl     # overlay editável dos templates de scaffold
├── config.yaml          # Config: brain
├── state.yaml           # State: installed_globals[]
└── commands.yaml        # aliases globais do `ray run`
```

O ray não guarda mais um vault próprio em `~/.ray/vault`. O cérebro é a vault
Obsidian que o usuário já mantém; `ray brain set <path>` só a valida e registra.

### Dois destinos de documentação, e a regra é binária
1. **`docs/` do projeto** — o **estado atual**: arquitetura, convenções, como
   rodar, como fazer deploy. Versionado, viaja com o código.
2. **O cérebro** — todo o resto: tarefa, exploração, aprendizado, decisão em
   disputa, spec em aberto. Wired via Filesystem MCP `brain` quando configurado.

A pergunta que decide: *se alguém clonasse o repo amanhã, isto precisaria estar
lá?* Roteamento por `.claude/rules/brain.md` + comando `/document`.

---

## 10. `ray doctor` (e `--fix`)

`preflight.Run(looker, needPython)` é a **fonte única** (usada pelo doctor e pelo
init ai). Cada `Check{Name, Found, Required, Hint, Fix []runner.Command}`:

| Check | Required | Fix (auto) |
|---|---|---|
| `npx` | sim | — (instalar Node.js) |
| `node` | não | — |
| `python3.10+` | sim (se needPython) | — |
| `uv` | sim (se needPython) | `sh -c "curl -LsSf https://astral.sh/uv/install.sh \| sh"` |
| `headroom` | não | `uv tool install headroom-ai[mcp]` |
| `graphify` | não | `uv tool install graphifyy` |

`--fix` roda os `Fix` dos checks faltantes, re-checa e avisa: se `uv` acabou de
ser instalado, reabrir o shell pro PATH pegar. Deps de sistema (node/python) não
têm auto-fix — só a dica. Sem `--fix`, ao faltar required, imprime a dica de
rodar `ray doctor --fix` e sai ≠ 0.

---

## 11. Merge de `.mcp.json` e `settings.json`

- **`mcp.WriteServers`**: lê o `.mcp.json` existente, garante `mcpServers{}`,
  define cada server por nome (substitui o de mesmo nome, preserva o resto),
  reescreve com `MarshalIndent`+`\n`. `dryRun` imprime em vez de gravar. Entrada:
  `{command, args?, env?}`.
- **`claudecfg.MergeSettings`**: aplica settings da receita (`model`,
  `effortLevel`) + bloco `hooks`, preservando chaves existentes; idempotente.

---

## 12. Curadoria default dos perfis (Apêndice A — atual no código)

**Integrações (todos):** as 6 ligadas (`allIntegrations()`).
**Scaffold (todos):** `model: opus`, `effortLevel: high`.

**Base comum (todos):**
| Categoria | Componente | via |
|---|---|---|
| Prompt/IA | `jeffallan/claude-skills@prompt-engineer` | skills |
| Código (review) | `development-tools/code-reviewer` | aitmpl agent |
| Código (debug) | `development-tools/debugger` | aitmpl agent |
| Documentação | `github/awesome-copilot@documentation-writer` | skills |

**`go`** (`create: go mod init {{.Name}}`) — base + `samber/cc-skills-golang@{golang-code-style,
golang-error-handling, golang-design-patterns, golang-performance, golang-testing,
golang-security, golang-documentation}`.

**`web`** (`create: npx create-next-app@latest . --yes`) — base +
`vercel-labs/agent-skills@{react-best-practices, composition-patterns,
writing-guidelines, web-design-guidelines}`, `anthropics/skills@{frontend-design,
webapp-testing}`, `hoodini/ai-agents-skills@owasp-security`.

**`flutter`** (`create: flutter create .`) — base +
`flutter/skills@{flutter-apply-architecture-best-practices, flutter-fix-layout-issues,
flutter-setup-declarative-routing, flutter-add-widget-test, flutter-add-integration-test,
flutter-build-responsive-layout}`, `firebase/agent-skills@firebase-security-rules-auditor`,
`leonxlnx/taste-skill@imagegen-frontend-mobile`.

> Tudo editável. Itens `owner/repo@skill` → `source: owner/repo` + `skill: skill`.

---

## 13. Estratégia de testes

A fronteira `runner` torna tudo testável sem rede. Por pacote:
- **runner:** `FakeRunner` grava chamadas; um único teste real roda `echo`.
- **profile:** load válido/inválido (via desconhecido, campo faltando); `EnsureDir`
  só grava faltantes; list/add/remove.
- **installer:** cada linha da tabela §6 → comando exato (incl. `-g`); globais e
  servers corretos.
- **mcp/claudecfg:** merge cria válido, preserva, não duplica em 2ª aplicação.
- **scaffold:** cria o conjunto build; não-sobrescrita; `--force` regenera mas
  nunca o handoff; overlay learn adiciona regra+guard; teste do `guard-code.sh`
  (libera `docs/x.md`, bloqueia `lib/main.dart`).
- **vault:** `Ensure` cria e é idempotente; `Status` conta `.md`.
- **preflight/doctor:** com `Looker` mockado, aborta sem `npx`; sem `python` só
  se needPython; tabela formatada.
- **initai (fumaça):** ponta-a-ponta com `FakeRunner` em `t.TempDir()`, build e
  learn, respeita `--dry-run`, exit ≠ 0 quando componente falha.
- **runfile:** precedência projeto>global, `findProjectFile` sobe a árvore.

---

## 14. Plano de fases (ordem de construção)

> Cada fase deixa `go build ./...` e `go test ./...` verdes; commit por fase.
> TDD onde há lógica (profile, installer, scaffold, runner, vault, mcp).

```
Fase 0  Bootstrap: git init + .gitignore Go + go mod + Cobra/yaml + main.go +
        root (--verbose/--dry-run) + esqueleto da árvore de comandos.
Fase 1  runner: interface Run(ctx,Command)→Result; ExecRunner (real+dryrun);
        FakeRunner.
Fase 2  profile: structs + Load/Validate + Defaults + EnsureDir + List/Starter/
        WriteNew/Remove.
Fase 3  installer: Resolve → Plan{Commands,Globals,Servers}; componentCommand +
        integrations.go (tabela §6).
Fase 4  mcp + claudecfg: merge idempotente de .mcp.json e settings.json.
Fase 5  vault: Ensure + Status.
Fase 6  scaffold: templates go:embed + EnsureTemplates + WriteFiles (não-sobrescr.)
        + modos (SystemFiles/HookSettings) + guard-code.sh/session-start.sh.
Fase 7  preflight + doctor (com Fix/--fix).
Fase 8  rayconfig (Config+State) + raypaths + initai.Run (os 10 passos) +
        cmd/init_ai.go.
Fase 9  cmd de gestão: profile (list/show/add/edit/remove/path), vault
        (init/status/open/path), docs (init/set/open/path), openutil.
Fase 10 runfile + cmd/run.go (aliases) + cmd/new.go (create+git init+init ai).
Fase 11 Makefile + CI (make ci) + README.
Fase 12 Validação manual num dir descartável (RAY_HOME apontado a temp):
        doctor, brain set, init ai --dry-run, depois real; inspecionar .claude/,
        docs/, .mcp.json, settings.json; testar --mode learn (guard bloqueia código).
```

Dependências: 0 → 1 → 2 → 3 → {4,5,6,7} → 8 → {9,10} → 11 → 12.

---

## 15. Melhorias a INCORPORAR nesta reconstrução (gaps do código antigo)

Estes são pontos que ficaram pendentes/imperfeitos e que devem ser resolvidos já
na segunda volta — documentados aqui pra não se perderem:

1. **`.gitignore` no `ray new` (bug real).** Hoje `ray new` faz `git init` mas
   **não** gera `.gitignore`. Como `graphify update .` cria `graphify-out/` em
   todo projeto, além de `.env`/artefatos de stack, eles seriam commitados por
   engano. **Fazer:** gerar um `.gitignore` no scaffold com, no mínimo:
   `graphify-out/`, segredos (`.env`, `*.local`), e artefatos por stack
   (`/<bin>`, `build/`, `.dart_tool/`, `node_modules/`, `.next/`). Ideal: um
   template base + trecho específico por perfil (campo opcional na receita).

2. **`ray run` com passagem de argumentos.** Hoje os passos rodam sem shell e sem
   args extras. **Fazer:** suportar `ray run test -- -run TestX` (repassa tudo
   após `--` ao último/único passo) e/ou aceitar um passo em forma de string de
   shell (pipes/`&&`) além da lista de passos.

3. **`ray update`.** Nada se auto-atualiza. **Fazer:** comando que roda
   `npx skills update` e `uv tool upgrade` (headroom/graphify), reaproveitando o
   `runner` e reportando como o `doctor`.

4. **Testes do pacote `cmd`.** A camada `internal/cmd` ficou sem testes. **Fazer:**
   testes de fumaça por comando (com `RAY_HOME` em `t.TempDir()` e `FakeRunner`
   injetado), verificando flags, exit codes e saída.

5. **`ray completion` documentado.** Existe (via Cobra) mas não está documentado
   nem facilitado. **Fazer:** seção no README (bash/zsh/fish) e, se valer, um
   `ray completion install` de conveniência.

---

## 16. Comandos de gestão — detalhes

- **`profile list`**: garante defaults, imprime `nome — descrição` (ordenado).
- **`profile show <n>`**: load+valida e imprime componentes resolvidos + comandos
  que rodariam (equivale a um dry-run só do perfil).
- **`profile add <n>`**: `Starter(n)` → `WriteNew` (erro se existe).
  **`edit`** abre no `$EDITOR`; **`remove`** apaga; **`path`** imprime o dir.
- **`brain set <path>`**: aponta para uma vault Obsidian existente. Valida
  (`vault.Verify`) e **nunca cria nem reorganiza** — o usuário é dono do
  cérebro, o ray é consumidor. **`status`**: caminho, existe?, nº de `.md`.
  **`open`/`path`**: app/caminho. Caminho em `config.yaml` (`brain`), override
  `RAY_BRAIN`.
- **`run [alias]`**: sem alias ou `--list` lista (com origem project/global);
  alias inexistente → erro apontando `--list`. Passos rodam em sequência, sem
  shell, abortando no 1º exit≠0; respeita `--dry-run`/`--verbose`.

---

## 17. Erros, segurança, exit codes

- **Abortam no início:** alvo inválido/sem permissão; perfil inexistente; `npx`
  ausente; (se headroom/code_graph) `python3.10+`+`uv` ausentes.
- **Não derrubam o resto:** falha de um componente → `✗` e segue.
- **Exit ≠ 0** se houve qualquer falha.
- **`--dry-run`** imprime tudo, não executa nada.
- **Não-sobrescrita** de scaffold (exceto regenerar com `--force`; handoff nunca).

---

## 18. Makefile / CI / commits

- **Makefile:** `build`, `install`, `test`, `vet`, `fmt`, `fmt-check`,
  `ci` (= fmt-check + vet + test).
- **CI:** GitHub Actions em push/PR rodando `make ci`.
- **Commits:** mensagens **sem** `Co-Authored-By` de IA. Commits pequenos por
  fase; branch antes de mexer se estiver na default.

---

### Apêndice — invariantes que já custaram caro (não reaprender)
- Pacote PyPI do grafo: **`graphifyy`** (CLI `graphify`); registrar com
  `--platform claude`; MCP server é `graphify-mcp`.
- headroom: extra `[mcp]`; server roda `headroom mcp`.
- `.claude/handoff.md` é intocável por `--force`.
- Hook scripts referenciados em `settings.json` precisam existir → por isso
  `SystemFiles` os escreve sempre, fora da receita.
- Globais só entram no `state.yaml` se **todos** os comandos do step deram 0.
- Servers MCP são por-projeto e sempre re-registrados, mesmo com global já feito.
```

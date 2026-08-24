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

A estrutura de pacotes, as regras de dependência e as fronteiras estão em
[`architecture.md`](architecture.md), que é a fonte — descreve o sistema como
ele é hoje e é corrigido junto com a mudança que o defasa. Não repita aqui o
que está lá.

O que este guia acrescenta sobre arquitetura são as **razões** das fronteiras,
e elas estão distribuídas nas seções que as originaram: o `Plan` como dado
puro no §6, o scaffold em camadas no §7, e a ordem dos passos do `init ai` no
§8.

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
    Name string // subpasta em <ComponentsDir>/<Name>, e nome dentro de Dest no projeto
    Dest string // diretório-contêiner relativo ao projeto (".claude/skills", ".claude/agents")
}
type Scaffold struct {
    Files    []ScaffoldFile      // {Path, Template?}
    Settings map[string]any      // model, effortLevel, ...
}
```
**Validação** (`Validate`): `name` obrigatório; `Component.name` e
`Component.dest` obrigatórios; todo `ScaffoldFile.Path` não-vazio. Erro claro
no primeiro problema.

**Servidor MCP não é componente.** Declara-se em `integrations`, e sai no
`.mcp.json` — `plan.Servers` é preenchido por `resolveIntegrations`, nunca a
partir de `Components`.

**v3 — sem aquisição de rede.** Até a v2, `Component` descrevia uma fonte
externa (`via: skills|aitmpl|git`) e um `Acquirer` buscava o conteúdo pela
rede (`npx`, `git clone`) num cache content-addressed. Isso foi removido
inteiro: `internal/acquire` não existe mais, `components:` não aceita mais
`via`. Um componente hoje é só um nome que aponta para
`<ComponentsDir>/<Name>` — uma pasta que o **usuário** mantém à mão, nunca o
`ray`. `init ai`/`update` copiam de lá para `<projeto>/<Dest>/<Name>` pela
mesma política de sobrescrita (`store.DecideOverwrite`) que já protegia os
arquivos de scaffold. Zero rede, zero `npx`/`git` para conteúdo, zero cache de
aquisição — só cópia de arquivo local.

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

Espelha o `root.AddCommand` (`internal/cmd/root.go`) — dez comandos de topo.

```
ray
├── new <perfil> <nome>     # cria projeto do stack (create + git init) + init ai
├── run [alias]             # executa aliases; sem alias ou --list lista (com origem)
├── init ai [path]          # monta o ambiente de IA numa pasta existente
├── profile  list | show <n> | add <n> | edit <n> | remove <n> | path
├── brain    set <path> | status | open | path   # a vault Obsidian do usuário
├── doctor   [--fix]        # checa deps; --fix instala o que o ray consegue
├── update   [path]         # recopia componentes do overlay local e atualiza ferramentas
├── status   [path]         # diagnostica o ambiente vendorizado (drift, forks)
└── stats    [path]         # atividade medida dos mecanismos de Token Economy
```
Cobra fornece `help` e `completion` de graça. **`--version` não é de graça:**
ele só registra a flag quando o comando raiz preenche o campo `Version` — e
enquanto isso não foi feito, `ray --version` respondia *"unknown flag"*. O valor
vem do `debug.ReadBuildInfo` (`internal/cmd/version.go`), não de `-ldflags -X`:
o caminho de instalação documentado é `go install .`, e ldflags só alcança quem
passa a flag, então a versão ficaria `devel` exatamente para quem instalou como
o README manda. Build de tag mostra a versão do módulo; build local mostra
`devel` com revisão, data e — o que mais importa — `dirty` quando a árvore não
estava limpa. **Arquivo não rastreado conta como sujo**, e o excesso é
deliberado: um `.go` novo ainda não commitado entra na compilação, e o
`vcs.modified` não tem como separá-lo de um rascunho solto. Daí a regra que
parece contraintuitiva — um arquivo qualquer largado na raiz marca o binário
como sujo, e a resposta é apagar o arquivo, nunca ignorá-lo no `.gitignore`:
ignorar cegaria o marcador para o caso que ele existe para pegar.

**Não existem `ray vault` nem `ray docs`.** O `brain` os substituiu: o ray deixou
de guardar vault própria em `~/.ray/vault` e passou a apontar para uma vault que
o usuário já mantém (§9, §14). O `internal/vault` sobreviveu como validador —
`Verify`/`Status` —, mas não há mais comando com esse nome.

**`ray` sem subcomando** não cai no help do Cobra: imprime a tela de abertura.
Fora de um projeto ela sugere `ray new` / `ray init ai`; dentro de um
`.claude/`, mostra perfil e inventário e aponta `claude` / `ray status`. Lê só
arquivo — sem git, sem lookup de PATH, sem carregar receita — e sai **0** mesmo
com dependência required faltando, que vira alerta na própria tela. A linha de
fatos é a mesma do `ray status`, servidores MCP incluídos: contá-los é leitura
de arquivo, e duas contagens diferentes para o mesmo projeto faziam as duas
telas mentirem uma sobre a outra. Um `.mcp.json` ilegível derruba o `status`,
que existe para diagnosticar, mas só apaga o número aqui. Não há linha
"deps: ok": a linha de dependência só existe quando falta algo. `ray --help`
segue inalterado, e `cobra.NoArgs` faz comando inexistente errar em vez de cair
na tela. Os comandos-grupo (`init`, `profile`, `brain`) saem do mesmo
esqueleto, e ele precisa de `RunE` além do `Args`: o Cobra checa `Runnable()`
**antes** de validar argumento, então grupo sem `RunE` respondia help e exit 0
a qualquer filho inexistente.

### Flags globais (persistentes, root)
`--verbose` (`-v`), `--dry-run`, e `--version` sem forma curta. O `v` é
declarado no `--verbose` de propósito: o Cobra só o dá ao `--version` se o
encontrar livre, e sem isso `ray -v <comando>` imprimia a versão, não rodava o
comando e saía 0.

### `ray init ai` — flags
`--profile` (obrigatório) · `[path]` posicional (default `.`) ·
`--force` (regenera scaffold) · `--no-global` · `--reinstall-global`.

### `ray new <perfil> <nome>` — flags
Reaproveita as do `init ai` + `--no-git` (pula `git init`). Fluxo: valida alvo
vazio/inexistente → `MkdirAll` → roda `create` (templates `{{.Name}}`) no novo
dir → `git init -q` (salvo `--no-git`) → chama `initai.Run`. Falha num passo de
`create` **aborta** (não montar IA sobre projeto meio-criado).

### Demais flags
`doctor --fix` · `run --list` · `update --profile <n> --force --no-global`. O
`--profile` do `update` sobrescreve a receita gravada em `.claude/.ray-profile`;
o `--force` passa por cima de edição local e de árvore suja; o `--no-global`
pula os `uv tool upgrade`. `status` e `stats` não têm flag própria.

---

## 6. Integrações — mapeamento receita → comando real

Tabela **exata** ao código (`installer.Resolve` + `integrations.go`). `Plan` tem
três listas: `Commands` (por-projeto, sempre rodam), `Globals` (install-once,
rastreados por `Key` em `state.yaml`), `Servers` (entradas de `.mcp.json`).

| Item da receita | Tipo | Comando / Server gerado |
|---|---|---|
| `components: [{name, dest}]` | Cópia local | `store.CopyTree(<ComponentsDir>/<Name>, <projeto>/<Dest>/<Name>)` — sem rede, decide sobrescrever pela mesma política de hash dos arquivos de scaffold |
| `headroom` | Global `headroom` + Server | install: `uv tool install headroom-ai[mcp]` · server `headroom` → `headroom mcp` |
| `brain` | Server (condicional) | só se `ray brain set` configurou path válido: `brain` → `npx -y @modelcontextprotocol/server-filesystem <path>` |
| `code_graph` | Global `code_graph` + Command + Server | global: `uv tool install graphifyy` **e** `graphify install --platform claude` · por-projeto: `graphify update .` (constrói o grafo, tree-sitter, sem LLM) · server `graphify` → `graphify-mcp` (stdio, lê `graphify-out/graph.json`) |

**Histórico (v2, removido — não se aplica mais):** até a v2, `components:`
aceitava `via: git|skills|aitmpl` e um `Acquirer` buscava o conteúdo pela rede
(`npx skills add`, `npx claude-code-templates`, `git clone`), com cache
content-addressed em `internal/store`. `via: git` era o caminho preferido p/
repositórios oficiais, pinado por `ref`, sem exposição a symlink/telemetria/
flag-drift de terceiros — mas as notas sobre symlink do `skills add` e
telemetria eram problemas **daquele** mecanismo, e não existem mais porque o
mecanismo não existe mais.

**Notas que ainda valem:**
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

Não há `.claude/rules/*`: o que era regra solta virou seção do `CLAUDE.md`.
`rules/` existiu só como overlay do modo learn (`learn.md`, `learn-teaching.md`,
`learning-journal.md`) — removido inteiro junto com o modo. Não sobrou nenhum
consumidor de `.claude/rules/`; se a pasta reaparecer um dia, é para outro uso,
não para reviver este.

A regra que sustenta o conjunto: **o que está no `CLAUDE.md` nunca se repete no
documento por feature.** E cada item verificável desse documento vira um teste
que carrega o mesmo identificador, de modo que um `grep` pelo identificador no
diretório de testes responde "isso está implementado?" em um segundo.

**Templates** (`internal/scaffold/templates/*.tmpl`, `go:embed`), placeholders
`{{.ProjectName}}` e `{{.Stack}}`. `EnsureTemplates(~/.ray/templates)` copia os
embutidos como overlay editável; `render` prefere o overlay, cai pro embed.
Mapa `templateFor` liga cada path → arquivo `.tmpl`. Arquivos `.sh` saem `0755`.

**Arquivos "de sistema" (sempre escritos pelo ray, fora da receita):**
`scaffold.SystemFiles()` garante `.claude/hooks/session-start.sh`,
`guard-add.sh`, `guard-vocab.sh`, `guard-plans.sh` e `guard-handoff.sh`. Isso
garante que todo hook referenciado em `settings.json` exista no disco. No
`initai`, os arquivos da receita + os de sistema são deduplicados por path
(receita ganha).

`scaffold.HookSettings()` devolve o bloco `hooks` p/ mesclar no
`settings.json`: `SessionStart` (sempre, injeta o handoff) + `PreToolUse` (os
três guards de aviso) + `PostToolUse` (`guard-handoff`). Mecânica dos quatro
guards, e por que `guard-handoff` é o único `PostToolUse`: `docs/features.md`.

**Removido: modo learn.** Existiu como overlay opt-in (`--mode build|learn`) —
mentora/revisora, hook de bloqueio duro (`guard-code.sh`), escada de ensino de
4 degraus, diário e marcos verificáveis em `.claude/.local/`. O usuário decidiu
usar o `ray` só para desenvolvimento; aprendizado é combinado por fora, sem o
`ray` no meio. `internal/learn`, `internal/cmd/learn.go`, os 4 templates de
`.claude/rules/*`, `guard-code.sh.tmpl`, `profile.Milestone` e a flag `--mode`
saíram inteiros — não há mais overlay ortogonal ao perfil.

### Handoff entre sessões
`.claude/handoff.md` (estado vivo) + `.claude/rules/handoff.md` (diretiva: antes
de encerrar/limpar, sobrescrever o handoff) + `session-start.sh` (no
`SessionStart`, injeta o handoff na nova sessão).

---

## 8. Fluxo do `ray init ai` (passos exatos — `initai.Run`)

1. Resolver `target = Abs(path)`; `ensureWritableDir` (MkdirAll
   + escreve/remove um probe `.ray-write-test`).
2. Resolver `~/.ray` (profiles, templates, store); `profile.EnsureDir` +
   `scaffold.EnsureTemplates`.
3. `profile.Load(profiles/<profile>.yaml)`.
4. **Preflight** (aborta se faltar required): `needPython = Headroom || CodeGraph`.
   `preflight.MissingRequired` → `preflight.MissingRequiredError{From: FromGate}`,
   que renderiza cada dependência faltando com o conselho do `preflight.Advice`
   e fecha com um rodapé apontando `ray doctor`.
5. Resolver o cérebro (warning se `brain` ligado mas não configurado ou com
   caminho inválido — nesse caso o server **não** é registrado) e
   `installer.Resolve(prof, Options{Global, BrainPath})`.
7a. **Globais** (`runGlobals`): pula os já em `state.yaml` (salvo `--reinstall-global`);
    `--no-global` pula todos; grava `state` só em execução real.
7b. **Componentes por-projeto**: cada `c.Dir = target`, roda e acumula (falha
    isolada **não** aborta).
8. `mcp.WriteServers(target, plan.Servers, dryRun, out)` — merge idempotente.
9. `claudecfg.MergeSettings(target, merge(prof.Settings, HookSettings()), …)`.
10. `scaffold.WriteFiles(dedup(prof.Files + SystemFiles()), …)` →
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
├── store/               # cache content-addressed do conteúdo adquirido (I2)
├── config.yaml          # Config: brain
├── state.yaml           # State: installed_globals[]
└── commands.yaml        # aliases globais do `ray run`
```

O `store/` é a linha-base pristina: é contra ela que o `ray update` decide entre
atualizar e preservar, e que o `ray status` diz se um componente foi editado
localmente. Apagá-lo não quebra nada, mas os dois perdem a resposta — passam a
dizer *procedência desconhecida*, que é o que existe para esse caso.

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
têm auto-fix — só a dica.

**O que o ray diz quando falta.** O `Hint` e o `Fix` de cada `Check` não são
decoração: o `preflight.Advice` os traduz numa linha acionável — `ray doctor
--fix` quando o ray sabe instalar sozinho, o `Hint` quando não sabe — e o
`preflight.MissingRequiredError` é a **única** renderização, herdada tanto pelo
gate do `init ai` quanto pelo rodapé do `doctor`. A tabela do `doctor` traz a
mesma coluna `HINT`, preenchida só para o que falta: aconselhar sobre algo
presente é ruído. Sem nada a aconselhar a coluna nem aparece. Faltando required,
sai ≠ 0.

Depois de `ray doctor --fix` rodar, o conselho para uma dependência com `Fix`
deixa de apontar o `--fix`: repetir o comando que acabou de não funcionar é
pior que não dizer nada. É o que o `preflight.Origin` modela, e é por isso que
ele é um tipo de três valores e não um booleano.

A mensagem **não** imprime o comando cru do `Fix` — não vale normalizar
`curl | sh` na saída da ferramenta. Quem quer auditar usa
`ray doctor --fix --dry-run`, que imprime `+ <comando>` sem executar.

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
- **profile:** load válido/inválido (`name`/`dest` faltando no componente,
  campo faltando); `EnsureDir` só grava faltantes; list/add/remove.
- **installer:** cada linha da tabela §6 → comando exato; globais e
  servers corretos.
- **mcp/claudecfg:** merge cria válido, preserva, não duplica em 2ª aplicação.
- **scaffold:** cria o conjunto de sistema; não-sobrescrita; `--force` regenera
  mas nunca o handoff; `guard-handoff.sh` silencioso sob o orçamento e avisando
  acima dele.
- **vault:** `Verify` aceita vault existente e recusa caminho que não é uma;
  `Stat` conta `.md`. Não há teste de criação porque não há criação.
- **preflight/doctor:** com `Looker` mockado, aborta sem `npx`; sem `python` só
  se needPython; tabela formatada.
- **initai (fumaça):** ponta-a-ponta com `FakeRunner` em `t.TempDir()`,
  respeita `--dry-run`, exit ≠ 0 quando componente falha.
- **runfile:** precedência projeto>global, `findProjectFile` sobe a árvore.

---

## 14. Comandos de gestão — detalhes

- **`profile list`**: garante defaults, imprime `nome — descrição` (ordenado).
  **Receita quebrada aparece na lista, marcada** — `(invalid: …)` quando parseia e
  não valida, `(unreadable: …)` quando nem parseia, e aí o nome exibido é o do
  arquivo. Omitir seria esconder: quem não vê o nome não tem o que passar para
  `profile show`, que é onde mora o erro completo com caminho.
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
- **`update [path]`**: atualiza as ferramentas globais (`uv tool upgrade`) e
  re-adquire cada componente, decidindo por **hash de conteúdo** — não por
  git-status. Flags: `--force` (árvore suja e sobrescrita), `--profile`
  (sobrescreve a receita gravada), `--no-global` (pula os passos globais).
  O que a feature garante, e o que o `--dry-run` aplica: `features.md`.
- **`status [path]`**: diagnostica o ambiente vendorizado **deste projeto**, em
  quatro checagens offline (fork, git, `.gitignore`, MCP). O princípio, os três
  níveis de saída e o que cada checagem garante: `features.md`. Aqui ficam só as
  duas mecânicas que não cabem lá:
  - **O escopo de vigilância é derivado das negações do bloco do `.gitignore`**
    — hoje `.claude/` e `.mcp.json` — menos um denylist onde mora o `docs/`, que
    é do usuário. Derivado, e não fixo, porque lista fixa dessincroniza em
    silêncio: a whitelist ganha entrada e o status para de vigiar parte do
    ambiente sem nada falhar. **O `git add` da nota é outra pergunta** e usa
    três fontes unidas, filtradas por existir em disco: a whitelist inteira,
    os arquivos que a receita escreve na raiz, e o `.gitignore` — que entra
    sempre, porque o `ray` sempre o escreve, mesmo com receita ilegível.
    Commitar e vigiar são perguntas diferentes; antes dessa separação o
    `ray status` mandava commitar **menos** que o `ray new` para o mesmo projeto.
  - **A linha de fatos conta conteúdo, não entradas de topo**: uma skill é um
    `SKILL.md`, um agente e um comando são um `.md`, e a contagem desce em
    subdiretório. Contar entradas de diretório fazia um README solto em
    `skills/` valer uma skill, e um grupo de comandos com namespace valer um
    comando só.

  Um caso de degradação que vale registrar: sem `.claude/.ray-profile` a
  checagem de fork inteira cala — não há receita a comparar, e `.claude/`
  copiado à mão é caso normal. Com o registro presente e a receita ilegível é o
  oposto: vira problema, com o erro junto. Os dois casos saíam iguais, e o
  segundo deixava o usuário sem `profile:` na saída e sem pista do porquê.

---

## 15. Erros, segurança, exit codes

- **Abortam no início:** alvo inválido/sem permissão; perfil inexistente; `npx`
  ausente; (se headroom/code_graph) `python3.10+`+`uv` ausentes.
- **Não derrubam o resto:** falha de um componente → `✗` e segue.
- **Exit ≠ 0** se houve qualquer falha.
- **Exceção deliberada: o `ray status` sai 0 mesmo achando problema.** O
  `doctor` erra quando falta dependência *required* porque ali o próximo
  comando quebra de verdade; no `status`, "3 arquivos não commitados" é
  informação, e exit ≠ 0 tornaria o comando inútil em qualquer script que o
  encadeie. Só falha de leitura erra.
- **`--dry-run`** imprime tudo, não executa nada e **não escreve nada**. As duas
  metades precisam ser ditas: o `--dry-run` só alcança o `runner.ExecRunner`,
  então todo caminho que toca o disco fora do runner (`os.MkdirAll`,
  `os.WriteFile`) tem de perguntar por ele à mão. Foi assim que `ray new
  --dry-run` e `ray init ai --dry-run` passaram a criar o diretório-alvo sem
  que nenhum teste percebesse — o que existia usava `t.TempDir()`, um diretório
  que já existe, onde o `MkdirAll` é no-op.
- **Não-sobrescrita** de scaffold (exceto regenerar com `--force`; handoff nunca).

---

## 16. Makefile / CI / commits

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
- **Sem hash pristino, não se afirma "intocado".** O `store.DecideOverwrite`
  precisa do hash *upstream* para decidir sem linha-base, e obtê-lo exige
  rede. Um diagnóstico offline que chamasse a função com o upstream vazio
  receberia "não é fork" — errado, e errado na direção que faz o usuário
  confiar que a edição dele sobrevive. Por isso o `ray status` reporta
  *procedência desconhecida* nesse caso, em vez de palpitar.
```

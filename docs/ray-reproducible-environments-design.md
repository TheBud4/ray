# `ray` — Design: Ambientes de IA que viajam com o repo

> **Status:** aprovado (brainstorming) · **Data:** 2026-06-28 ·
> **Revisão v2:** 2026-07-01 (endurecimento da aquisição/store/update)
> **Escopo:** enriquecimento conceitual do `ray`. Não substitui o
> `ray-build-guide.md`; refina a política de persistência do conteúdo de IA e o
> modelo de compartilhamento. As seções do build guide afetadas estão listadas em
> §11.
>
> **v2 — o que mudou (2026-07-01).** O brainstorm de reforço fechou os pontos
> frágeis do modelo de captura: (a) **aquisição por fonte** — um contrato
> `Acquirer` com `GitAcquirer` (git direto, pinado por `ref`) e `CliAcquirer`
> (embrulha os installers), escolhido *por componente na receita*, priorizando
> repositórios oficiais via git (§5.2); (b) **MCP é ferramenta, não conteúdo** —
> nunca entra no store (§4); (c) `CliAcquirer` **força `--copy` e desliga
> telemetria** (senão o vendoring commita symlinks — §5.1); (d) `ray update`
> passa a **detecção de fork por conteúdo** (hash pristino local), protegendo até
> edições commitadas (§6, §13); (e) **captura de licença + procedência** ao lado
> do conteúdo (§5.3); (f) modo learn: `--level` vira *input de geração* e o
> **diário fica local/gitignored** (§9); (g) correção de honestidade na métrica
> de economia (§8.3).

---

## 1. Contexto e motivação

O `ray` monta, num diretório, um ambiente de desenvolvimento com Claude Code
econômico em tokens e rico em ferramentas (`ray init ai`). Hoje ele é um
**orquestrador puro**: a receita aponta para installers externos
(`npx skills`, `claude-code-templates`, `uv tool install`) e o conteúdo de IA
(skills, agentes, comandos) é instalado no `.claude/` do projeto como algo
**efêmero/reinstalável**.

Essa volatilidade é a fragilidade central da ideia: cada componente é um
**ponteiro** para um upstream que pode mudar de versão ou desaparecer (os próprios
apêndices do build guide listam invariantes frágeis — `graphifyy` com dois "y",
`headroom-ai[mcp]`, repos de skills de donos aleatórios no GitHub). Reinstalar um
projeto antigo pode simplesmente não reproduzir o ambiente original.

Três garantias foram priorizadas para quem recebe um ambiente (você-do-futuro em
outra máquina, um time, a comunidade): **resiliência no tempo**,
**reprodutibilidade** e **simplicidade/portabilidade**. Composabilidade/herança de
receitas foi explicitamente despriorizada.

## 2. A tese enriquecida

> O `ray` para de tratar o conteúdo de IA do projeto como efêmero e passa a
> **commitá-lo no `.claude/` do projeto**, de modo que o ambiente sobreviva ao
> upstream e viaje junto com o código — mantendo o perfil global como a única
> fonte de curadoria.

Uma única mudança de política — **commitar o `.claude/` em vez de ignorá-lo** —
entrega a resiliência sem introduzir nenhum formato novo nem nenhum artefato de
gestão dentro do projeto. É a forma mínima de "dobrar" a premissa
orquestrador-vs-dono: o `ray` continua sem autorar conteúdo; o que muda é que a
**cópia local passa a pertencer ao repo do projeto**, versionada junto com o
código que ela serve. É o análogo direto do `vendor/` do Go.

## 3. Decisões

1. **Fonte única da verdade = perfil em `~/.ray/profiles/*.yaml`.**
   Customização é editar esse YAML, dentro da pasta do `ray`. **Nenhum** arquivo
   de gestão do `ray` é criado no projeto — sem `ray.yaml` de receita, sem
   `ray.lock`. O perfil global é a receita; o projeto só recebe o resultado.

2. **Conteúdo textual instala project-local e é commitado.**
   Skills, agentes e comandos vão para `.claude/skills|agents|commands/` do
   projeto e entram no git. Esse é o vendoring — a fonte da resiliência.
   Consequência: componentes de conteúdo instalam **no projeto**, não com
   `--global`.

3. **`--global` é reservado para conteúdo pessoal cross-project.**
   Skills instaladas com `--global` ficam no `~/.claude` do usuário: são
   pessoais, **não viajam** com o repo e **não** são resilientes-via-repo. Fica
   nítida a divisão: *skill do projeto = commitada; skill pessoal = global*.

4. **Ferramentas sempre na última versão.**
   headroom, graphify e MCP servers (binários/pacotes) **não** são vendorizados
   nem fixados — `init ai` instala o latest de cada um. `ray update` mantém
   atualizado. Reprodutibilidade exata se aplica ao **conteúdo** (commitado), não
   às ferramentas (infraestrutura transparente).

5. **`.gitignore` separa dependência capturada de saída gerada.**
   O `.gitignore` gerado **deixa de ignorar** `.claude/skills|agents|commands/`
   (agora commitados) e **continua ignorando** artefatos de runtime
   (`graphify-out/`, `.env`, `node_modules/`, `.dart_tool/`, etc.). Regra-mãe:
   **dependência capturada → commit; saída gerada → ignore.**

## 4. Modelo de compartilhamento

Compartilhar um ambiente = **compartilhar o resultado, não a receita**. O time
clona o repo e já recebe o `.claude/` pronto, offline, sem depender de nenhum
upstream. A receita continua **pessoal**, no `~/.ray` de cada um.

Isso satisfaz a premissa "compartilhável" por um caminho diferente do óbvio: não
se distribui um artefato de receita reproduzível; distribui-se o **estado final
materializado**, que é justamente o que um colaborador precisa para *usar* o
ambiente (não para regenerá-lo).

## 5. Topologia em disco

```
~/.ray/                               # global, por usuário (a ferramenta)
├── profiles/*.yaml                   # ÚNICA fonte de curadoria; editada à mão
└── store/<hash>/                     # cache content-addressed (§5.1)

~/.claude/                            # global, por usuário
└── skills/...                        # skills PESSOAIS (--global); não viajam

/caminho/meu-projeto/                 # o projeto do usuário (um repo git)
├── .git/
├── .gitignore                        # ignora runtime, NÃO ignora .claude/conteúdo
├── .claude/
│   ├── skills/<skill>/SKILL.md       # COMMITADO (vendorizado)
│   ├── agents/<agent>.md             # COMMITADO
│   ├── commands/<cmd>.md             # COMMITADO
│   ├── settings.json                 # COMMITADO
│   └── handoff.md                    # vivo, gerido pela IA
├── .mcp.json                         # COMMITADO (aponta p/ ferramentas latest)
├── docs/                             # docs do projeto, COMMITADO
└── src/...                           # código do projeto
```

O `ray` (a ferramenta, `github.com/murilopmr/ray`) **não embute conteúdo**: seu
binário segue leve e orquestrador. O conteúdo mora no `.claude/` do **projeto**,
não no repositório da ferramenta.

### 5.1 Cache local de conteúdo (`~/.ray/store`)

Um cache content-addressed por usuário, que **se preenche sozinho** conforme você
instala skills — papel análogo ao `~/go/pkg/mod` ou ao cache do `npm`. Mantém o
`ray` como **orquestrador puro**: ele só guarda cópias do que o upstream
entregou; nunca autora nem curadora conteúdo (Sabor A do brainstorming).

- **Como popula (agnóstico ao adquiridor):** cada componente é materializado
  por um `Acquirer` (§5.2). O `store` cacheia o **resultado** — o diretório
  materializado — sem saber se veio do git ou de um CLI: calcula o hash
  content-addressed (`sha256` sobre `(caminho-relativo, conteúdo)` ordenado) e
  guarda em `~/.ray/store/<hash>/`. **Não há snapshot-diff:** no `GitAcquirer` o
  `ray` buscou o conteúdo, então sabe exatamente o que é; no `CliAcquirer` a
  localização de depósito é determinística (`.claude/skills/<skill>/`,
  `.claude/agents/<name>.md`), então a captura é por pasta conhecida. Um índice
  leve mapeia `coordenada → hash`, com a coordenada **namespaced por origem**
  (`git:repo@ref#path`, `skills:source@skill`, `aitmpl:type:ref`) — sem colisão
  entre trilhas, dedup de conteúdo **global**.
- **Como serve (cache-first):** num `init ai`, antes de baixar, o `ray` checa se
  a coordenada já está no cache. Se estiver, **restaura do cache** (rápido,
  offline) em vez de tocar a rede; senão, baixa e popula.
- **Dedup:** a mesma skill usada em 10 projetos é guardada **uma vez**.
- **Frescor (política):** o "sempre latest" do conteúdo é mantido **sob demanda
  pelo `ray update`**, que re-puxa do upstream e atualiza cache + `.claude/`. O
  `init ai` prioriza velocidade/offline (cache-first); a atualização é explícita.
- **Nunca viaja, nunca é commitado.** É otimização local de performance e
  resiliência-na-origem (offline-first do que você já usou). O design da §1–§4
  funciona inteiro sem ele; o cache só o acelera e o torna offline-friendly.

### 5.2 Modelo de aquisição — `Acquirer` (git direto vs CLI)

O mecanismo de instalação é **propriedade da fonte, declarada na receita** — o
`ray` não infere. Um contrato pequeno, no mesmo espírito do `Runner`/`Looker`
(interface injetada):

```
type Acquirer interface {
    Acquire(ctx, comp) (dir, prov string, err error) // materializa o conteúdo
    Key(comp) string                                 // coordenada de cache
}
```

- **`GitAcquirer`** (preferido; `via: git`): a fonte *é* um repositório. Resolve
  `owner/repo` → tarball/clone num **`ref` fixado** → localiza o subdir do
  componente (`path`) → copia para `.claude/` e para o `store`. Determinismo
  real: o `ref` é o pin; nenhuma exposição a symlink/telemetria/flag de terceiro.
  É o caminho para **maximizar fontes oficiais** (`anthropics/skills`,
  `vercel-labs/agent-skills`, `flutter/skills`, …).
- **`CliAcquirer`** (`via: skills` / `aitmpl`): embrulha os installers do
  ecossistema quando eles agregam valor (resolver por *nome* de skill via
  registro, layouts fora do padrão). Captura por localização determinística.

O `store` (§5.1) é **agnóstico** aos dois: se um fetch git e um fetch CLI
produzem os mesmos bytes, deduplicam para **uma** entrada de conteúdo. Isso
generaliza o antigo par skills/aitmpl para "N fontes, cada uma com seu
adquiridor", e é o que mata a fragilidade de upstream no nervo (premissa §1):
onde dá, o `ray` busca o git você-mesmo, pinado; onde o CLI ajuda, ele orquestra.

**Ferramentas (MCP) não têm adquiridor.** MCP é *processo*, não conteúdo (§4):
fica na trilha "latest + config commitada no `.mcp.json`", nunca no `store`.

### 5.3 Procedência e licença (compliance do vendoring)

Vendorizar é **redistribuir** conteúdo de terceiros. Ao materializar um
componente, o `Acquirer` também captura a **LICENSE** do repositório de origem
(o `GitAcquirer` já tem o repo em mãos; o `CliAcquirer` conhece o repo pela
coordenada e busca a LICENSE uma vez, cacheando por repo) e grava, ao lado do
conteúdo, um `.ray-origin` mínimo (`repo@ref`) + o texto da licença. Isso:

- satisfaz a **preservação de aviso** exigida por licenças permissivas
  (MIT/Apache — justamente as das fontes oficiais);
- entrega **procedência commitada de graça** (`repo@ref` por componente);
- é o mesmo padrão do `vendor/` do Go, que mantém as licenças dos módulos.

Fonte **sem LICENSE** = copyright padrão, todos os direitos reservados: o `ray`
**vendoriza mesmo assim, mas avisa (soft)** — `via-origin: desconhecida`. Sem
bloqueio duro (a maioria das skills da comunidade é sem-licença por desleixo).
`.ray-origin` + LICENSE **são commitados** — não são bookkeeping do `ray`, são
material de compliance que *deve* viajar (por isso não ferem §3).

## 6. Fluxos

- **`ray init ai`** — lê o perfil global; para cada componente de conteúdo,
  serve do cache `~/.ray/store` se presente (cache-first, §5.1) ou baixa e
  popula o cache; deposita o conteúdo project-local em `.claude/`; instala as
  ferramentas no latest; mescla `.mcp.json`/`settings.json`; gera o scaffold e o
  `.gitignore` correto. O conteúdo instalado fica pronto para ser commitado.
- **Clonar o repo** — o colaborador já tem o `.claude/` completo; não precisa
  rodar `init ai` para *usar* o ambiente. (Rodaria apenas se quisesse
  reinstalar/atualizar as ferramentas a partir do perfil dele.)
- **`ray update`** — atualiza ferramentas (`uv tool upgrade`) e re-adquire
  conteúdo via o `Acquirer` de cada componente (respeitando o `ref`: pinado =
  no-op; `main` = re-resolve HEAD). É **protetor por conteúdo**, não por estado
  do git (§13): compara o hash do que está no disco com o **hash pristino** que
  o upstream entregou por último (metadado local em `~/.ray`). Se forem iguais,
  sobrescreve; se diferem, **você editou** — pula com aviso e exige `--force`.
  Isso protege até edições **commitadas** (o furo do modelo antigo). Exige árvore
  limpa (ou `--force`) para o diff ficar legível; em massa, a decisão é
  **por componente**.

## 7. Premissas testadas e *confirmadas* (não mudaram)

O brainstorming questionou várias premissas e, no fim, manteve a maioria — o que
é um resultado legítimo:

- **Orquestrador, não dono do conteúdo** — mantido. O `ray` não autora conteúdo;
  só muda *de quem é a cópia local* (passa a ser o repo do projeto).
- **Receita plana, sem herança/composição** — mantido. Sem lock, sem camadas.
- **Economia de tokens via headroom + graphify** — a *tese* é mantida, mas
  **estruturada** nesta volta (§8): de duas flags soltas para uma categoria
  "Token Economy" de mecanismos plugáveis, com métricas e UX. headroom+graphify
  passam a ser *uma* implementação, não a definição da tese.

## 8. Token Economy — a tese estruturada

O valor-âncora do `ray` ("ambiente econômico em tokens") era uma **afirmação sem
loop de feedback**: ninguém — nem o usuário, nem o `ray` — sabia se funcionava,
quanto economizava, ou se um mecanismo estava ocioso. Esta seção converte a
promessa em estrutura, medição e controle.

**Princípio de design (a restrição honesta).** O `ray` vive *fora* do loop da
sessão do Claude Code: monta o ambiente e sai de cena; o consumo de tokens
acontece *dentro* do Claude Code, onde o `ray` não está. Logo, o `ray` mede
**proxies de atividade** (fáceis, a partir de hooks e dos logs que os MCP servers
já escrevem) e **estima** tokens apenas quando a telemetria do Claude Code está
acessível — **nunca finge precisão que não tem**.

### 8.1 Três mecanismos, uma categoria

O que hoje são flags soltas (`headroom`, `code_graph`) e um comportamento
embutido (handoff) passam a ser **mecanismos de uma categoria "Token Economy"**,
cada um com a mesma forma:

| Mecanismo | Como economiza | Origem |
|---|---|---|
| **Grafo de código** | a IA consulta o grafo em vez de reabrir arquivos | graphify (MCP) |
| **Compressão de contexto** | comprime o contexto quando cresce | headroom (MCP) |
| **Handoff entre sessões** | injeta o estado da sessão anterior, sem reconstruir | built-in (hook) |

### 8.2 Abstração do "provedor de economia"

Um contrato pequeno e comum descreve cada mecanismo (conceitual):

```
Mechanism {
  Name      string            // "code-graph", "context-compression", "handoff"
  Kind      string            // mcp | hook | builtin
  Install   []runner.Command  // como provisionar (vazio p/ built-in)
  Server    *mcp.Server       // se expõe MCP (graphify-mcp, headroom mcp)
  MetricKey string            // chave-proxy lida pelo `ray stats` (§8.3)
}
```

graphify/headroom/handoff viram **três implementações** desse contrato. Benefício:
se um upstream mudar de rumo ou morrer, é **um mecanismo a substituir**, não a
tese inteira — ataca a fragilidade de upstream (premissa 1) no ponto mais
sensível. Mecanismos novos entram pela mesma porta (§8.4).

### 8.3 `ray stats` — tornar visível e provável (proxies leves)

Novo comando que **agrega métricas-proxy** que cada mecanismo deixa num local
conhecido (ex.: `.claude/.ray-metrics/`, ignorado no git como artefato de
runtime). Sem instrumentação invasiva: lê o que **já existe** — contagem de
handoffs injetados (hook), e os logs que os MCP servers já escrevem.

- **Honestidade na UI:** reporta **atividade medida**, sem afirmar causa que
  não mede — *"142 consultas ao grafo · 7 compressões de contexto · 12
  handoffs"* (e **não** "142 consultas evitaram reabrir arquivos", que seria uma
  causalidade não medida). Tokens só aparecem **se** a telemetria do Claude Code
  estiver acessível, e sempre rotulados como *estimativa*. Nunca inventa número
  nem relação causal.
- **Loop de feedback:** o usuário passa a *ver* o valor; o `ray` ganha base para
  afinar/justificar a curadoria.

### 8.4 Novos mecanismos (plugáveis pela mesma interface)

Candidatos honestos, adicionáveis sem tocar o resto:

- **Sumarização de docs** — um resumo curado que a IA lê em vez de reabrir docs
  longos.
- **Escopo de leitura por tarefa** — regra que enxuga o que a IA abre por
  contexto de tarefa.
- **Descartado:** cache de respostas — é responsabilidade do Claude Code, não do
  ambiente. YAGNI.

### 8.5 UX controlável

O invisível vira tocável: `ray status` (§10.4) ganha uma visão de economia
(quais mecanismos ativos, última atividade), e o handoff fica inspecionável/
editável (o usuário vê e ajusta o estado que será injetado na próxima sessão).

## 9. Modo learn — de proibição a prática

**Diagnóstico.** O `--mode learn` de hoje é definido por uma *negação*: um hook
bloqueia a IA de editar código + uma regra diz "ensine, não faça". É uma
**restrição**, não uma **pedagogia** — falta o lado positivo (o que de fato
ensina), progressão e memória do que já foi aprendido. (Sinal de subdesenvolvimento:
o `ray-roadmap.md` é um currículo artesanal de 1.400 linhas — o pensamento
pedagógico já existe, o produto só não o captura.)

**A virada.** Transformar o learn de uma *proibição* numa *prática*: um **prompt
de ensino** socrático (parametrizado por nível) + diário que progride + marcos
verificáveis + geração de tutorial sob demanda — **sem o `ray` virar autor de
conteúdo** (princípio §1 preservado, ver §9.5).

### 9.1 Prompt de ensino socrático, parametrizado por nível

O coração do modo. O `ray` scaffolda no `.claude/` um **prompt de ensino fixo e
personalizado** (uma regra/persona) que define o método: perguntar antes de
contar, escada de dicas (hint ladder), explicar o *porquê*, propor o próximo
passo em vez de entregá-lo. Isso é só mais um artefato que o `ray` já sabe
entregar via template embutido — ele orquestra o **prompt**, não o conteúdo.

**Nível do aluno (`--level`).** Flag no `init ai`/`new`, válida com `--mode
learn`: `iniciante | intermediário | avançado` (default `intermediário`). A flag
é um **input de geração**: escolhe qual **variante** do prompt é scaffoldada —
iniciante recebe muito andaime e explicação; avançado recebe só os *deltas* (o
que é novo), quase só revisão. O que viaja no repo é o **conteúdo de ensino
gerado** (que é só conteúdo, como qualquer skill vendorizada), **não** um
marcador "meu nível" — trocar = re-rodar `init ai --mode learn --level X
--force`. (Assim, compartilhar o repo não expõe seu progresso pessoal; ver §9.2.)

**A escada de dicas (hint ladder).** O "quanto entregar" é um **protocolo
explícito** no prompt de ensino, do menos ao mais revelador:

1. Pergunta reflexiva ("o que acontece se…?")
2. Ponteiro conceitual ("isso é sobre ownership de memória")
3. Dica de localização ("olha a função X / o arquivo Y")
4. Estratégia ("vai precisar de um ponteiro aqui; pensa por quê")
5. Solução parcial (esqueleto com lacunas)
6. Resposta completa + explicação (último recurso)

**Controle: o aluno puxa; a IA escala só se travar.** O default é o degrau mínimo
do nível; o aluno pede *"mais/dica"* para subir um degrau por vez. A IA só sobe
sozinha após algumas tentativas travadas. A **resposta completa (degrau 6) está
sempre disponível, mas é gateada por confirmação** ("tem certeza? tenta mais
uma?") — evita o atalho preguiçoso sem nunca deixar o aluno preso. O `--level`
define **onde a escada começa** e **com que rapidez sobe**: iniciante começa no
degrau 2–3 e sobe rápido; avançado começa no 1 e sobe a contragosto.

### 9.2 Diário de aprendizado (progressão)

Generaliza o **handoff** num **diário de aprendizado**. Reaproveita máquina
existente — o `session-start` já injeta continuidade — mas com **disciplina de
curadoria** embutida, senão vira parede de texto que custa contexto toda sessão
(irônico num produto cuja tese é economia de tokens).

**O diário é pessoal → local, não vendorizado.** Coerente com "skill pessoal =
global" (§3): progresso e log de aprendizado **não viajam** no repo. Vivem num
local ignorado (`.claude/.local/` no `.gitignore`, ou `~/.ray/learn/<projeto>`).
Só o **conteúdo de ensino** gerado é commitado; o **progresso** é seu e fica na
sua máquina.

**O que captura — *deltas de entendimento*, não atividade:** conceitos que você
sacou (o "aha" em uma linha), o que ainda está nebuloso, equívocos corrigidos (da
checagem de §9.3), e o próximo passo. **Não** captura: comandos rodados, arquivos
tocados, o blow-by-blow — isso é transcript, não aprendizado.

**Como estrutura — head vivo + log append-only (anti-inchaço):**
- um **head pequeno e fixo** ("Onde você está: dominado · em aberto · próximo
  passo"), **sempre reescrito**, é o **único** trecho que o `session-start`
  injeta — custo de contexto **limitado**, seja na 3ª ou na 30ª sessão;
- abaixo, um **log append-only** que registra a jornada para você reler quando
  quiser, mas que **não entra no contexto**. Memória longa sem custo recorrente.

Mesma filosofia do handoff (estado vivo injetado, histórico não), aplicada ao
aprendizado.

### 9.3 Marcos verificáveis (learn-by-building, fiel ao §1)

Um campo **opcional** na receita declara marcos de um projeto-escola — cada marco
é um **comando verificável**, não prosa:

```yaml
milestones:
  - goal: "Esqueleto compila"
    verify: "go build ./..."
  - goal: "Runner com testes verdes"
    verify: "go test ./internal/runner/..."
```

O `ray` fornece só a **máquina**: roda o `verify` (fronteira `runner`), rastreia
em que marco você está e registra a passagem no diário (§9.2). Marcos ≈ aliases
do `ray run`; o `ray` **não autora** o currículo — a curadoria vive na receita do
usuário (§1). É um overlay sobre o `ray new`. (Quando há um tutorial gerado em
§9.4, ele pode referenciar esses marcos como seus checkpoints verificáveis.)

**Gate duro mecânico + checagem suave de compreensão.** O marco só fica "verde"
quando o `verify` passa — objetivo, no domínio do `ray`. Mas *passar ≠ aprender*
(dá pra colar a resposta do degrau 6). Por isso, ao cruzar um marco, o prompt de
ensino faz a IA pedir para você **explicar o porquê**; sua resposta e as lacunas
que aparecerem vão para o diário (§9.2) — **sem bloquear** o progresso. O `ray`
**nunca finge medir entendimento** (não é verificável por comando); ele torna o
entendimento um ritual e deixa um *rastro* que um humano revisa. O `--level`
modula: iniciante sempre recebe a checagem; avançado quase nunca.

### 9.4 Geração de tutorial completo sob demanda

Se o aluno pedir, um **comando** (ex.: `/tutorial`, análogo ao `/document` já
existente) dispara a geração de um **documento-tutorial 0→100% do projeto** —
exatamente o que o `ray-roadmap.md` é, mas para qualquer projeto. O comando
carrega um **prompt específico** (também scaffoldado, calibrado pelo nível de
§9.1) que orienta a IA a produzir o material: fundamentos → fases → "pronto
quando…". O documento gerado mora em `docs/` (versionado com o projeto).

**Quem gera é a IA, dentro da sessão — não o `ray`.** O `ray` só entrega o
prompt; a prosa nasce no runtime. Isso é o que mantém §1 intacto (ver §9.5).

### 9.5 O que continua descartado (e por quê a geração NÃO é o "Sabor B")

A linha que **não** se cruza: o `ray` **autorar/curar** um registry de trilhas ou
embutir prosa de currículo no binário — isso romperia §1 e exigiria o `ray`
invocar um LLM (coisa que ele nunca faz). A geração de tutorial (§9.4) é
**diferente**: o `ray` orquestra *prompts* (seu ofício de sempre) e a **IA**
gera o conteúdo *dentro da sessão*, sob demanda. O `ray` segue orquestrador, não
dono de conteúdo. O `ray-roadmap.md` é *evidência* desse tipo de artefato — o que
o §9.4 faz é permitir que a IA produza um equivalente para qualquer projeto.

## 10. UX e primeira experiência

Melhorias incorporadas do brainstorming de UX (a jornada do "instalei o `ray`" ao
"ambiente de IA funcionando", com o mínimo de tropeços):

1. **First-run caloroso.** `ray` sem subcomando mostra uma tela de "começando" —
   **não** o help cru do Cobra: uma linha de status de dependências (`doctor`
   silencioso), o próximo comando sugerido (`ray new go meu-app`) e um alerta se
   faltar dependência required. `ray --help` continua listando todos os comandos.

2. **Fix de dependências inline no `init ai`.** Quando o preflight acha deps
   required faltando e há TTY interativo, o `ray` oferece *"Instalar agora?
   [S/n]"* e roda os `Check.Fix` (mesma fonte do `doctor --fix`), depois continua
   o fluxo. Sem TTY ou com `--yes`, comporta-se de forma não-interativa (aborta
   com a dica, ou instala direto). Reusa o `preflight` como fonte única — só
   adiciona a *oferta* interativa na fronteira do `init ai`. Colapsa três comandos
   (`doctor --fix` → re-`init ai`) num fluxo só.

3. **Rodapé "Próximos passos" no resumo.** O `Summary` ganha um rodapé ciente do
   contexto. Num repo git, instrui a versionar o vendoring
   (`git add .claude && git commit`) e a iniciar a sessão (`claude`). Fecha o
   loop do vendoring — a ferramenta ensina a commitar o `.claude/`. (Instrução de
   git só aparece dentro de um repo.)

4. **`ray status` (painel do ambiente).** Novo comando: num projeto, lê o
   `.claude/` e o `.mcp.json` e mostra skills/agents/commands presentes, MCP
   servers conectados, versões das ferramentas (headroom/graphify) e o **estado
   git do `.claude/`** (limpo / sujo / untracked = *drift* do vendoring).
   Observabilidade do "seu ambiente de IA".

> **Não incorporado:** seletor interativo de perfil (omitir perfil → picker) —
> considerado, fora desta volta.

## 11. Impacto no `ray-build-guide.md`

Seções a ajustar na reconstrução:

- **§7 (Scaffold)** — `.claude/skills|agents|commands/` passam a ser tratados
  como conteúdo **commitável**, não efêmero.
- **§15.1 (bug do `.gitignore`)** — o `.gitignore` gerado precisa **whitelistar**
  o conteúdo de IA e **blacklistar** só runtime/segredos. Detalhar a regra-mãe.
- **§5 / §6 (flags e mapeamento)** — documentar que componentes de conteúdo são
  project-local por padrão; `--global` é só para conteúdo pessoal cross-project.
- **§16 (`ray update`)** — incluir o re-puxar de conteúdo (cache + `.claude/`)
  com revisão via git diff, além do upgrade de ferramentas.
- **§9 (layout de `~/.ray`)** — adicionar `store/` (cache content-addressed,
  §5.1); provável novo pacote `internal/store` (captura, hash, lookup por
  coordenada), tocando disco só via a fronteira já existente.
- **§5 (árvore de comandos) + §16** — novos comandos `ray status` (painel do
  ambiente, §10.4) e `ray stats` (métricas de economia, §8.3); root passa a ter o
  first-run caloroso (§10.1); `init ai` ganha a oferta interativa de fix de deps
  (§10.2) e o rodapé "Próximos passos" (§10.3).
- **§4.1 / §6 (modelo + integrações)** — `Integrations.Headroom` e
  `Integrations.CodeGraph` passam a ser modeladas como **mecanismos de uma
  categoria Token Economy** com um contrato comum (§8.2); o handoff entra como
  terceiro mecanismo (kind `builtin`/`hook`). Provável pacote
  `internal/economy` (lista de mecanismos + leitura das métricas-proxy).
- **§7 (modos `build|learn`) + §5 (flags)** — enriquecer o overlay learn (§9):
  prompt de ensino socrático scaffoldado, **parametrizado por nível** via nova
  flag `--level iniciante|intermediário|avançado` (default `intermediário`,
  assada no `.claude/`); diário de aprendizado no vault (generaliza o handoff);
  marcos verificáveis opcionais na receita (`milestones: [{goal, verify}]`, ≈
  aliases do `run`, rodados via `runner`, com **gate duro mecânico + checagem
  suave de compreensão** registrada no diário); comando `/tutorial` (análogo ao
  `/document`) que dispara a geração in-session de um tutorial 0→100% em `docs/`,
  guiada por prompt scaffoldado. O prompt de ensino codifica a **escada de dicas**
  (6 degraus; aluno puxa, IA escala se travar, degrau 6 gateado por confirmação);
  o **diário** tem head vivo injetado no `session-start` + log append-only fora do
  contexto.
- **§13 (testes)** — cobrir: first-run do root, oferta interativa do `init ai`
  (com TTY simulado/`--yes`), rodapé condicional a repo git, `ray status` lendo
  `.claude/`+`.mcp.json` e o estado git, `ray stats` agregando métricas-proxy de
  um diretório-fixture, e marcos do learn (o `verify` roda e a passagem é
  registrada) — tudo com `FakeRunner`.

## 12. Fora de escopo (YAGNI desta volta, com razão)

- **`ray.lock` / fixação de versões** — substituído pelo conteúdo commitado, que
  *é* o pin. Ferramentas ficam no latest por decisão explícita.
- **Biblioteca curada/embutida no `ray` (Sabor B)** — descartada por fazer o
  `ray` virar dono de conteúdo, contrariando §1 e a premissa confirmada. O cache
  da §5.1 (Sabor A) entrega o benefício de offline/dedup sem cruzar essa linha.
- **Bundles standalone / registry de receitas** — descartados por violarem
  simplicidade/portabilidade.
- **Herança/composição de receitas** — despriorizada explicitamente.
- **Seletor interativo de perfil** — considerado no brainstorming de UX, fora
  desta volta (§10).
- **`ray` autorar/curar currículo (registry de trilhas ou prosa embutida)** —
  descartado por romper §1 e exigir o `ray` invocar um LLM (§9.5). Note a
  distinção: a geração de tutorial in-session (§9.4) **não** é isto — lá o `ray`
  só entrega o prompt e a IA gera o conteúdo na sessão.
- **Instrumentação ativa dos MCP servers (métricas precisas)** — descartada nesta
  volta (§8.3): acopla o `ray` ao interior dos mecanismos e é frágil/invasivo. As
  métricas ficam em proxies leves (hooks/logs existentes).

## 13. Decisões resolvidas

As três questões em aberto foram fechadas (2026-06-28). Convergem num princípio
único: **rede e sobrescrita são explícitas (via `ray update`); o caminho cotidiano
(`init ai`) é rápido, offline e seguro-por-git.**

- **Política de update (`.claude/` vendorizado).** `ray update` **sobrescreve por
  padrão** — o `.claude/` é commitado, então toda mudança aparece no `git diff` e
  é reversível. **Salvaguarda de drift:** se houver mudanças *não-commitadas* no
  `.claude/`, o `update` avisa e exige confirmação/`--force` antes de engolir
  edição manual. Conversa com o `ray status` (§10.4), que detecta o drift.
- **`ray stats` — só proxies (§8.3).** Nesta volta, apenas métricas-proxy de
  atividade (consultas ao grafo, compressões, handoffs). A estimativa de tokens
  via telemetria do Claude Code fica como **gancho futuro** atrás de um flag
  experimental (`--estimate-tokens`), só se/quando houver formato estável —
  depender de formato não-documentado contradiz a restrição honesta do §8.
- **Frescor do cache — cache-first puro (§5.1).** `init ai` serve do cache sem
  tocar a rede; trazer o latest é **explícito** via `ray update`. Modelo mental
  do `npm`/`go mod`. Sem `git ls-remote` no caminho comum.

# Features — ray

O que cada feature **é**, o que tem de valer sempre para ela continuar correta, e
o que foi recusado de propósito ao desenhá-la. Descreve estado: se algo aqui
ficar defasado do código, virou ficção — corrija junto com a mudança que o
defasou.

Fronteira com os outros documentos, para nada ser dito duas vezes:

| Documento | Responde |
|---|---|
| `architecture.md` | como o sistema é montado — pacotes, fronteiras, regras de dependência |
| `conventions.md` | como se escreve código aqui |
| **este arquivo** | o que cada feature faz e o que precisa continuar verdade |
| `README.md` | como usar |

Duas naturezas, separadas de propósito: a Parte 1 é o que o `ray` faz quando
você o roda; a Parte 2 é o conteúdo que ele **escreve nos projetos dos outros**.
Confundir os dois é o erro mais caro deste repositório.

---

# Parte 1 — O que o `ray` faz

## Tela de abertura

`ray` sem subcomando não imprime o help do Cobra: imprime uma tela que **orienta**
e ramifica por contexto. O sinal de "isto é um projeto ray" é o `.claude/`
existir — o mesmo sinal que o `ray status` usa.

Fora de um projeto, a tela oferece criar um. Dentro, oferece o que fazer com o
que já existe: nome do perfil, inventário, e os dois próximos passos plausíveis
(`claude`, `ray status`).

**O que tem de valer:**

- **Só fatos baratos.** Perfil e inventário saem de leitura de arquivo. Sem git,
  sem MCP, sem carregar receita — a tela orienta, não diagnostica.
- **Nenhum processo é criado.** Sem receita carregada nada é obrigatório, então
  a tela não tem dependência a avisar e não paga `--version` por ferramenta. A
  dependência é assunto do `ray doctor`.
- **Não existe linha `deps: ok`.** Uma linha de confirmação fixa na tela mais
  vista do CLI gasta a mesma atenção que um alerta precisaria para significar
  alguma coisa.
- Falha de leitura do `.claude/` (permissão) é erro de verdade: exit 1.

A contagem de inventário é **compartilhada** com o `ray status`, e isso é
deliberado: duas contagens divergiriam. O que os dois não compartilham é a
origem do nome do perfil — a tela lê o registro em `.claude/.ray-profile`, o
`status` usa o nome da receita carregada. São valores diferentes com o mesmo
rótulo, e unificá-los faria a tela pagar o carregamento de receita que a decisão
acima acabou de evitar.

## `ray status`

Diagnostica o ambiente vendorizado **deste projeto**. A fronteira com o
`ray doctor` é limpa e não se cruza: o `doctor` pergunta se a *máquina* está
pronta (dependências externas, com `--fix`); o `status` pergunta se *este
projeto* está são. O `status` não repete a tabela de dependências.

O princípio é que **status é diagnóstico, não inventário**: mostra o que está
errado ou fora do lugar, e cala sobre o que está bem. Ambiente são imprime duas
linhas. Três níveis de saída, e a distinção entre eles é o coração da feature:

| Nível | O que é | Quando aparece |
|---|---|---|
| **Fato** | inventário; nunca é alerta | sempre |
| **Nota** | estado esperado que o usuário precisa saber | ex.: ambiente ainda não versionado |
| **Problema** (`⚠`) | algo está errado agora | ex.: falta uma negação no `.gitignore` |

Ambiente são imprime **só fatos**. É isso que faz o `⚠` significar alguma coisa
quando aparece — alerta que aparece quando não deve treina o usuário a ignorá-lo.

### As quatro checagens

**Fork por conteúdo** é o núcleo, porque é a única que responde algo que o
usuário não consegue ver de outro jeito: compara o hash da árvore em disco
contra o hash pristino guardado no `store`.

| Comparação | Estado | O que o comando diz |
|---|---|---|
| hashes iguais | intocado | `ray update` atualiza |
| hashes diferem | editado localmente | `ray update` preserva |
| sem pristino | procedência desconhecida | não afirma nada |

O terceiro estado não é conservadorismo decorativo. A decisão de sobrescrita só
é respondível offline **quando há linha-base**: com pristino presente, a resposta
é `disco == pristino` e nada mais. Sem pristino, decidir exigiria comparar com o
componente em `~/.ray/components/`, que o `status` não lê — e, mesmo lendo,
`disco != componente` não separa edição local de componente que mudou na pasta.
Então o comando não decide, e diz que não sabe.

**Git** — duas chamadas, zero heurística, sobre `.claude/` e `.mcp.json`. Não
inclui `docs/` nem `CLAUDE.md`: são do usuário, e tratar edição deles como desvio
de ambiente seria falso positivo. Nunca versionado é **nota** com o `git add` que
resolve; versionado e divergente é **problema**. A distinção entre os dois sai de
listar os arquivos rastreados, não de adivinhar pelos códigos de status —
logo depois do `ray init ai` tudo está fora do versionamento, e esse é o estado
normal, porque o próprio `ray` acabou de pedir o commit.

**`.gitignore`** — a falha silenciosa que motivou a feature. O bloco vive entre
marcadores; se alguém apagar as negações, o conteúdo vendorizado volta a ser
ignorado e nada avisa. A checagem exige os marcadores e todas as linhas da base
dentro deles, nomeando qual falta.

**MCP** — só o que o `ray` sabe: o caminho de cérebro configurado, que é dele, e
a existência do `Command` de cada servidor no `PATH`, que é verificável sem
adivinhação. "Apontar para caminho morto" **não** é genericamente detectável: um
servidor MCP tem comando, argumentos e ambiente, e nada ali é tipado como
caminho — varrer argumentos procurando o que "parece caminho" seria chute, ainda
mais num arquivo onde o usuário também põe servidores próprios.

**O escopo de vigilância é derivado das negações do bloco do `.gitignore`** —
hoje `.claude/` e `.mcp.json` — menos um denylist onde mora o `docs/`, que é do
usuário. Derivado, e não fixo, porque lista fixa dessincroniza em silêncio: a
whitelist ganha entrada e o `status` para de vigiar parte do ambiente sem nada
falhar. **O `git add` do rodapé é outra pergunta** e usa três fontes unidas,
filtradas por existir em disco: a whitelist inteira, os arquivos que a receita
escreve na raiz, e o `.gitignore` — que entra sempre, porque o `ray` sempre o
escreve, mesmo com receita ilegível. Commitar e vigiar são perguntas
diferentes; antes dessa separação o `ray status` mandava commitar **menos** que
o `ray new` para o mesmo projeto.

**A linha de fatos conta conteúdo, não entradas de topo**: uma skill é um
`SKILL.md`, um agente e um comando são um `.md`, e a contagem desce em
subdiretório. Contar entradas de diretório fazia um README solto em `skills/`
valer uma skill, e um grupo de comandos com namespace valer um comando só.

Sem `.claude/.ray-profile`, a checagem de fork inteira cala — não há receita a
comparar, e `.claude/` copiado à mão é caso normal. Com o registro presente e a
receita ilegível é o oposto: vira problema, com o erro junto. Os dois casos
saíam iguais, e o segundo deixava o usuário sem `profile:` na saída e sem pista
do porquê.

**O que tem de valer:**

- **Exit code é sempre 0**, exceto falha real de leitura. Problema detectado não
  é exit não-zero: aqui "3 arquivos não commitados" é informação, e transformá-la
  em erro tornaria o comando inútil em qualquer script que o encadeie.
- **Nunca vai à rede.** Funciona offline, como o `init ai` cache-first.
- **Cada checagem sabe ficar quieta quando não pode rodar.** Ausência de
  pré-requisito é omissão, não erro: sem repositório git, a seção git some.
- **Sem `.claude/`, o comando diz uma frase e para** — não cospe quatro checagens
  vazias.

**Recusado de propósito:** reusar o `update` em modo de simulação para
diagnosticar. Seria zero lógica nova, mas ele lê `~/.ray/components/` para
comparar — e um status que depende de uma pasta que num clone novo pode nem
existir, para dizer se o `.claude/` está são, não é um status.

## Dependência ausente

A renderização do que dizer sobre uma dependência que falta mora no mesmo lugar
que decide se ela existe. Fonte única da checagem é fonte única da mensagem, e os
**dois** consumidores usam: o gate do `init ai` e a tabela do `ray doctor`.

**O que tem de valer:**

- **Não é interativo.** Um prompt "instalar agora?" só teria onde agir num caso
  estreito: o que é obrigatório sem condição não tem instalação automática (o
  `python3.10+`, quando uma integração o liga), então o prompt dispararia
  exatamente onde não há o que executar.
- **O `npx` é opcional, e `ray new` confere o que o `create:` precisa.** Só o
  `create:` de certas receitas (a `web`) usa o `npx`; tratá-lo como obrigatório
  barrava `init ai` e `doctor` numa máquina sem Node por um programa que nem
  entrava no caso. Em troca, `ray new` confere, **antes de criar a pasta**, que o
  programa de cada passo do `create:` existe, e nomeia a receita e o passo quando
  não existe. "Existe" é "o processo inicia", não "o `--version` passa": o `go`
  sai com código 2 no `--version` e está instalado.
- **O comando de instalação não aparece cru na saída.** Não se normaliza
  `curl | sh` impresso na tela. Quem quer auditar roda o `doctor` com `--fix` em
  modo de simulação, que imprime o comando sem executá-lo.
- A coluna de dica aparece **só** para o que falta.

## Rodapé de próximos passos

Ao fim do `ray init ai`, o resumo termina com o `git add` exato do que aquela
execução escreveu e que precisa ser versionado, seguido do commit e do `claude`.

**O que tem de valer:**

- **Nunca `git add -A`, `--all` ou `.`** — os caminhos são nomeados um a um. Um
  dos hooks que o `ray` instala avisa contra `add` cego; o rodapé não pode
  contradizer o hook.
- **Caminho que não existe em disco não entra**, senão o `git add` falha.
- **Fora de um repositório git, as duas linhas de git somem** e sobra o `claude`.
- **Se a execução falhou pela metade, o rodapé inteiro some.** Mandar commitar um
  ambiente quebrado é pior que não dizer nada, porque grava o estado quebrado.

O `.gitignore` entra na lista, e é o caso que mais importa: ele não está na
whitelist do próprio bloco porque *é* o arquivo que a contém — sem ele
commitado, as negações não existem no clone e o `.claude/` inteiro volta a ser
ignorado na máquina de quem clonar.

## `ray update` e a proteção de edição local

O `update` recopia o conteúdo declarado na receita a partir de
`~/.ray/components/` — nunca da rede — e **protege edição local por hash de
conteúdo**, não por estado do git. É a decisão que sustenta a promessa do
vendoring: você pode editar uma skill vendorizada, commitar, e o `update`
seguinte preserva sua edição — porque compara contra a linha-base pristina
guardada em `.claude/.ray-pristine.yaml` (no projeto, versionada), e o commit não
muda o conteúdo do arquivo. Por morar no projeto, ela viaja: mover a pasta ou
clonar o repositório não faz os componentes virarem "procedência desconhecida". Rodar `init ai`
de novo no mesmo alvo segue a mesma política: componente editado é preservado
(entra em `Skipped`, com aviso) e a linha-base não se move; só `--force`
sobrescreve.

**O que tem de valer:**

- **Edição local nunca é sobrescrita em silêncio.** Componente divergente entra
  no resumo como preservado, com o motivo.
- **Linha-base ilegível para tudo, antes de qualquer efeito.** Com o
  `.ray-pristine.yaml` do projeto (ou o `pristine.yaml` da máquina, enquanto o
  projeto ainda depende dele) corrompido, o `update` e o `init ai` recusam, com o
  caminho e a saída (apagar o arquivo; os componentes passam a "procedência
  desconhecida"),
  em vez de instalar, copiar e só falhar ao gravar. O `status` lista o arquivo
  como problema. Ausente é normal; só ilegível é erro. O arquivo é gravado de
  forma atômica, para um leitor concorrente nunca ver o meio.
- **O modo de simulação decide exatamente como a execução real decidiria**,
  com ou sem linha-base gravada. Sem rede envolvida, ler o "upstream" (a pasta
  local) é grátis mesmo em dry-run — não há mais um caso que só a execução real
  conseguiria resolver.
- **A guarda de árvore limpa não vale para a simulação** — simular não suja nada,
  e barrar a simulação empurra a pessoa para forçar a execução, que é o oposto do
  que a guarda quer.

## Mensagens de ausência

Comando que não tem nada a fazer **diz isso**, em vez de sair mudo com código 0.
Quatro comandos já convergiram na mesma forma sem regra escrita: frase curta,
minúscula, que nomeia a ausência e às vezes o comando que resolve.

Não há helper compartilhado, e isso é decisão: quatro contextos pedem quatro
frases, e centralizar trocaria clareza por indireção.

**Onde a precisão da frase importa:** o resumo do `update` cobre **componentes**;
passo global bem-sucedido não entra em nenhuma das listas. Por isso a frase diz
que não há componentes a atualizar, e não que não há nada a fazer — a segunda
contradiria a saída dos passos globais impressa logo acima.

**Um caso é de classe diferente e mais grave:** componente sem pasta
correspondente em `~/.ray/components/` era o tipo de coisa fácil de descartar
em silêncio. Não é: entra no resumo (`Failed` no `init ai`, `Skipped` no
`update`) nomeando o caminho que faltou — receita com componente inexistente é
situação a relatar, não erro a engolir.

## Perfis de fábrica

Na primeira leitura de `~/.ray/profiles/` o ray escreve quatro receitas: `go`,
`web` e `flutter` (cada uma com o `create:` do seu stack) e `base`. O `base` é o
ambiente de IA sem stack — sem `create:` e sem linhas extras de `.gitignore` — e
é o perfil que `ray init ai` usa quando `--profile` não é passado. Todas ligam
`headroom` e `code_graph`. `ray new base <nome>` também funciona: cria a pasta e
o `git init`, sem passo de criação de projeto.

A chave `scaffold.gitignore_stack` da receita lista linhas que o `init ai`
acrescenta ao bloco do `.gitignore`, depois das linhas fixas (ex. `node_modules/`
no `web`, `/{{.ProjectName}}` no `go`). Aceita `text/template` com os mesmos
dados do scaffold. Sem a chave, o bloco só tem as linhas fixas.

## Integrações

Um servidor MCP (`headroom`, `code_graph`) se declara em `integrations`, nunca
em `components`. São conceitos disjuntos por construção: `Component` só tem
`name` e `dest` — não há campo para dizer "isto é um servidor", então não
existe forma de confundir os dois no formato da receita.

`installer.Resolve` traduz as integrações ligadas num `Plan` com três listas:
`Commands` (por-projeto, sempre rodam), `Globals` (install-once, rastreados por
`Key` em `state.yaml`), `Servers` (entradas de `.mcp.json`).

| Integração | Tipo | Comando / Server gerado |
|---|---|---|
| `headroom` | Global + Server | install: `uv tool install headroom-ai[mcp]` · server `headroom` → `headroom mcp` |
| `code_graph` | Global + Command + Server | global: `uv tool install graphifyy` **e** `graphify install --platform claude` · por-projeto: `graphify update .` (constrói o grafo, tree-sitter, sem LLM) · server `graphify` → `graphify-mcp` (stdio, lê `graphify-out/graph.json`) |

**O que tem de valer:**

- O pacote PyPI do grafo é `graphifyy` (dois "y"); o binário é `graphify`.
- `headroom-ai[mcp]` precisa do extra `[mcp]` pra expor `headroom mcp`.
- Um global só vira `state.AddGlobal(key)` se **todos** os comandos do step
  saírem 0; `--no-global` pula todos; `--reinstall-global` ignora o state.
- Servers são **sempre** registrados por-projeto, mesmo que o global já tenha
  sido instalado antes — um `.mcp.json` novo não pode ficar sem a entrada só
  porque outro projeto já passou pelo global.

---

# Parte 2 — O que o `ray` escreve nos projetos dos outros

O conteúdo desta parte é gerado a partir dos templates em
`internal/scaffold/templates/`. Não é o ambiente **deste** repositório: é o que
o `ray` instala no repositório de quem o usa.

A árvore que `ray init ai` produz na pasta-alvo:

```text
<alvo>/
├── CLAUDE.md                 # a base estável: 12 seções XML (ver abaixo)
├── SECURITY.md                # [MUST]/[SHOULD], regras p/ código gerado por IA
├── .gitignore                 # bloco do ray entre marcadores (negações + gitignore_stack)
├── .mcp.json                  # headroom + graphify, se ligados na receita
├── docs/                      # o ESTADO ATUAL do projeto (versionado)
│   ├── README.md              # os dois papéis + o laço spec-driven
│   └── architecture.md  conventions.md
└── .claude/
    ├── .ray-profile           # perfil usado; é o que o `ray update` lê
    ├── .ray-pristine.yaml     # hash do que o ray escreveu em cada componente (versionado)
    ├── settings.json          # model, effortLevel, hooks
    ├── handoff.md              # estado vivo (gerido pela IA; NUNCA tocado por --force)
    ├── commands/{destilar,document,handoff,revisar}.md
    ├── hooks/{session-start,guard-add,guard-vocab,guard-plans,guard-handoff}.sh
    └── agents/  skills/        # populados pela cópia local de componentes
```

## `CLAUDE.md` — a base estável

Todo perfil default escreve, entre outros, um `CLAUDE.md` em 12 seções XML,
**nesta ordem**, que é load-bearing e travada por teste:

```
<role> <context> <stack> <documentation_sources> <architecture> <test_strategy>
<conventions> <quality_gates> <workflow> <agent_behavior> <output_format> <edge_cases>
```

`<workflow>` fica perto do fim de propósito — instrução de processo no topo é
atropelada pelo volume da spec colada no turno. `<edge_cases>` fecha o
documento: é a cláusula que impede invenção quando a premissa falha. A regra
que sustenta o conjunto: **o que está no `CLAUDE.md` nunca se repete no
documento por feature.**

## Arquivos de sistema e os hooks em `settings.json`

`scaffold.SystemFiles()` garante que `.claude/hooks/session-start.sh` e os
quatro guards abaixo existam no disco de todo projeto, **fora da receita** —
isso garante que todo hook referenciado em `settings.json` tenha o script que
ele nomeia. No `init ai`, os arquivos da receita e os de sistema são
deduplicados por path (receita ganha).

`scaffold.HookSettings()` devolve o bloco `hooks` que o ray instala em
`settings.json`: `SessionStart` (sempre, injeta o handoff) + `PreToolUse` (os
três guards de aviso) + `PostToolUse` (`guard-handoff`). Cada comando é
`bash "$CLAUDE_PROJECT_DIR"/.claude/hooks/<nome>.sh`: um caminho relativo
dependeria do diretório em que a sessão do Claude abriu, e numa subpasta o
script não seria achado — o aviso sumiria em silêncio.

**Como o `init ai` aplica isso a um `settings.json` que já existe.** O que é seu
vence, e `--force` é a única forma de o ray impor o dele:

- `model`, `effortLevel` e qualquer outra chave de topo que o arquivo já tem
  ficam como estão; só as que faltam são preenchidas.
- `hooks` é unido por evento. Os seus ficam. Os do ray — todo comando que começa
  com `bash "$CLAUDE_PROJECT_DIR"/.claude/hooks/` — são retirados e reinseridos
  a cada execução, de modo que um guard renomeado ou removido numa versão nova
  do `ray` não fica órfão apontando para um script que sumiu. O mesmo vale para
  o formato antigo, `bash .claude/hooks/` (caminho relativo): um projeto montado
  antes da troca migra no próximo `ray init ai`, sem ficar com os dois. Os hooks que a receita declara em
  `scaffold.settings.hooks` convivem com os do ray.
- Rodar de novo não muda o arquivo, byte a byte.
- Com `--force`, o bloco `hooks` e as chaves de topo são substituídos; chaves que
  o ray não gerencia (ex.: `env`) continuam preservadas.

Um hook seu cujo comando comece com um desses dois prefixos é tratado como do
ray e será reescrito: esse diretório é o que o ray gerencia.

## Os quatro hooks de aviso

`guard-add` (contra `git add` cego), `guard-plans` (contra escrever plano dentro
do repo), `guard-vocab` (contra vocabulário de processo em artefato entregue) e
`guard-handoff` (contra `.claude/handoff.md` passar do dobro do orçamento).

**Decisão central: avisam, não bloqueiam.** Hook que trava trabalho legítimo vira
hook desligado. Com aviso, o custo de um falso positivo é uma linha ignorada; com
bloqueio, é uma sessão travada — e na segunda vez, alguém remove o hook. A força
que se perde é real e está aceita: **a IA pode ignorar o aviso.**

**Três dos quatro rodam antes da escrita** (`guard-add`, `guard-plans`,
`guard-vocab`) — aviso que chega depois não redireciona nada, e os três leem só o
que a chamada carrega. `guard-handoff` é a exceção: precisa do tamanho final do
arquivo no disco, que Edit e MultiEdit não carregam no payload (só o trecho
alterado) — ver a seção dedicada abaixo.

**Nenhum concede permissão.** Um hook de aviso não tem o direito de decidir
permissão em nome do usuário; ele injeta a mensagem e deixa o fluxo normal
seguir.

### O escopo do `guard-vocab`

Ele varre **o texto que a chamada carrega** — o conteúdo a escrever, a string
nova de uma edição, cada string nova de uma edição múltipla — e nunca o arquivo
no disco. Vazio, sai calado.

Isso o alinha aos outros dois, que sempre leram só a chamada; ele era o membro
que destoava. E derruba a exigência de o arquivo existir, que era o que o
prendia a rodar depois da escrita.

**Consequência aceita:** vocabulário que já está num arquivo fica invisível ao
hook. É escolha, não lacuna — ele é freio de reflexo sobre o que está sendo
escrito agora, não lint de repositório. Se um dia doer, a resposta é uma
varredura separada, não devolver o hook ao disco.

**Recusado de propósito:** escopar pelo diff do git. Depende de um repositório,
que o projeto scaffoldado pode não ser; e falha em silêncio — se a mudança já foi
commitada entre uma edição e outra, o diff vem vazio e o hook cala, que é um
falso negativo pior que o ruído que se queria remover.

### Por que `guard-handoff` roda depois, e por que o orçamento é o dobro

Nasceu de um caso real: o próprio `.claude/handoff.md` do `ray` chegou a 355
linhas — quase 9x o alvo de ~40 que `/handoff` declara — sem nenhum aviso, porque
nada verificava. `/handoff` manda "fique enxuto"; o verbo é confiança, e
confiança sem checagem é exatamente o que essa família de hooks existe para não
depender.

Ele é `PostToolUse`, ao contrário dos outros três: `guard-vocab` e `guard-plans`
leem `content`/`new_string` do payload, que é o texto que a chamada está
escrevendo. Isso funciona para "tem vocabulário de processo aqui?", mas não para
"quantas linhas tem o arquivo?" — um `Edit` carrega só o trecho alterado, não o
arquivo inteiro, e contar linhas do trecho não diz o tamanho final. Só o arquivo
no disco, depois da escrita, responde isso.

**O orçamento do aviso é o dobro do alvo (80, não 40).** `/handoff` já declara
~40 como alvo; repetir o mesmo número aqui dispararia o aviso em toda sessão só
por variação normal — e é exatamente esse ruído que faz um hook de aviso parar de
ser lido (mesmo racional do escopo do `guard-vocab`, acima). O gate dispara só
quando o handoff dobrou o alvo, que é o sinal de que virou narrativa em vez de
estado derivado.

**O aviso não numera linhas.** Varrendo um trecho solto, o número seria relativo
ao trecho e não corresponderia a nada que se possa abrir. Número errado é pior
que número nenhum, e quem acabou de escrever a linha não precisa de coordenada.

## `/destilar`

Um comando in-session que leva para a documentação do projeto o que um trabalho
fechado virou **estado**, deixando no lugar de origem o que é processo.

**Não é código Go, e isso é decisão de arquitetura:** o `ray` não autora
conteúdo. Destilar exige ler, julgar o que virou estado e redigir — isso é
autoria. O `ray` entrega o prompt e para aí.

**O que tem de valer:**

- **Só destila trabalho concluído.** Trabalho em aberto não se destila.
- **Cada candidato é confirmado no código antes de virar texto.** Procura-se a
  evidência concreta — o símbolo, o teste correspondente, o trecho que implementa
  a invariante. Candidato sem evidência não entra.
- **Não adivinha escopo.** Sem superfície declarada e sem mudança pendente, o
  comando pergunta em vez de inferir.
- **Atualizar documentação de estado é automático; propor decisão travada não.**
  A assimetria é deliberada: descrição sempre contradiz o que estava lá, porque o
  estado mudou — é para isso que serve. Decisão travada é a categoria que existe
  justamente para não ser reaberta sem perguntar, e um agente que a reescreve
  sozinho esvazia a própria categoria.
- **Reporta o que entrou, o que foi recusado por falta de confirmação, e o que
  não tinha nada a destilar.**

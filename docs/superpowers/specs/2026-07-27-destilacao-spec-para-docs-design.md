# Destilação de spec para `docs/` — design

**Data:** 2026-07-27
**Estado:** aprovado, não implementado
**Substitui:** o "item 4 — máquina de publicação cérebro → repo" do handoff de 2026-07-27

## O problema, e por que o desenho anterior caiu

O handoff anterior descrevia o item 4 como uma **máquina de publicação**: a spec
nasce no cérebro, amadurece, e ao virar `status: aprovada` é transformada e
copiada para `docs/specs/NNN-*.md` no repo.

Esse desenho não sobrevive à regra que o próprio projeto adota. `docs/` é
**estado atual** — o que alguém que clonasse o repo amanhã precisaria ler para
entender o sistema como ele é hoje. Uma spec implementada é registro de uma
decisão passada. Uma spec aprovada é plano: descreve o que o sistema **vai** ser.
Nenhuma das duas é estado atual, e publicar qualquer uma delas em `docs/` viola a
regra binária de roteamento.

O motivo original de publicar não era de roteamento, era mecânico: o agente
precisava do arquivo dentro do repo para implementar. Esse motivo **deixou de
existir** quando a integração `brain` passou a expor a vault ao agente por MCP.
Ele lê a spec direto do cérebro; não precisa de cópia.

Segue disso que **a spec nunca sai do cérebro**, e o que viaja para o repo é o
que ela causou:

| Produto da spec | Onde vive |
|---|---|
| Critérios de aceite | testes `CA-NN:` no repo |
| Contrato e invariantes | `docs/architecture.md` |
| Convenção que ficou | `docs/conventions.md` |
| Decisão travada | `<architecture>` do `CLAUDE.md` |
| A spec em si | cérebro, para sempre |

O passo 8 do `<workflow>` já manda fazer exatamente isso. O que falta não é uma
máquina de transporte: é dar dentes a um passo que hoje depende de alguém
lembrar.

## O que se mediu antes de decidir

O desenho anterior previa transformação de dialeto markdown em quatro frentes.
Medido nas 40 specs de `Projetos/Trabalho/Refatoração NextUp Bricks/specs/`, que
são o único corpus real:

| Previsto | Ocorrências reais |
|---|---|
| Callouts customizados | 0 |
| Tarefas com `📅`/`⏫` | 0 |
| Embeds, block refs, tags inline | 0 |
| Wikilinks | 44 — dos quais 40 são o campo `projeto:` do frontmatter |

Sobram 4 wikilinks de corpo, em 3 arquivos, todos apontando para notas que não
existem na vault. A transformação de dialeto, que era a premissa central do item
4, quase não tem objeto.

Também se verificou que o `/sync-vault` do `next_up_bricks` — tratado no handoff
como implementação rival a ser conciliada — **está morto**: os dois caminhos
hardcoded nele apontam para pastas que não existem mais (a vault foi
reorganizada; `next_up_bricks/specs/` não existe), e o repo não tem spec alguma.
Não há concorrente a aposentar. É mais um caso do padrão já conhecido:
configuração apontando para um mundo que deixou de existir.

## A máquina

Um comando in-session, `/destilar NNN`, que o `ray` scaffolda junto de
`/revisar` e `/document`.

Não é código Go. O `ray` não autora conteúdo — destilar exige ler a spec, julgar
o que virou estado e redigir, e isso é autoria. O `ray` entrega o prompt.

### Mapeamento seção → destino

Fixo, derivado das seções do template de spec. O que não couber é reportado, não
improvisado.

| Seção da spec | Destino | Por quê |
|---|---|---|
| Objetivo · Por que agora | fica no cérebro | motivação é processo |
| Fora de escopo | fica no cérebro | delimita a spec, não o sistema |
| Requisitos funcionais | já virou código | não se documenta duas vezes |
| Critérios de aceite | já viraram testes `CA-NN:` | o nome do teste é a documentação |
| Regras de negócio e invariantes | `docs/architecture.md` | o que precisa ser verdade sempre é estado |
| Contratos | `docs/architecture.md` | forma real do sistema |
| Dependências e impacto | fica no cérebro | mapa do trabalho, não do sistema |
| Estratégia de teste específica | `docs/conventions.md`, **só se** virou regra permanente | senão é processo daquela spec |
| Perguntas em aberto | vazia por definição | `aprovada` exige isso |
| Decisões durante a implementação | **dividida** — decisão travada para `<architecture>` do `CLAUDE.md`; história do desvio fica no cérebro | a decisão é estado, o enredo é processo |

### Fluxo

1. Lê a spec no cérebro pelo MCP `brain`. Se o `status` não for `implementada`,
   para: não se destila plano.
2. Extrai os candidatos das quatro seções marcadas.
3. **Confirma cada candidato no código.** A superfície a inspecionar são os
   arquivos nomeados na seção "Dependências e impacto" da spec, mais o que
   `git status`/`git diff` mostrarem por commitar. Para cada candidato, procura a
   evidência concreta: o símbolo do contrato, o teste `CA-NN` correspondente, ou
   o trecho que implementa a invariante. Candidato sem evidência não vira texto.
   Se "Dependências e impacto" estiver vazia e não houver diff pendente, o
   comando não adivinha o escopo: pergunta.
4. Aplica em `docs/architecture.md` e `docs/conventions.md`, e mostra o diff.
5. Decisão travada: propõe o texto e espera OK. Nunca escreve sozinho.
6. Reporta o que entrou, o que foi recusado por falta de confirmação, e o que não
   tinha nada a destilar.

A assimetria do passo 4 contra o 5 é deliberada. Atualizar `docs/` é descrição, e
ela sempre contradiz o que estava lá — o estado mudou, é para isso que serve.
Decisão travada é a categoria que existe justamente para não ser reaberta sem
perguntar; um agente que a reescreve sozinho esvazia a própria categoria.

### Onde ele para

- **A spec contradiz uma decisão travada já registrada** — não reescreve, para.
  Ou a spec estava errada, ou a decisão precisava ter sido reaberta antes de
  implementar. Os dois casos são do usuário.
- **Candidato não confirmado no código** — reporta "a spec diz X, o diff não
  mostra". É o mecanismo que impede `docs/` de nascer mentindo, e é o que
  distingue este desenho de um que apenas copia intenção.
- **Nada a destilar** — diz isso e encerra. Não inventa trabalho.

## O que este desenho desfaz

Trabalho já commitado que passa a estar errado:

- **`docs/specs/TEMPLATE.md.tmpl` sai do scaffold**, e a entrada correspondente
  some do `templateFor` em `internal/scaffold/scaffold.go`. Specs não viajam.
- **`docs/README.md.tmpl` descreve o laço terminando em publicação** (passo 4:
  "publicar a spec aprovada em `docs/specs/`"). Reescrever para terminar em
  destilação.
- **O passo 8 do `<workflow>`, no `CLAUDE.md.tmpl`**, manda atualizar
  `docs/architecture.md` e `docs/conventions.md` à mão ao fechar a spec — que é
  precisamente o que este comando mecaniza. O passo passa a ser "rode
  `/destilar NNN`", e a instrução manual vira o caminho de exceção para quando
  não houver comando disponível. Sem esse ajuste, o laço fica com duas
  instruções concorrentes para a mesma tarefa, e a manual é a que o agente
  encontra primeiro.
- **`Sistema/Templates/Template - Feature.md`, na vault**, afirma que "`aprovada`
  é o gatilho de publicação para o `docs/` do repositório". Falso sob este
  desenho. Além disso é o mais pobre dos dois templates de spec que existem: não
  tem Contratos, Regras de negócio e invariantes, nem Decisões durante a
  implementação — exatamente as seções que a destilação consome. Precisa
  absorvê-las do `TEMPLATE.md.tmpl` que está saindo do scaffold, senão o comando
  lê seções que não existem.

Novo: `.claude/commands/destilar.md.tmpl`, mais a entrada no `templateFor` e a
menção no `docs/README.md`.

## Testes

No `ray`, o mesmo padrão dos outros comandos scaffoldados: presença no
`templateFor` e renderização do template. O comando em si é prompt e não tem
teste unitário.

A verificação real é executá-lo contra uma spec de verdade. O corpus existe: as
40 specs do NextUp Bricks, todas `implementada`, num repo que ainda não tem
`docs/` — o que torna o primeiro `/destilar` também o primeiro teste de que o
mapeamento seção→destino aguenta um caso real.

## Decisões tomadas nesta rodada

1. Publicar spec implementada não faz sentido: `docs/` é estado atual. Por
   extensão, publicar spec aprovada também não — é plano.
2. A máquina vira **destilação**, não publicação.
3. Comando próprio `/destilar NNN`, simétrico a `/revisar NNN`. Não absorvido
   pelo `/revisar` (revisor que produz revisa o próprio trabalho, o que colide
   com a divisão em assentos) nem pelo `/document` (comando de dois modos tende a
   fazer os dois pela metade).
4. A spec propõe os candidatos; o código confirma. Documenta-se o que é, não o
   que foi prometido.
5. Aplica em `docs/`; propõe a decisão travada.

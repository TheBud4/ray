# Arquitetura — ray

Como o sistema é montado **hoje**. Descreve estado, não intenção: se algo aqui
ficar defasado do código, virou ficção — corrija junto com a mudança que o
defasou.

As decisões travadas ("não reabrir sem perguntar") **não** moram aqui: moram na
seção `<architecture>` do `CLAUDE.md`, que é o que o agente lê em todo turno.
Aqui fica a descrição; lá, o que não se discute.

Stack: Go 1.25 · Cobra v1.10.2 · `gopkg.in/yaml.v3`. Módulo
`github.com/TheBud4/ray`.

## Visão em uma tela

O `ray` transforma uma **receita** (perfil YAML) em decisões, e depois em
efeitos no disco. O núcleo de domínio é puro: recebe receita, devolve o que
fazer, sem tocar disco nem rede.

```text
  receita (~/.ray/profiles/*.yaml)
        │  (lê e valida)
        ▼
  ┌───────────┐   (traduz integrações em ações)   ┌───────────┐
  │  profile  │ ────────────────────────────────▶ │ installer │
  └───────────┘                                   └───────────┘
        │                                               │  (delega processo externo)
        │ (orquestra os 10 passos)                      ▼
        ▼                                         ┌───────────┐
  ┌───────────┐                                   │  runner   │──▶ npx, uv, git
  │  initai   │                                   └───────────┘
  └───────────┘
        │  (escreve a árvore de orientação)
        ▼
  ┌───────────┐   (renderiza templates embutidos)
  │ scaffold  │ ──▶ CLAUDE.md, SECURITY.md, docs/, .claude/
  └───────────┘
```

`runner` é a **única fronteira** do `ray` para processos externos — é o que
torna tudo o mais testável só com memória, trocando `ExecRunner` por
`FakeRunner`.

## Onde mora o quê

```text
internal/
├── cmd/          # ponto de montagem da árvore de comandos (Cobra)
│
├── profile/      # modelo de receita: o que um perfil declara
├── installer/    # traduz as integrações de uma receita em ações
├── initai/       # orquestra os 10 passos de `ray init ai`
├── scaffold/     # escreve a árvore de orientação; templates embutidos
├── update/       # `ray update`: re-aquisição protegendo edição local
│
├── acquire/      # materializa conteúdo por fonte (git pinado / CLI)
├── store/        # cache content-addressed do conteúdo adquirido
│
├── claudecfg/    # merge idempotente de .claude/settings.json
├── mcp/          # modelo de servidor MCP + merge idempotente de .mcp.json
├── vault/        # valida o cérebro do usuário (valida, nunca cria)
│
├── economy/      # mecanismos de Token Economy
├── metrics/      # proxies de atividade desses mecanismos
├── learn/        # máquina verificável do modo learn
│
├── rayconfig/    # lê e grava ~/.ray/config.yaml e o State
├── raypaths/     # resolve onde o ray guarda estado em disco
├── runfile/      # aliases do `ray run` (ray.yaml do projeto + global)
├── preflight/    # fonte única de checagem de dependências externas
├── runner/       # ÚNICA fronteira para processos externos
└── openutil/     # abre um caminho no app default do sistema
```

## Regras de dependência

Quem pode importar quem — e principalmente **quem não pode**, que é a metade que
o código sozinho não conta:

- Só `runner` executa processo externo. Nenhum outro pacote chama `os/exec`
  direto — quem precisa de processo recebe um `Runner` por parâmetro.
- Só `preflight` decide se uma dependência externa existe. Não replique a
  checagem no pacote que vai usá-la.
- `raypaths` é o único que resolve caminho de estado. Nenhum pacote monta
  `~/.ray/...` à mão.
- O núcleo de domínio (`profile`, `economy`, `learn`) **não** toca disco nem
  rede — é o que o mantém testável só com memória.
- `vault` **valida** o cérebro, nunca o cria. O `ray` é consumidor da vault do
  usuário, não dono dela.

## Onde entra código novo

- Comando novo do CLI → `internal/cmd/<verbo>.go`, montado na árvore Cobra.
- Regra que decide algo a partir da receita → o pacote de domínio, sem I/O.
- Efeito no disco de um projeto scaffoldado → `internal/scaffold`, como template.
- Chamada a ferramenta externa → passa por `runner`; a checagem de existência,
  por `preflight`.

Não confunda `scaffold/templates/` com o `docs/` deste repositório: o primeiro é
o que o `ray` **escreve nos projetos dos outros**; o segundo descreve o `ray`.
Editar um achando que é o outro é o erro mais caro aqui.

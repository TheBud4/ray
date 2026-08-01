<p align="center">
  <img src="docs/assets/logo.png" width="340" alt="Logo do ray: óculos escuros sobre um splash de tinta laranja">
</p>

<h1 align="center">ray</h1>

<p align="center">
  CLI pessoal em Go para criar projetos e montar ambientes de IA econômicos em tokens.
</p>

<p align="center">
  <a href="https://github.com/TheBud4/ray/actions/workflows/ci.yml"><img src="https://github.com/TheBud4/ray/actions/workflows/ci.yml/badge.svg" alt="Status do CI"></a>
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="Licença MIT"></a>
</p>

`ray` é uma CLI pessoal em Go que (a) cria projetos novos de um stack e (b)
monta, com um comando, um ambiente de desenvolvimento com IA (Claude Code)
econômico em tokens e rico em ferramentas.

A feature âncora é `ray init ai`, que instala num diretório: skills, agentes,
comandos e servers MCP curados por stack, regras que guiam a IA, e docs base.
O `ray` **não embute conteúdo de componentes** — ele orquestra installers
externos (`npx skills`, `npx claude-code-templates`, `uv tool install`)
conforme uma receita (`~/.ray/profiles/<nome>.yaml`), editável e agnóstica ao
binário.

## Instalação

```sh
go install .
# ou, dentro do repo:
make install
```

Depois, confira as dependências externas:

```sh
ray doctor          # checa npx, python3.10+, uv, headroom, graphify
ray doctor --fix    # instala o que o ray consegue instalar sozinho
```

## Quick start

```sh
# projeto novo do zero (cria a pasta, git init, e já monta a IA)
ray new go meuprojeto

# ambiente de IA numa pasta que já existe
cd algum-projeto-existente
ray init ai --profile go
```

## O ambiente de IA é versionado

O `ray init ai` escreve o ambiente **dentro do seu repositório**, e ele é para
ser commitado. Quem clonar recebe os mesmos agentes, skills, regras e servidores
MCP sem instalar nada — o ambiente viaja com o código em vez de morar na máquina
de quem o montou.

Por isso o `ray` escreve um bloco no seu `.gitignore` com negações explícitas: o
conteúdo de IA vendorizado é commitado mesmo que alguma regra anterior o
ignorasse.

**Commitado:**

`.claude/skills/` · `.claude/agents/` · `.claude/commands/` ·
`.claude/settings.json` · `.claude/.ray-profile` · `.mcp.json` · `docs/` ·
`**/.ray-origin` · `**/LICENSE`

**Nunca commitado** — runtime, segredo e material pessoal:

`.claude/.local/` (diário de aprendizado e marcos do modo learn) ·
`.claude/.ray-metrics/` · `.claude/handoff.md` · `graphify-out/` · `.env` ·
`*.local`

Ao terminar, o `ray init ai` imprime o `git add` com os caminhos que aquela
execução criou.

## Comandos

| Comando | O que faz |
|---|---|
| `ray new <perfil> <nome>` | Cria um projeto do stack (`create` da receita + `git init`) e monta a IA nele. |
| `ray init ai --profile <n> [path]` | Monta o ambiente de IA numa pasta existente (default: diretório atual). |
| `ray run [alias] [-- extra]` | Roda um alias de `ray.yaml` (projeto) ou `~/.ray/commands.yaml` (global); sem alias, lista os disponíveis. |
| `ray profile list\|show\|add\|edit\|remove\|path` | Gerencia as receitas em `~/.ray/profiles`. |
| `ray brain set\|status\|open\|path` | Aponta o ray para a sua vault Obsidian e a expõe ao agente por MCP. Valida o caminho; nunca cria nem reorganiza. |
| `ray doctor [--fix]` | Checa/instala dependências externas. |
| `ray update [path]` | Re-adquire ferramentas e conteúdo; protege edições suas por hash de conteúdo (exige `--force` para sobrescrever). `--no-global` deixa as ferramentas da máquina onde estão e atualiza só o projeto. |
| `ray stats [path]` | Agrega as métricas-proxy de economia de token registradas em `.claude/.ray-metrics/`. |
| `ray status [path]` | Diagnostica o ambiente vendorizado: o que o `ray update` faria com cada componente, se o `.claude/` está versionado, se o bloco do `.gitignore` segue intacto e se os servidores MCP resolvem. Ambiente são imprime duas linhas. |
| `ray learn check [path]` | Roda o `verify` do marco corrente e registra a passagem. |

Flags globais: `--verbose` (`-v`), `--dry-run` (imprime o que seria feito, sem
executar nem escrever nada) e `--version`, que não tem forma curta — o `-v` é da
verbosidade.

O `--version` sai do que o `go install` grava no binário: build de tag mostra a
versão do módulo, build local mostra `devel` com revisão, data e `dirty` quando
a árvore não estava limpa — um binário de árvore suja não corresponde a commit
nenhum. Arquivo não rastreado conta como sujeira, mesmo que não seja compilado:
um `.go` ainda não commitado entra no binário, e o marcador prefere errar para
`dirty` a deixar passar esse caso.

```sh
ray --version
# ray version devel (f905791, 2026-08-01T00:41:58Z, dirty)
```

### `--mode build\|learn`

Todo `ray init ai`/`ray new` aceita `--mode` (default `build`):

- **`build`**: a IA implementa normalmente, sem restrições.
- **`learn`**: a IA vira mentora/revisora. Um hook `PreToolUse` bloqueia
  `Edit`/`Write`/`MultiEdit` fora de uma allowlist de documentação
  (`*.md` e tudo sob `docs/` e `.claude/`) — a IA explica e revisa, mas não
  escreve código diretamente.

Em modo learn a IA negocia um contrato com você na primeira sessão — o quanto
você quer que ela entregue de cada vez, o que você quer conseguir fazer
sozinho, e o que vai contar como pronto. O combinado e o seu progresso vivem
em `.claude/.local/`, que é gitignorado: compartilhar o repo não expõe o seu
aprendizado.

Peça "mais" quando uma dica não bastar. A resposta completa está sempre
disponível, mas ela te pergunta antes o que você já tentou.

## Completion

Cobra gera completion de graça para bash, zsh, fish e PowerShell:

```sh
# bash (uma vez, ou adicione ao seu .bashrc)
ray completion bash > /etc/bash_completion.d/ray

# zsh
ray completion zsh > "${fpath[1]}/_ray"

# fish
ray completion fish > ~/.config/fish/completions/ray.fish
```

Veja `ray completion --help` para os detalhes de cada shell.

## Desenvolvimento

```sh
make build       # go build ./...
make test        # go test ./...
make vet         # go vet ./...
make fmt         # gofmt -w .
make fmt-check   # falha se algo precisar de gofmt
make ci          # fmt-check + vet + test (o que o CI roda)
```

Detalhes de arquitetura, modelo de dados e o plano de fases de construção
estão em [`docs/ray-build-guide.md`](docs/ray-build-guide.md).

## Licença

[MIT](LICENSE) — © 2026 TheBud4.

<h1 align="center">
  <img src="docs/assets/logo.png" width="240" alt="Logo do ray: óculos escuros sobre um splash de tinta laranja"><br>
  ray
</h1>

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

## Comandos

| Comando | O que faz |
|---|---|
| `ray new <perfil> <nome>` | Cria um projeto do stack (`create` da receita + `git init`) e monta a IA nele. |
| `ray init ai --profile <n> [path]` | Monta o ambiente de IA numa pasta existente (default: diretório atual). |
| `ray run [alias] [-- extra]` | Roda um alias de `ray.yaml` (projeto) ou `~/.ray/commands.yaml` (global); sem alias, lista os disponíveis. |
| `ray profile list\|show\|add\|edit\|remove\|path` | Gerencia as receitas em `~/.ray/profiles`. |
| `ray vault init\|status\|open\|path` | Gerencia o vault de conhecimento da IA (`~/.ray/vault`, compatível com Obsidian). |
| `ray docs init\|set\|open\|path` | Gerencia o vault central de documentação do usuário (cross-project). |
| `ray doctor [--fix]` | Checa/instala dependências externas. |

Flags globais: `--verbose`, `--dry-run` (imprime o que seria feito, sem
executar nada).

### `--mode build\|learn`

Todo `ray init ai`/`ray new` aceita `--mode` (default `build`):

- **`build`**: a IA implementa normalmente, sem restrições.
- **`learn`**: a IA vira mentora/revisora. Um hook `PreToolUse` bloqueia
  `Edit`/`Write`/`MultiEdit` fora de uma allowlist de documentação
  (`*.md`, `docs/**`, `.claude/**`) — a IA explica e revisa, mas não escreve
  código diretamente.

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

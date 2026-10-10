# Política de segurança — ray

O `ray` executa processos externos, instala ferramentas de terceiros e escreve
arquivos dentro de repositórios de outras pessoas. A superfície de risco é essa,
e não a de uma aplicação web — as regras abaixo tratam do que este programa
realmente faz.

## Severidade

- **[MUST]** — bloqueante. Uma mudança que viola não entra.
- **[SHOULD]** — esperado. Divergir exige justificativa escrita.

## Regras para código gerado por IA

- **[MUST]** A IA é assistente, não autoridade de segurança. Sugestão que toca
  execução de processo, instalação de pacote ou escrita fora do diretório do
  projeto é revisada por humano antes de entrar.
- **[MUST]** Nome de pacote e URL de origem **nunca** vêm de memória do modelo. Vêm do manifesto, da receita ou do usuário — verificados.

## Execução de processo externo

O risco central deste programa: ele roda `uv`, `graphify`, `git` e os passos de
`create:` e do `ray.yaml` com argumentos que saem de arquivos de receita.

- **[MUST]** Todo processo externo passa por `internal/runner`. É o que mantém a
  superfície auditável em um lugar só.
  **Duas exceções, e elas são fechadas:**
  1. `spawnEditor` em `internal/cmd/profile.go` chama `exec.Command` direto para
     abrir o `$EDITOR` do usuário, porque um editor interativo precisa herdar o
     terminal e a interface do `runner` bufferiza stdout/stderr em `Result` —
     ela não consegue entregar um TTY cru.
  2. `preflight.PathLooker` (`internal/preflight/pathlooker.go`) chama
     `exec.LookPath`, que resolve um nome contra o `$PATH` sem criar processo.
     Não é execução, mas é um import de `os/exec` fora do `runner`, e por isso
     está listado.

  Nenhum `os/exec` novo entra fora dessas fronteiras; um caso a mais exige
  discussão, não precedente.
- **[MUST]** Argumento de comando é passado como **elemento de slice**, nunca
  concatenado numa string de shell. Não invocar através de `sh -c` com entrada
  interpolada.
- **[MUST]** Valor vindo de receita (`~/.ray/profiles/*.yaml`) é entrada não
  confiável: a receita é um arquivo editável, e uma receita compartilhada é
  código de terceiro. Validar antes de virar argumento.
- **[SHOULD]** Falha de processo externo é reportada com o comando manual
  equivalente, para o usuário poder auditar o que seria executado.

## Conteúdo e instalação de terceiros

- **[MUST]** O `ray` **não baixa conteúdo**. Skills, agentes e comandos são
  copiados de `~/.ray/components/`, uma pasta que o usuário mantém à mão; a
  procedência e a licença do que está lá são de quem a curou. Nenhum código
  novo busca conteúdo pela rede, nem por `git`, nem por `npx`.
- **[MUST]** A única instalação de terceiro são as integrações da receita
  (`headroom`, `code_graph`), por `uv tool install` e `graphify install`, via
  `runner`. Conferir o **nome** do pacote caractere a caractere antes de
  acrescentar ou alterar um: typosquatting acerta justamente quem confia no
  autocompletar e em nome ditado por IA — um caractere trocado num nome
  plausível é a via mais barata de execução de código na máquina do usuário.
- **[SHOULD]** Telemetria de installer de terceiro desligada quando o instalador
  permitir.

## Escrita no disco do usuário

- **[MUST]** Escrita fica dentro do diretório alvo e de `~/.ray`. Nunca escrever
  fora sem o usuário ter pedido aquele caminho.
- **[MUST]** Um symlink no projeto que leve para fora dele não é seguido: `init ai`
  e `update` recusam o destino antes de qualquer escrita (`internal/safepath`).
  O teste enumera o que um `init ai` escreve num projeto vazio: um destino
  novo que o guard esqueça faz o teste falhar. Escrita transitória (o probe de
  gravabilidade) usa nome aleatório e criação exclusiva, nunca um nome fixo que
  um clone possa ter plantado como symlink.
- **[MUST]** Não sobrescrever arquivo existente sem `--force` explícito, e
  `.claude/handoff.md` **nunca** é sobrescrito nem com `--force` — é estado vivo
  do usuário.
- **[MUST]** O `ray` valida a vault do usuário; **nunca** cria, move nem
  reorganiza. `ray brain set` recusa um caminho que não passa em `vault.Verify`
  e não o grava em `config.yaml`.

## Segredos

- **[MUST]** Nenhum segredo em arquivo versionado — chaves, tokens, senhas.
- **[MUST]** O `ray` não pede, não guarda e não loga credencial. Token de MCP que
  apareça em `.mcp.json` de um projeto pertence ao usuário: não copiar para
  cache, log ou relatório.
- **[MUST]** Nunca logar caminho absoluto de usuário junto com conteúdo de
  arquivo de config em mensagem de erro pública.

## Dependências

- **[MUST]** `go.mod`/`go.sum` versionados.
- **[MUST]** Revisar antes de adicionar: preferir biblioteca padrão. Este projeto
  tem **duas** dependências diretas de propósito — cada nova precisa justificar
  por que a stdlib não serve.
- **[SHOULD]** `govulncheck` antes de release.

## Checklist antes de integrar

- [ ] Nenhum `os/exec` novo fora de `internal/runner`
- [ ] Nenhum argumento de comando concatenado em string de shell
- [ ] Nenhuma aquisição de conteúdo pela rede; nome de pacote novo conferido
- [ ] Nenhum segredo, token ou caminho pessoal em código, teste ou fixture
- [ ] `make ci` verde

## Reportar uma vulnerabilidade

Repositório pessoal: reporte direto ao autor. Não abra issue pública com detalhe
explorável e não commite prova de conceito no repo.

## Regra final

Na dúvida entre conveniência e segurança em algo que roda na máquina de outra
pessoa, escolha segurança e explique o custo. Este programa é executado com as
permissões do usuário, dentro dos repositórios dele.

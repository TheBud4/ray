# Política de segurança — ray

O `ray` executa processos externos, baixa conteúdo de terceiros e escreve
arquivos dentro de repositórios de outras pessoas. A superfície de risco é essa,
e não a de uma aplicação web — as regras abaixo tratam do que este programa
realmente faz.

## Severidade

- **[MUST]** — bloqueante. Uma mudança que viola não entra.
- **[SHOULD]** — esperado. Divergir exige justificativa escrita.

## Regras para código gerado por IA

- **[MUST]** A IA é assistente, não autoridade de segurança. Sugestão que toca
  execução de processo, aquisição de conteúdo ou escrita fora do diretório do
  projeto é revisada por humano antes de entrar.
- **[MUST]** Nome de pacote, URL de origem e `ref` de git **nunca** vêm de
  memória do modelo. Vêm do manifesto, da receita ou do usuário — verificados.

## Execução de processo externo

O risco central deste programa: ele roda `npx`, `uv`, `git` e outros com
argumentos que saem de arquivos de receita.

- **[MUST]** Todo processo externo passa por `internal/runner`. É o que mantém a
  superfície auditável em um lugar só.
  **Única exceção, e ela é fechada:** `spawnEditor` em `internal/cmd/profile.go`
  chama `exec.Command` direto para abrir o `$EDITOR` do usuário, porque um editor
  interativo precisa herdar o terminal e a interface do `runner` bufferiza
  stdout/stderr em `Result` — ela não consegue entregar um TTY cru. Nenhum
  `os/exec` novo entra fora dessas duas fronteiras; um terceiro caso exige
  discussão, não precedente.
- **[MUST]** Argumento de comando é passado como **elemento de slice**, nunca
  concatenado numa string de shell. Não invocar através de `sh -c` com entrada
  interpolada.
- **[MUST]** Valor vindo de receita (`~/.ray/profiles/*.yaml`) é entrada não
  confiável: a receita é um arquivo editável, e uma receita compartilhada é
  código de terceiro. Validar antes de virar argumento.
- **[SHOULD]** Falha de processo externo é reportada com o comando manual
  equivalente, para o usuário poder auditar o que seria executado.

## Aquisição e vendorização de conteúdo

- **[MUST]** Fonte git é adquirida **pinada por `ref`**. Nunca `HEAD` de branch
  móvel — conteúdo que muda sob os pés quebra a reprodutibilidade que é o ponto
  do produto.
- **[MUST]** Conferir o **nome** do pacote caractere a caractere antes de
  instalar. Typosquatting acerta justamente quem confia no autocompletar e em
  nome ditado por IA — um caractere trocado num nome plausível é a via mais
  barata de execução de código na máquina do usuário.
- **[MUST]** Registrar procedência e licença do que é vendorizado. Conteúdo sem
  origem conhecida dentro de `.claude/` do usuário é dívida que ninguém
  consegue auditar depois.
- **[SHOULD]** Telemetria de installer de terceiro desligada quando o instalador
  permitir.

## Escrita no disco do usuário

- **[MUST]** Escrita fica dentro do diretório alvo e de `~/.ray`. Nunca escrever
  fora sem o usuário ter pedido aquele caminho.
- **[MUST]** Não sobrescrever arquivo existente sem `--force` explícito, e
  `.claude/handoff.md` **nunca** é sobrescrito nem com `--force` — é estado vivo
  do usuário.
- **[MUST]** O `ray` valida a vault do usuário; **nunca** cria, move nem
  reorganiza. Um caminho de cérebro inválido emite aviso e não registra o server
  MCP.

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
- [ ] Fonte git nova está pinada por `ref`
- [ ] Nenhum segredo, token ou caminho pessoal em código, teste ou fixture
- [ ] `make ci` verde

## Reportar uma vulnerabilidade

Repositório pessoal: reporte direto ao autor. Não abra issue pública com detalhe
explorável e não commite prova de conceito no repo.

## Regra final

Na dúvida entre conveniência e segurança em algo que roda na máquina de outra
pessoa, escolha segurança e explique o custo. Este programa é executado com as
permissões do usuário, dentro dos repositórios dele.

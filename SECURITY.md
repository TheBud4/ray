# Política de segurança — ray

O `ray` executa processos externos, instala ferramentas de terceiros e escreve
arquivos dentro de repositórios de outras pessoas. Essa é a superfície de risco
principal. A outra é o `ray serve`, um servidor HTTP local que existe só enquanto
o comando roda e só aceita o navegador da própria sessão: ele tem as regras de
uma aplicação web, na seção dele. As regras abaixo tratam do que este programa
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
  1. `editorCommand`/`spawnEditor` em `internal/cmd/profile.go`, usados por
     `profile edit` e `template edit`, abrem o `$EDITOR` do usuário sem passar
     pelo `runner`, porque um editor interativo precisa herdar o terminal e a
     interface do `runner` bufferiza stdout/stderr em `Result` — ela não
     consegue entregar um TTY cru. `exec.LookPath` decide se o `$EDITOR` é um
     executável inteiro; senão é quebrado em palavras, sem shell, e roda por
     `exec.Command`.
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
- **[MUST]** A cópia de um componente (`~/.ray/components/<Name>`) não leva para o
  projeto conteúdo de fora da pasta do componente: um link de arquivo dentro dele
  só passa se o alvo ficar dentro do componente; link para fora, pendente ou para
  pasta recusa o `init ai` e o `update` antes de qualquer efeito
  (`store.ResolveSources`). A própria pasta do componente pode ser um symlink.
- **[MUST]** Não sobrescrever arquivo existente sem `--force` explícito, e
  `.claude/handoff.md` **nunca** é sobrescrito nem com `--force` — é estado vivo
  do usuário.
- **[MUST]** O `ray` valida a vault do usuário; **nunca** cria, move nem
  reorganiza. `ray brain set` recusa um caminho que não passa em `vault.Verify`
  e não o grava em `config.yaml`.

## Servidor local (`ray serve`)

O `ray serve` expõe a edição de `~/.ray/templates` a um navegador, e o mesmo
navegador carrega páginas hostis que podem tentar alcançar o servidor. O raio de
dano não é só `~/.ray/templates`: um template editado é o que o `init ai` grava
nos projetos seguintes. As defesas ficam em `internal/serve/security.go` e valem
para toda requisição, antes de qualquer rota.

- **[MUST]** Escuta só em `127.0.0.1`, literal (`serve.Listen`) — nunca
  `0.0.0.0`, `localhost` nem outra interface.
- **[MUST]** Toda chamada a `/api/*`, de leitura ou de escrita, exige o token da
  sessão no cabeçalho `X-Ray-Token`: 32 bytes de `crypto/rand`, novos a cada
  execução, comparados em tempo constante; sem ele, ou errado, 401. Os estáticos
  (`/`, `app.js`, `app.css`, `favicon.svg`) saem sem token, porque a página
  precisa carregar antes de ter o token, e por isso são embutidos no binário e não
  carregam dado do usuário.
- **[MUST]** O `Host` tem de ser exatamente `127.0.0.1:<porta>`; qualquer outro
  (`localhost:…`, `[::1]:…`, outra porta, vazio) é 403, em qualquer rota.
- **[MUST]** Uma escrita (`PUT`, `POST`) exige `Content-Type: application/json` e
  um `Origin` **presente** e igual a `http://127.0.0.1:<porta>`; senão 403. Só
  existem `GET`, `PUT` e `POST` (o resto é 405), e `GET` nunca muda estado.
- **[MUST]** Nenhuma defesa depende de cookie: o servidor não grava cookie nem
  responde cabeçalho CORS (`Access-Control-*`).
- **[MUST]** Toda resposta, inclusive as recusas, leva
  `Content-Security-Policy` (`default-src 'none'`, script, estilo, conexão e
  imagem só de `'self'`, `frame-ancestors 'none'`), `X-Content-Type-Options:
  nosniff` e `Referrer-Policy: no-referrer`; as de `/api/*` também
  `Cache-Control: no-store`.
- **[MUST]** Corpo de até 2 MiB (413) e conteúdo decodificado de até 1 MiB
  (422). O `http.Server` tem prazo de leitura do cabeçalho (5 s), de leitura
  (15 s), de escrita (30 s) e de ociosidade (60 s).
- **[MUST]** Dado do usuário (nome, conteúdo, mensagem de erro, diff) entra no DOM
  só por `textContent`/`createElement`. Um teste estático recusa no JS embutido
  `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`, `eval`,
  `new Function`, `srcdoc`, `javascript:` e atributo `on*` ou `style` montado
  por `setAttribute`, e no HTML, script, estilo e manipulador inline. O JS lê o token do fragmento da URL, apaga o fragmento
  (`history.replaceState`) e o guarda só em `sessionStorage`.
- **[MUST]** O navegador manda só o **nome** do template, que vale apenas se for
  igual a um da lista do catálogo; o destino é sempre montado pelo `catalog`.
  O Salvar recusa gravar através de symlink — o alvo ou qualquer diretório entre
  `~/.ray/templates` e ele; o Restaurar padrão recusa, como no CLI, um alvo que é
  symlink. O confinamento é contra a entrada do navegador, não contra links que
  o próprio usuário põe em `~/.ray`: a pasta `~/.ray/templates` pode ser um link.
- **[MUST]** Hooks (`*.sh.tmpl`) são somente leitura na web: Salvar e Restaurar
  respondem 403. Um hook vira script executável nos projetos, e um desvio aqui
  seria execução de código; ele se edita pelo `ray template edit`.
- **[MUST]** Toda escrita (Salvar e Restaurar) passa por um único lock, e o
  Salvar leva o hash do conteúdo que a página leu: se o disco mudou desde então,
  409 e nada é gravado.
- **[MUST]** O token nunca vai para o argv de outro processo — o argv é legível
  por qualquer usuário da máquina. O `ray` grava um HTML 0600 num diretório
  temporário 0700 (no Windows, o diretório temporário do próprio usuário), que
  redireciona para a URL com o token; quem abre o navegador recebe só o caminho
  desse arquivo, que é apagado ao sair. Se o diretório não puder ser privado, o
  comando recusa e manda usar `--no-open`. Com `--no-open` a URL com o token é
  impressa no terminal, com o aviso de que quem lê aquele terminal pode usá-la.

**O que essas regras não cobrem:**

- Uma página hostil que pede direto `http://127.0.0.1:<porta>` manda o `Host`
  certo: quem a barra é o token (que ela não tem) e a regra do `Origin`. A
  conferência do `Host` existe contra DNS rebinding.
- Outro processo do mesmo usuário que leia o diretório temporário durante a
  sessão lê o token.
- Um template que gira sem produzir saída não pode ser interrompido
  (`text/template` não aceita cancelamento). O prazo de 5 s do preview e da
  conferência de render antes de gravar só limita a frequência: no máximo uma
  goroutine abandonada nova por prazo vencido em cada chamada, ou seja, até duas
  a cada 5 s somando preview e gravação. A saída do render tem teto de 4 MiB.
- Navegadores em sandbox (snap, flatpak) podem não enxergar o `/tmp` e não abrir o
  lançador; nesse caso, `--no-open`.

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
- [ ] No `ray serve`: dado do usuário só sai por `/api/*` (estático não tem
      token) e entra no DOM só por `textContent`; rota que escreve passa pelo
      lock de escrita
- [ ] `make ci` verde

## Reportar uma vulnerabilidade

Repositório pessoal: reporte direto ao autor. Não abra issue pública com detalhe
explorável e não commite prova de conceito no repo.

## Regra final

Na dúvida entre conveniência e segurança em algo que roda na máquina de outra
pessoa, escolha segurança e explique o custo. Este programa é executado com as
permissões do usuário, dentro dos repositórios dele.

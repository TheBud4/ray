# Convenções — ray

As convenções que o código segue **hoje**. Descreve estado, não intenção: se algo
aqui ficar defasado do código, virou ficção — corrija junto com a mudança que o
defasou.

## Código e nomeação

- **Idioma:** doc de pacote e comentários em **português**; identificadores,
  mensagens de teste e mensagens de commit em **inglês**. Os dois convivem de
  propósito — o comentário explica a decisão para quem mantém, o identificador
  segue o idioma do Go.
- Todo pacote abre com `// Package <nome> <o que faz>`, uma frase. Os 21 pacotes
  de `internal/` seguem isso, sem exceção; um pacote novo sem doc destoa.
- O comentário explica **decisão, invariante ou risco** — nunca repete o que a
  linha ao lado já diz.
- Referência a seção de doc de design (`design §8.1`, `build guide §7`) é o
  padrão em doc de pacote, e ajuda a achar o porquê.

## Testes

- `_test.go` ao lado do código, mesmo pacote.
- Nome de teste em inglês, descrevendo comportamento:
  `TestWriteFilesNoOverwriteUnlessForce`.
- Mensagem de falha no formato `got = %v, want %v` — o que se obteve antes do
  que se esperava.
- Efeito em disco se testa com `t.TempDir()`, nunca em caminho real.
- Processo externo se testa com `FakeRunner`, nunca chamando o binário.
- **`CA-NN` não se aplica aqui.** É a convenção dos projetos que o `ray`
  scaffolda, onde o nome do teste liga o código à spec. O `ray` não trabalha por
  spec numerada; não force a convenção nos testes dele.

## Commits

- **Conventional Commits.** Tipos em uso: `feat`, `fix`, `docs`, `refactor`,
  `chore`. Escopo entre parênteses quando ajuda: `feat(scaffold):`.
- **Mensagem em inglês**, e explica **o resultado entregue** — não a lista de
  arquivos. O corpo diz por que a mudança existe; quem quer os arquivos roda
  `git show --stat`.
- **Pequenos e coerentes.** Não misturar no mesmo commit feature, refatoração,
  correção, formatação, dependências e documentação.
- O commit de código inclui os testes relacionados — nunca em commits separados.
- **Sem trailer de co-autoria.** Nenhum `Co-Authored-By` de ferramenta ou agente
  de IA em mensagem de commit — nunca, em nenhum commit. O autor é quem assina.

## Branches

Trabalho vai direto em `main`, com push explícito. Não há fluxo de PR próprio.

## Anti-padrões

Os erros que este projeto já cometeu e não quer repetir — um por linha, sempre
com a correção ao lado, para virar item de revisão em vez de prosa:

- ❌ Documentação apontando para artefato que o scaffold não escreve mais →
  ✅ ao remover um path do `templateFor`, varrer `internal/` e `docs/` por quem
  o citava. Um comando que lê de caminho inexistente nasce quebrado em todo
  projeto novo, e nenhum teste pega.
- ❌ Instruir o agente a buscar por um campo que só a vault do autor tem →
  ✅ só assumir o que o próprio scaffold estabelece.
- ❌ Mensagem ao usuário escrita em português, no meio de uma saída em inglês →
  ✅ a regra de idioma tem uma fronteira só, e ela não é o arquivo: **comentário
  e doc de pacote em português, tudo que o usuário lê em inglês**. Aconteceu
  três vezes no mesmo `internal/initai/initai.go`, e nenhuma apareceu na
  revisão — um aviso só é visto quando o caminho que o emite roda. A exceção
  é **conteúdo scaffoldado** (templates, bloco do `.gitignore`): aquilo é
  material do projeto gerado, que é pt-BR por inteiro, não mensagem do CLI.

Esta lista cresce por acúmulo: um item entra quando o mesmo erro aparece pela
segunda vez, não na primeira. Se um item aqui for verificável por lint ou teste,
a regra pertence ao gate, não a esta lista.

## Artefatos que andam juntos

Pares em que mexer num obriga a atualizar os outros **no mesmo ciclo** — é a
lista que impede o repo de acumular arquivo gerado defasado:

- `internal/scaffold/scaffold.go` (`templateFor`) → `internal/profile/defaults.go`
  (`baseScaffoldFiles`) → os testes de espelho dos dois → o template em
  `internal/scaffold/templates/`.
- Mudança na árvore que o scaffold escreve → a árvore documentada em
  `docs/ray-build-guide.md`.
- `internal/scaffold/templates/claude/hooks/*.sh.tmpl` → as cópias que o próprio ray usa em `.claude/hooks/` (regeneradas do template, nunca editadas à mão). **Este par tem gate:** `TestRayOwnHooksMatchTemplates` compara byte-a-byte e confere o modo executável; `TestRayOwnHooksHaveNoStrays` recusa `.sh` órfão no diretório. Os dois derivam de `SystemFiles(ModeBuild)`, então um hook novo entra no gate sozinho.

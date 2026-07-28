> ⚠️ **Documento congelado (tutorial de Go usando o `ray` v1).**
> Este roadmap ensina **Go** construindo uma versão *anterior* do `ray` e, de
> propósito, **não** acompanha o design de ambientes reprodutíveis. Alguns
> detalhes aqui (p.ex. `.claude/` como efêmero, `.gitignore` com `graphify-out/`,
> semântica antiga de `--global`) foram **substituídos** — para o modelo vigente,
> veja o `ray-build-guide.md`. Mantido intacto pelo seu valor **pedagógico** (aprender
> Go do zero), não como referência do produto atual.

---

# `ray` — Tutorial completo: programando uma CLI em Go do zero

> **Para quem é isto.** Você quer construir o `ray` à mão e, no caminho, aprender
> Go de verdade — não copiar e colar. Este documento ensina os **fundamentos da
> linguagem** e, em paralelo, **como cada peça da CLI é programada**, usando o
> próprio `ray` como projeto-escola.
>
> **Como usar.** Leia a **Parte 1** (fundamentos) com calma uma vez. Depois siga a
> **Parte 2** (construção) na ordem, digitando tudo você mesmo. O
> `ray-build-guide.md` é a referência de _o quê/por quê_; este arquivo é o _como_,
> explicado.
>
> **Versões:** Go **1.25** (ago/2025) · Cobra **v1.10.2** · `gopkg.in/yaml.v3`.
> Módulo: `github.com/murilopmr/ray`.

---

# Índice

- **Parte 0 — Preparando o terreno** (instalar Go, a toolchain, primeiro programa)
- **Parte 1 — Fundamentos de Go** (a linguagem inteira que o ray usa)
- **Parte 2 — Construindo o ray** (fase a fase, com Go aplicado e explicado)
- **Parte 3 — Testes em Go** (o que torna o ray confiável)
- **Parte 4 — Empacotar e distribuir**
- **Apêndice — referência rápida**

---

# Parte 0 — Preparando o terreno

## 0.1 Por que Go para uma CLI?

Go foi feita no Google para resolver dores de software de larga escala, mas
acabou sendo **excelente para CLIs**:

- **Compila para um binário único e estático.** Sem runtime, sem `node_modules`,
  sem "instale o Python certo". Você gera um arquivo `ray` e ele roda.
- **Cross-compilation trivial.** Da sua máquina Linux você gera o binário pra
  macOS/Windows mudando duas variáveis de ambiente.
- **Biblioteca-padrão forte** para o que uma CLI faz: `os`, `os/exec`, `io`,
  `path/filepath`, `encoding/json`, `flag`, `testing`.
- **Rápida de compilar e simples de ler.** Poucas construções, pouca "mágica".
- **Concorrência embutida** (goroutines), que mal usaremos aqui, mas é parte do
  motivo de Go ser popular.

## 0.2 Instalando o Go

Baixe em <https://go.dev/dl> (ou use o gerenciador da sua distro, mas a versão
oficial costuma ser mais nova). Confirme:

```sh
go version
# go version go1.25.x linux/amd64
```

Variáveis úteis (veja com `go env`):
- `GOPATH` (default `~/go`): onde ficam pacotes baixados e binários instalados
  (`~/go/bin` — coloque no seu `PATH`).
- `GOMODCACHE`, `GOCACHE`: caches de módulos e de build.

Você **não** precisa colocar o projeto dentro de `GOPATH` — isso era exigência
antiga. Com **módulos** (desde Go 1.11), o projeto vive em qualquer pasta.

## 0.3 A toolchain — os comandos que você vai usar o tempo todo

| Comando | O que faz |
|---|---|
| `go run .` | compila e roda o pacote atual (sem deixar binário) |
| `go build ./...` | compila tudo; `./...` = "este diretório e todos abaixo" |
| `go test ./...` | roda todos os testes |
| `go vet ./...` | análise estática: pega bugs comuns que compilam mas estão errados |
| `gofmt -l .` / `go fmt ./...` | formatação canônica (Go tem **um** estilo, imposto por ferramenta) |
| `go mod init <path>` | cria o `go.mod` (inicia um módulo) |
| `go get <pkg>` | adiciona/atualiza uma dependência |
| `go mod tidy` | adiciona o que falta e remove o que sobra no `go.mod`/`go.sum` |
| `go install <pkg>@latest` | compila e instala um binário em `~/go/bin` |
| `go doc <pkg>` | documentação de um pacote no terminal |

> **`gofmt` não é opcional culturalmente.** Em Go não se discute estilo: rode o
> formatador. Configure seu editor pra rodar `gofmt`/`goimports` ao salvar.

## 0.4 Seu primeiro programa

```go
// arquivo: hello.go
package main

import "fmt"

func main() {
	fmt.Println("olá, ray")
}
```

```sh
go run hello.go
```

Três coisas já aparecem e valem para tudo em Go:

1. **Todo arquivo pertence a um `package`.** O pacote `main` com a função `main()`
   é o que vira um **executável**. Qualquer outro nome de pacote vira uma
   **biblioteca**.
2. **`import`** traz outro pacote. `fmt` ("format") é a stdlib de I/O formatado.
3. **`func main()`** é o ponto de entrada do binário.

## 0.5 Módulos e a estrutura do projeto

Um **módulo** é uma unidade versionável de código, identificada por um _module
path_ (uma URL, por convenção). Crie o do ray:

```sh
mkdir ray && cd ray
git init
go mod init github.com/murilopmr/ray
```

Isso gera `go.mod`:

```
module github.com/murilopmr/ray

go 1.25
```

O _module path_ é o **prefixo de import** de todo pacote interno. O pacote em
`internal/runner/` será importado como `github.com/murilopmr/ray/internal/runner`.

### A pasta `internal/`

Go tem uma regra especial: **pacotes dentro de uma pasta `internal/` só podem ser
importados por código que compartilha o pai de `internal/`**. Ou seja, ninguém
fora do seu módulo consegue importar `…/internal/runner`. É o jeito de Go dizer
"isto é privado do projeto". Por isso todo o código do ray vive em `internal/` —
ele é uma aplicação, não uma biblioteca pública.

### Exportado vs não-exportado: a regra da maiúscula

Em Go **não existe `public`/`private`**. A visibilidade é decidida pela **primeira
letra do identificador**:

- `Começa com Maiúscula` → **exportado** (visível fora do pacote).
- `começa com minúscula` → **não-exportado** (privado do pacote).

Vale para tipos, funções, campos de struct, métodos, constantes. Então:

```go
type Profile struct {   // Profile é usável por outros pacotes
	Name string          // Name é visível
	internalCache map[string]any // invisível fora do pacote profile
}
```

Você vai _projetar_ a API de cada pacote escolhendo o que merece maiúscula.

---

# Parte 1 — Fundamentos de Go

Esta parte ensina **toda** a linguagem que o ray usa. Os exemplos já são, sempre
que possível, trechos reais que você vai escrever depois.

## 1.1 Variáveis, tipos e o zero value

```go
var name string = "go"   // forma longa
var count int            // sem inicializar → recebe o ZERO VALUE
name := "go"             // forma curta (só dentro de função): tipo inferido
```

**Não existe `null`/`undefined` perigoso.** Toda variável tem um **zero value**
bem definido:

| Tipo | Zero value |
|---|---|
| números (`int`, `float64`) | `0` |
| `string` | `""` (vazia) |
| `bool` | `false` |
| ponteiros, slices, maps, funções, interfaces, channels | `nil` |
| struct | struct com cada campo no seu zero value |

Isso molda o design idiomático: um `Config{}` recém-criado já é utilizável, com
campos vazios. No ray, `LoadState()` devolve um `State{}` vazio quando o arquivo
não existe — e isso "simplesmente funciona".

> **`:=` só funciona dentro de funções.** No nível de pacote, use `var`.

### Tipos básicos que importam aqui
`string`, `bool`, `int`, `int64`, `byte` (= `uint8`), `rune` (= `int32`, um ponto
Unicode), `float64`, e `[]byte` (fatia de bytes — onipresente em I/O).

## 1.2 Constantes e `iota`

```go
const FileName = "config.yaml"   // tipo inferido

const (
	ViaSkills = "skills"
	ViaAitmpl = "aitmpl"
)
```

`iota` gera sequências (enum-like), útil mas que o ray usa pouco — preferimos
constantes de string nomeadas (`ViaSkills`) porque elas viram exatamente o valor
no YAML.

## 1.3 Funções e múltiplos retornos

```go
func add(a, b int) int { return a + b }
```

O recurso mais marcante: **múltiplos valores de retorno**. É como Go faz
tratamento de erro (sem exceptions):

```go
func Home() (string, error) {
	if h := os.Getenv("RAY_HOME"); h != "" {
		return h, nil           // sucesso: valor + nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err          // falha: zero value + erro
	}
	return filepath.Join(home, ".ray"), nil
}
```

Padrão universal: `valor, err := f()`, depois `if err != nil { ... }`. Você vai
escrever isso milhares de vezes; é a espinha dorsal de Go.

**Retornos nomeados** (usados com parcimônia):

```go
func sub(name string) (path string, err error) {
	home, err := Home()
	if err != nil {
		return // devolve path="" e o err atual
	}
	return filepath.Join(home, name), nil
}
```

**Variádicas** (`...`): `func Println(a ...any)`. No ray, `runner.Command` guarda
`Args []string` e você "espalha" com `args...`.

## 1.4 Structs

A struct é o tipo composto central de Go (não há classes):

```go
type Command struct {
	Name string
	Args []string
	Dir  string
}

c := Command{Name: "npx", Args: []string{"-y", "skills"}, Dir: "/tmp"}
fmt.Println(c.Name)   // acesso por ponto
```

Inicialize **com nomes de campo** (`Command{Name: ...}`) — é legível e robusto a
reordenação. O `Command{}` vazio é válido (todos os campos no zero value).

## 1.5 Slices — a "lista" de Go

Slices são o tipo de coleção que você mais usa. Um slice é uma _view_ sobre um
array: tem **comprimento** (`len`) e **capacidade** (`cap`).

```go
var xs []string          // nil slice, len 0 — pode dar append à vontade
xs = append(xs, "a")     // append devolve um NOVO slice; reatribua sempre
ys := []int{1, 2, 3}     // literal
fmt.Println(len(ys))     // 3
sub := ys[1:3]           // [2 3] — fatiamento [início:fim) (fim exclusivo)
```

**Pegadinha que custou caro no ray:** `append` pode realocar o array de fundo, ou
não. Não confie em identidade. Sempre faça `xs = append(xs, v)`.

`range` itera:

```go
for i, v := range ys {       // índice, valor
	fmt.Println(i, v)
}
for _, v := range ys {       // `_` descarta o índice
	use(v)
}
```

> **`_` é o "blank identifier".** Descarta valores que você é obrigado a receber
> mas não quer. Aparece muito em `for _, v := range` e `v, _ := f()`.

## 1.6 Maps — dicionários

```go
m := map[string]bool{}        // map vazio (pronto pra uso)
m["headroom"] = true
v := m["ausente"]             // devolve o ZERO VALUE (false), sem erro
v, ok := m["headroom"]       // "comma-ok": ok==true se a chave existe
delete(m, "headroom")
for k, val := range m { ... } // ordem ALEATÓRIA — nunca confie na ordem
```

O idioma `v, ok := m[k]` é como você distingue "existe e é false" de "não
existe". No ray, o merge de `.mcp.json` faz exatamente isso.

> **Maps não têm ordem.** Se precisar de saída ordenada (ex.: `profile list`),
> colete as chaves num slice e use `sort.Slice`.

## 1.7 Ponteiros (sem pânico)

Um ponteiro guarda o **endereço** de um valor. `&x` pega o endereço; `*p` o
desreferencia.

```go
func bump(n *int) { *n = *n + 1 }   // recebe ponteiro → altera o original

x := 10
bump(&x)
fmt.Println(x) // 11
```

Por que importam:
1. **Mutação.** Go passa argumentos **por cópia**. Pra um função alterar seu
   valor (ou pra evitar copiar uma struct grande), passe um ponteiro.
2. **Opcionalidade.** Um `*Config` pode ser `nil` ("não há config"), enquanto um
   `Config` sempre existe.

No ray, métodos que **alteram** o State usam receiver ponteiro: `func (s *State)
AddGlobal(...)`. E `Load()` devolve `*Config` pra poder ser `nil` em erro.

> Você **não** gerencia memória manualmente — Go tem garbage collector. Ponteiro
> aqui é sobre _semântica_ (compartilhar vs copiar), não sobre `malloc/free`.

## 1.8 Métodos e receivers

Método é uma função "presa" a um tipo, via **receiver**:

```go
type State struct{ InstalledGlobals []string }

// receiver por VALOR: opera numa cópia (bom para leitura)
func (s State) HasGlobal(key string) bool {
	for _, k := range s.InstalledGlobals {
		if k == key { return true }
	}
	return false
}

// receiver por PONTEIRO: pode mutar o original
func (s *State) AddGlobal(key string) {
	if !s.HasGlobal(key) {
		s.InstalledGlobals = append(s.InstalledGlobals, key)
	}
}
```

**Regra prática:** se o método **modifica** o receiver, ou se a struct é grande,
use ponteiro (`*T`). Se só lê e é pequena, valor serve. Na prática, escolha **um**
estilo por tipo e seja consistente (Go te deixa chamar `(&s).AddGlobal()`
automaticamente como `s.AddGlobal()`).

## 1.9 Interfaces — o coração do design do ray

Uma interface é um **contrato**: um conjunto de assinaturas de método. Qualquer
tipo que tenha esses métodos **satisfaz a interface automaticamente** — não há
`implements`, é estrutural ("duck typing" verificado em compilação).

```go
// O contrato: "algo que sabe rodar um comando".
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}
```

Agora dois tipos diferentes podem satisfazê-lo:

```go
type ExecRunner struct{ DryRun bool; Out io.Writer }
func (r ExecRunner) Run(ctx context.Context, c Command) (Result, error) {
	// ... chama os/exec de verdade
}

type FakeRunner struct{ Calls []Command }
func (f *FakeRunner) Run(ctx context.Context, c Command) (Result, error) {
	f.Calls = append(f.Calls, c)   // só grava, não executa nada
	return Result{ExitCode: 0}, nil
}
```

**É isso que torna o ray testável.** O código de produção pede um `Runner`; em
produção você injeta o `ExecRunner` (toca o sistema), e nos testes injeta o
`FakeRunner` (grava e devolve respostas programadas). Nenhum teste chama `npx` de
verdade. Essa é a fronteira mais importante do projeto.

Interfaces famosas da stdlib que você vai usar:
- `error` — um único método `Error() string`.
- `io.Writer` — `Write([]byte) (int, error)`. `os.Stdout`, um `bytes.Buffer` e um
  arquivo são todos `io.Writer`. Por isso funções do ray recebem `out io.Writer`:
  em produção passamos `os.Stdout`, no teste um buffer pra inspecionar a saída.
- `io.Reader` — o dual de `Writer`.

A interface vazia `interface{}`, hoje escrita **`any`**, casa com qualquer valor —
útil pra dados dinâmicos (ex.: JSON/`settings.json`: `map[string]any`). Use com
parcimônia; perde checagem de tipo.

### Type assertion e type switch

Quando você tem um `any` e quer o tipo concreto:

```go
v := root["mcpServers"]
servers, ok := v.(map[string]any)   // type assertion com comma-ok
if !ok { servers = map[string]any{} }
```

```go
switch x := v.(type) {        // type switch
case string:  useString(x)
case int:     useInt(x)
default:      // ...
}
```

## 1.10 Erros — a forma idiomática

`error` é só uma interface:

```go
type error interface { Error() string }
```

Criar erros:

```go
errors.New("name is required")
fmt.Errorf("invalid profile %s: %w", path, err) // %w "embrulha" o erro original
```

O verbo **`%w`** preserva a cadeia de causas, e você inspeciona com:

```go
errors.Is(err, os.ErrNotExist)   // é (ou embrulha) este erro-sentinela?
var perr *fs.PathError
errors.As(err, &perr)            // extrai um tipo concreto da cadeia
```

No ray, `LoadState` faz o padrão clássico de "arquivo opcional":

```go
data, err := os.ReadFile(p)
if err == nil {
	// existe: parseia
} else if !os.IsNotExist(err) {
	return nil, err   // erro real (permissão, etc.) → propaga
}
// não existe → segue com o zero value
```

**Filosofia:** erros são **valores**, tratados explicitamente onde acontecem. Não
há `try/catch`. Isso deixa o fluxo visível e força você a decidir o que fazer em
cada falha. Em CLI isso é ótimo: cada erro vira uma mensagem clara e um exit code.

## 1.11 `defer`, `panic`, `recover`

`defer` agenda uma chamada pra rodar quando a função **retornar** — ideal pra
limpeza (fechar arquivo, remover temp):

```go
f, err := os.Open(path)
if err != nil { return err }
defer f.Close()   // roda no fim, aconteça o que acontecer
```

`panic` aborta o programa (use só pra "isto nunca deveria acontecer"); `recover`
captura um panic dentro de um `defer`. Em CLI bem-feita você **quase nunca** usa
panic — devolve `error`.

## 1.12 Controle de fluxo

```go
if x > 0 { ... } else if x < 0 { ... } else { ... }

// if com inicializador (escopo curto): muito idiomático
if err := do(); err != nil { return err }

// for é o ÚNICO loop de Go (faz o papel de while também)
for i := 0; i < 3; i++ { ... }
for cond { ... }          // "while"
for { break }             // loop infinito

switch mode {
case "build": ...
case "learn": ...
default: ...
}
```

Em Go, `switch` **não** "cai" pro próximo case (sem `break` explícito). E o `if`
com inicializador (`if v, ok := m[k]; ok {…}`) mantém variáveis no escopo certo.

## 1.13 Generics (rápido)

Desde Go 1.18 há genéricos com _type parameters_:

```go
func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs { out[i] = f(x) }
	return out
}
```

O ray usa pouquíssimo genérico — a stdlib (`slices`, `maps`) já cobre o básico.
Bom saber que existe; não force.

## 1.14 Struct tags + (de)serialização

Tags são metadados em string nos campos, lidos por reflexão. É como `yaml.v3` e
`encoding/json` sabem o nome no arquivo:

```go
type Integrations struct {
	Headroom       bool `yaml:"headroom"`
	KnowledgeVault bool `yaml:"knowledge_vault"`
}
```

`yaml.Unmarshal(data, &v)` preenche a struct a partir do YAML; `yaml.Marshal(v)`
faz o caminho inverso. O `&v` (ponteiro) é obrigatório pra função poder
**escrever** na sua variável.

## 1.15 `go:embed` — embutir arquivos no binário

O ray embute os templates de scaffold **dentro** do executável, pra não depender
de arquivos soltos:

```go
import "embed"

//go:embed templates/*.tmpl
var embedded embed.FS   // um filesystem só-leitura dentro do binário
```

O comentário `//go:embed` (sem espaço depois de `//`) é uma **diretiva de
compilador**. `embed.FS` implementa `fs.FS`, então você lê com
`embedded.ReadFile("templates/x.tmpl")`. É o que permite distribuir um `ray`
único e ainda ter os templates default.

## 1.16 `context.Context` (o suficiente)

`context.Context` carrega cancelamento/prazo através de chamadas. O ray o passa
adiante (`Run(ctx, ...)`) pra que um Ctrl-C ou timeout possa interromper um
processo externo. Você raramente o cria à mão na CLI — o Cobra te dá um via
`cmd.Context()`, e você só repassa.

## 1.17 Goroutines e channels (você quase não vai usar)

`go f()` roda `f` concorrentemente; channels (`ch := make(chan int)`) comunicam
entre goroutines. O ray é essencialmente sequencial (instala uma coisa após a
outra), então isto fica como cultura geral, não como necessidade. Saiba que
existe; não precisa aqui.

---

# Parte 2 — Construindo o ray

Agora aplicamos tudo. A construção é dividida em **6 milestones**; cada um
entrega algo observável. Dentro de cada fase: **o que**, **o código** (com a
explicação dos conceitos novos) e **os testes**. Commit ao fim de cada passo,
sempre com `go build ./...` e `go test ./...` verdes, **sem** co-autor de IA.

```
M0 Esqueleto que anda      Fase 0
M1 Domínio puro (sem IO)   Fases 1–4  (runner, profile, installer, merges)
M2 Efeitos em disco        Fases 5–6  (vault, scaffold/templates)
M3 Primeiro comando útil   Fase 7     (preflight + doctor)
M4 Feature-âncora          Fase 8     (config/state/paths + initai + init ai)
M5 Gestão + criação        Fases 9–10 (profile/vault/docs, run, new + .gitignore)
M6 Acabamento              Fases 11–12 (update, Makefile/CI, README, validação)
```

---

## M0 — Esqueleto que anda

### Fase 0 · Bootstrap e a árvore de comandos (Cobra)

**Conceitos novos:** módulo, pacote `main`, dependências, Cobra.

Inicialize e adicione o Cobra:

```sh
go mod init github.com/murilopmr/ray
go get github.com/spf13/cobra@latest
```

`main.go` — o entrypoint mínimo (delega tudo ao pacote `cmd`):

```go
package main

import "github.com/murilopmr/ray/internal/cmd"

func main() {
	cmd.Execute()
}
```

`internal/cmd/root.go` — o comando raiz e as flags globais. **Cobra** modela uma
CLI como uma árvore de `*cobra.Command`; cada comando tem `Use`, `Short` e um
`RunE` (a função que roda, com erro). Flags **persistentes** valem pro comando e
todos os filhos.

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Flags globais, compartilhadas pelos subcomandos.
var (
	flagVerbose bool
	flagDryRun  bool
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ray",
		Short:         "Personal CLI for bootstrapping projects and AI dev environments",
		SilenceUsage:  true, // não despejar o help a cada erro
		SilenceErrors: true, // nós mesmos imprimimos o erro (em Execute)
	}
	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "verbose output")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "print actions without doing them")

	// Monta a árvore inteira agora (stubs viram comandos reais nas próximas fases).
	root.AddCommand(newInitCmd(), newNewCmd(), newRunCmd(),
		newProfileCmd(), newVaultCmd(), newDocsCmd(), newDoctorCmd())
	return root
}

// Execute roda a CLI e traduz erro em exit code.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

Repare nas escolhas idiomáticas: `SilenceUsage/SilenceErrors` (best practice atual
do Cobra) pra **nós** controlarmos a saída de erro; `os.Exit(1)` só no topo (nunca
no meio — `os.Exit` não roda `defer`s).

Um stub de filho (todos iguais por ora). `init.go` é um comando-**pai** que só
agrupa:

```go
package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "init", Short: "Initialization commands"}
	cmd.AddCommand(newInitAICmd())
	return cmd
}

func newInitAICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ai",
		Short: "Set up the AI development environment in a folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help() // por enquanto
		},
	}
}
```

Crie stubs parecidos para `new`, `run`, `profile`, `vault`, `docs`, `doctor`.

**Pronto quando:** `go build ./...` ok; `ray --help` lista os comandos; `ray init
--help` mostra `ai`. **Commit:** `chore: bootstrap ray CLI skeleton (cobra + command tree)`.

> **Por que "factory functions" (`newRootCmd()`)** em vez de variáveis globais
> `var rootCmd = ...`? Porque variáveis globais com estado dificultam teste
> (vazam entre testes) e criam ordem de init implícita. Funções que **constroem**
> um comando novo a cada chamada são testáveis e explícitas.

---

## M1 — Domínio puro (sem tocar disco de projeto nem rede)

Aqui mora o "cérebro" do ray. Tudo testável só com memória.

### Fase 1 · `runner` — a fronteira de execução

**Conceitos novos:** interface como contrato, `os/exec`, injeção de dependência,
o `FakeRunner`.

`internal/runner/runner.go`:

```go
// Package runner é a única fronteira do ray com processos externos.
package runner

import (
	"context"
	"os/exec"
	"strings"
)

// Command é um processo a executar. O Command{} vazio é inválido (Name vazio),
// mas Args/Dir são opcionais.
type Command struct {
	Name string
	Args []string
	Dir  string // diretório de trabalho ("" = atual)
}

// String dá uma forma legível pra logs e mensagens de erro.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Result é o que sobrou de rodar um Command.
type Result struct {
	Stdout, Stderr string
	ExitCode       int
}

// Runner é o contrato: "sei executar um Command". Tudo no ray depende DESTA
// interface, nunca de exec direto — é o que permite o FakeRunner nos testes.
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}
```

A implementação real, com `os/exec`:

```go
import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// ExecRunner roda comandos de verdade. Com DryRun, só imprime.
type ExecRunner struct {
	DryRun bool
	Out    io.Writer
}

func (r ExecRunner) Run(ctx context.Context, c Command) (Result, error) {
	if r.DryRun {
		if r.Out != nil {
			fmt.Fprintln(r.Out, "+ "+c.String())
		}
		return Result{ExitCode: 0}, nil
	}

	cmd := exec.CommandContext(ctx, c.Name, c.Args...) // ctx permite cancelar
	cmd.Dir = c.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	// Um exit ≠ 0 NÃO é erro de Go — é resultado. Distinguimos os dois:
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil // rodou, mas o programa falhou: ExitCode conta a história
	}
	if err != nil {
		return res, err // não conseguiu nem rodar (binário ausente, etc.)
	}
	return res, nil
}
```

Aqui aparecem juntos: `exec.CommandContext`, `bytes.Buffer` como `io.Writer`,
`errors.As` pra separar "o programa rodou e saiu ≠ 0" de "nem rodou". Essa
distinção é o que deixa o ray reportar `(exit 1)` vs `(comando não encontrado)`.

`internal/runner/fake.go`:

```go
package runner

import "context"

// FakeRunner grava chamadas e devolve respostas programadas. Receiver PONTEIRO
// porque cada Run muta Calls.
type FakeRunner struct {
	Calls   []Command
	Results map[string]Result // por Command.String(); ausente = sucesso
	Err     error             // se setado, todo Run devolve este erro
}

func (f *FakeRunner) Run(_ context.Context, c Command) (Result, error) {
	f.Calls = append(f.Calls, c)
	if f.Err != nil {
		return Result{}, f.Err
	}
	if r, ok := f.Results[c.String()]; ok {
		return r, nil
	}
	return Result{ExitCode: 0}, nil
}
```

**Teste** (`runner_test.go`) — note `t.TempDir`, comando real inofensivo:

```go
package runner

import (
	"context"
	"testing"
)

func TestFakeRecordsCalls(t *testing.T) {
	f := &FakeRunner{}
	_, _ = f.Run(context.Background(), Command{Name: "npx", Args: []string{"x"}})
	if len(f.Calls) != 1 || f.Calls[0].Name != "npx" {
		t.Fatalf("não gravou a chamada: %+v", f.Calls)
	}
}

func TestExecRunnerEcho(t *testing.T) {
	r := ExecRunner{}
	res, err := r.Run(context.Background(), Command{Name: "echo", Args: []string{"hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Stdout != "hi\n" {
		t.Fatalf("inesperado: %+v", res)
	}
}
```

Convenções de teste que valem pra tudo: arquivo termina em `_test.go`; função
`TestXxx(t *testing.T)`; `t.Fatalf` aborta o teste com mensagem. **Commit:**
`feat(runner): command execution boundary (exec + dry-run + fake)`.

### Fase 2 · `profile` — modelo, validação e defaults

**Conceitos novos:** structs com tags YAML, métodos de validação, `os`/`filepath`,
escrever defaults idempotentemente.

Comece pelos `raypaths` (curto e necessário): use o código da §4.4 do guia —
`Home()` lê `RAY_HOME` ou `~/.ray`; `ProfilesDir/TemplatesDir/VaultDir`.

`internal/profile/profile.go` — as structs (veja guia §4.1) e a validação. O
ponto pedagógico é o **método `Validate`** retornando erro no primeiro problema:

```go
func (p *Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for i, c := range p.Components {
		if err := c.validate(); err != nil {
			return fmt.Errorf("component %d: %w", i, err) // %w preserva a causa
		}
	}
	return nil
}

func (c Component) validate() error {
	switch c.Via {
	case ViaSkills:
		if c.Skill == "" || c.Source == "" {
			return fmt.Errorf("via skills requires both 'skill' and 'source'")
		}
	case ViaAitmpl:
		switch c.Type {
		case TypeAgent, TypeCommand, TypeMCP:
		default:
			return fmt.Errorf("via aitmpl requires type agent|command|mcp, got %q", c.Type)
		}
		if c.Ref == "" {
			return fmt.Errorf("via aitmpl requires 'ref'")
		}
	case "":
		return fmt.Errorf("'via' is required")
	default:
		return fmt.Errorf("unknown 'via' %q", c.Via)
	}
	return nil
}
```

`Load` é o casamento de tudo: ler arquivo → `Unmarshal` → `Validate`:

```go
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile %s: %w", path, err)
	}
	return &p, nil
}
```

`EnsureDir` mostra o **padrão da idempotência** (só escreve o que falta — nunca
sobrescreve edições do usuário):

```go
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range Defaults() {
		path := filepath.Join(dir, p.Name+".yaml")
		if _, err := os.Stat(path); err == nil {
			continue // já existe: respeita
		}
		data, err := yaml.Marshal(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
```

`internal/profile/defaults.go` traduz o Apêndice A (guia §12) em código — note os
**helpers** (`skill()`, `agent()`, `baseComponents()`) que reduzem repetição. Use
o conteúdo do guia.

**Testes** (table-driven — veja Parte 3): load válido vira a struct esperada;
cada caso inválido (via desconhecido, campo faltando) retorna erro; `EnsureDir`
em `t.TempDir()` cria defaults e não sobrescreve na 2ª chamada. **Commit:**
`feat(profile): recipe model, validation and built-in defaults`.

### Fase 3 · `installer` — receita vira um `Plan` (dados, não execução)

**Conceito-chave:** **separar decisão de efeito**. O installer **não executa
nada**; ele transforma uma receita numa estrutura de dados (`Plan`) que descreve o
que fazer. Quem executa é o `runner` (Fase 8). Isso torna o mapeamento
"receita→comando" testável por igualdade de strings, sem rede.

Defina já o tipo `mcp.Server` (mesmo que o _writer_ venha na Fase 4) — em Go a
ordem de definição entre pacotes não importa, mas você precisa do tipo pra
compilar:

```go
// internal/mcp/mcp.go (só o tipo por enquanto)
package mcp
type Server struct {
	Name, Command string
	Args          []string
	Env           map[string]string
}
```

`internal/installer/installer.go` — `Resolve` percorre a receita e acumula três
listas (a explicação está no guia §6):

```go
func Resolve(p *profile.Profile, opts Options) (Plan, error) {
	var plan Plan
	for _, c := range p.Components {
		cmd, err := componentCommand(c, opts)
		if err != nil {
			return Plan{}, err
		}
		plan.Commands = append(plan.Commands, cmd)
	}
	if p.Integrations.Headroom {
		plan.Globals = append(plan.Globals, GlobalStep{Key: "headroom",
			Commands: []runner.Command{headroomInstall()}})
		plan.Servers = append(plan.Servers, headroomServer())
	}
	// ... knowledge_vault, user_docs_vault, second_brain, obsidian_formats, code_graph
	return plan, nil
}

func componentCommand(c profile.Component, opts Options) (runner.Command, error) {
	switch c.Via {
	case profile.ViaSkills:
		args := []string{"skills", "add", c.Source, "--skill", c.Skill, "-a", "claude-code", "-y"}
		if opts.Global {
			args = append(args, "-g")
		}
		return runner.Command{Name: "npx", Args: args}, nil
	case profile.ViaAitmpl:
		flag, err := aitmplFlag(c.Type)
		if err != nil {
			return runner.Command{}, err
		}
		return runner.Command{Name: "npx", Args: []string{
			"claude-code-templates@latest", fmt.Sprintf("%s=%s", flag, c.Ref), "--yes"}}, nil
	default:
		return runner.Command{}, fmt.Errorf("unknown via %q", c.Via)
	}
}
```

`integrations.go` guarda os comandos concretos (use o guia §6 **ao pé da letra** —
`graphifyy`, `headroom-ai[mcp]`, `--platform claude`, server `graphify-mcp`).

**Teste** (table-driven, sem rede) — cada linha da tabela §6 vira um caso:

```go
func TestComponentCommand(t *testing.T) {
	cases := []struct {
		name string
		comp profile.Component
		opts Options
		want string
	}{
		{"skill local", profile.Component{Via: "skills", Source: "o/r", Skill: "s"},
			Options{}, "npx skills add o/r --skill s -a claude-code -y"},
		{"skill global", profile.Component{Via: "skills", Source: "o/r", Skill: "s"},
			Options{Global: true}, "npx skills add o/r --skill s -a claude-code -y -g"},
		{"agent", profile.Component{Via: "aitmpl", Type: "agent", Ref: "x/y"},
			Options{}, "npx claude-code-templates@latest --agent=x/y --yes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := componentCommand(tc.comp, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tc.want {
				t.Errorf("got %q want %q", got.String(), tc.want)
			}
		})
	}
}
```

**Commit:** `feat(installer): resolve recipes into a deterministic install plan`.

### Fase 4 · `mcp` + `claudecfg` — merges idempotentes

**Conceitos novos:** `encoding/json`, `map[string]any` para JSON dinâmico,
escrever sem destruir.

O desafio: o ray não é dono do `.mcp.json` — pode haver servers de terceiros lá.
Então **leia, mescle, reescreva**, preservando o resto. `map[string]any` modela
JSON arbitrário; `any` (a interface vazia) recebe qualquer valor.

```go
func WriteServers(targetDir string, servers []Server, dryRun bool, out io.Writer) error {
	if len(servers) == 0 {
		return nil
	}
	path := filepath.Join(targetDir, FileName)

	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing existing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	mcpServers, _ := root["mcpServers"].(map[string]any) // type assertion
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	for _, s := range servers {
		entry := map[string]any{"command": s.Command}
		if len(s.Args) > 0 {
			entry["args"] = s.Args
		}
		mcpServers[s.Name] = entry // por nome: idempotente (substitui, não duplica)
	}
	root["mcpServers"] = mcpServers

	pretty, _ := json.MarshalIndent(root, "", "  ")
	pretty = append(pretty, '\n')
	if dryRun {
		fmt.Fprintf(out, "# %s\n%s", path, pretty)
		return nil
	}
	return os.WriteFile(path, pretty, 0o644)
}
```

`claudecfg.MergeSettings` é o mesmo padrão pra `.claude/settings.json` (mescla
`model`, `effortLevel`, bloco `hooks`, preservando chaves). **Teste-chave:**
aplicar duas vezes não muda o arquivo (idempotência) e chaves alheias sobrevivem.
**Commit:** `feat(mcp,claudecfg): idempotent merges for .mcp.json and settings.json`.

> **Fim do M1.** Todo o núcleo decisório do ray existe e é testado sem tocar
> projeto real nem rede. O resto é efeito colateral e fiação.

---

## M2 — Efeitos em disco

### Fase 5 · `vault`

Curto: `Ensure(path)` cria `inbox/`, `notes/`, `README.md`, `.obsidian/` opcional
com `os.MkdirAll`/`os.WriteFile`, **idempotente** (não sobrescreve). `Status(path)`
conta `.md` com `filepath.WalkDir`. Teste em `t.TempDir()`: cria, 2ª chamada não
mexe, `Status` conta certo. **Commit:** `feat(vault): create and inspect the AI knowledge vault`.

### Fase 6 · `scaffold` — templates embutidos, render e modos

**Conceitos novos:** `text/template`, `go:embed`, FileMode (permissões), overlay.

Os templates ficam em `internal/scaffold/templates/*.tmpl` e são **embutidos**:

```go
//go:embed templates/*.tmpl
var embedded embed.FS
```

`text/template` substitui `{{.ProjectName}}` / `{{.Stack}}`. A opção
`missingkey=zero` evita explodir se faltar uma chave:

```go
func render(tmplName, overlayDir string, data Data) (string, error) {
	var raw []byte
	loaded := false
	if overlayDir != "" { // o usuário pode sobrepor um template em ~/.ray/templates
		if b, err := os.ReadFile(filepath.Join(overlayDir, tmplName)); err == nil {
			raw, loaded = b, true
		}
	}
	if !loaded {
		b, err := embedded.ReadFile("templates/" + tmplName)
		if err != nil {
			return "", fmt.Errorf("template %q not found: %w", tmplName, err)
		}
		raw = b
	}
	t, err := template.New(tmplName).Option("missingkey=zero").Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

A **não-sobrescrita** e a regra do handoff (nunca tocado, nem com `--force`):

```go
exists := false
if _, err := os.Stat(target); err == nil {
	exists = true
}
if exists && (f.Path == liveHandoff || !opts.Force) {
	return Result{Path: f.Path, Status: "skipped"}, nil
}
// arquivos .sh saem executáveis:
mode := os.FileMode(0o644)
if strings.HasSuffix(f.Path, ".sh") {
	mode = 0o755
}
```

`mode.go` traz `ValidMode`, `SystemFiles(mode)` (arquivos que o ray **sempre**
escreve — garantem que todo hook citado no settings exista) e `HookSettings(mode)`
(bloco a mesclar no settings). Use o guia §7. **Teste de ouro:** o `guard-code.sh`
libera `docs/x.md` e bloqueia `lib/main.dart` (teste de comportamento do script).
**Commit:** `feat(scaffold): layered orientation files, templates and modes`.

---

## M3 — Primeiro comando útil

### Fase 7 · `preflight` + `ray doctor`

**Conceitos novos:** `exec.LookPath`, injetar uma função (`Looker`) pra testar,
montar uma struct de dados que dirige comportamento (`Check.Fix`).

`Looker` é uma **função como valor** (Go trata funções como first-class), injetada
pra testar sem depender do PATH real:

```go
type Looker func(name string) (string, error)
var DefaultLooker Looker = exec.LookPath

type Check struct {
	Name     string
	Found    bool
	Required bool
	Hint     string
	Fix      []runner.Command // vazio = sem auto-fix
}

func Run(look Looker, needPython bool) []Check {
	if look == nil {
		look = DefaultLooker
	}
	found := func(name string) bool { _, err := look(name); return err == nil }
	checks := []Check{
		{Name: "npx", Found: found("npx"), Required: true, Hint: "install Node.js"},
		// ... python/uv condicionais a needPython; headroom/graphify informativos com Fix
	}
	return checks
}
```

O comando `doctor` é o primeiro que **faz** algo visível pro usuário: roda os
checks, imprime ✓/✗, e com `--fix` executa os `Check.Fix` via `ExecRunner`. Veja
guia §10. Aqui você liga a primeira flag local de comando:

```go
func newDoctorCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check runtime dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout() // io.Writer — testável (em produção é stdout)
			checks := preflight.Run(preflight.DefaultLooker, true)
			printChecks(out, checks)
			if fix {
				// roda os Fix e re-checa...
			}
			if missing := preflight.MissingRequired(checks); len(missing) > 0 {
				return fmt.Errorf("missing required dependencies: %v", missing)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "auto-install what ray can")
	return cmd
}
```

Note `cmd.OutOrStdout()`: escrever nele (em vez de `fmt.Println`) deixa o teste
capturar a saída num buffer. **Commit:** `feat(doctor): runtime dependency checks with --fix`.

---

## M4 — A feature-âncora

### Fase 8 · config/state/paths + `initai` + `ray init ai`

Esta é a maior fase. Ela **costura** tudo: carrega a receita, resolve o plano,
roda via runner, escreve MCP/settings, faz o scaffold, e devolve um resumo.

Primeiro a persistência (use o guia §4.2): `rayconfig.Config` (`~/.ray/config.yaml`)
e `rayconfig.State` (`~/.ray/state.yaml`). São o mesmo padrão "load opcional +
save" que você já viu, agora com métodos (`HasGlobal/AddGlobal`).

`initai.Run` segue os **10 passos** do guia §8. O coração é a separação entre
"globais install-once" e "componentes por-projeto", e o **acumulador de
resultados**:

```go
type Summary struct {
	Mode, Target string
	Installed, Failed, Created, Skipped, Warnings []string
	HadFailure   bool
}

// runOne classifica o resultado de UM comando e o registra no Summary.
func runOne(ctx context.Context, d Deps, c runner.Command, sum *Summary) bool {
	res, err := d.Runner.Run(ctx, c)
	switch {
	case err != nil:
		sum.Failed = append(sum.Failed, c.String()+" ("+err.Error()+")")
		sum.HadFailure = true
		return false
	case res.ExitCode != 0:
		sum.Failed = append(sum.Failed, fmt.Sprintf("%s (exit %d)", c.String(), res.ExitCode))
		sum.HadFailure = true
		return false
	default:
		sum.Installed = append(sum.Installed, c.String())
		return true
	}
}
```

Repare: **a falha de um componente não aborta o resto** (registra e segue), mas as
pré-checagens **sim** abortam. `Run` devolve o `Summary`; o comando imprime e sai
≠ 0 se `HadFailure`. A injeção de dependências (`Deps{Runner, Looker, Out}`) é o
que permite o teste de fumaça ponta-a-ponta com `FakeRunner` num `t.TempDir()`,
com `t.Setenv("RAY_HOME", tmp)`.

`cmd/init_ai.go` declara as flags (guia §5), monta `Params`/`Deps` com o
`ExecRunner` real e chama `initai.Run`. **Commit(s):** `feat(init-ai): end-to-end
AI environment setup` (se precisar, quebre em config+state / initai / cmd).

---

## M5 — Gestão e criação de projetos

### Fase 9 · `profile` / `vault` / `docs` (comandos)

Subcomandos com `Args` validados pelo Cobra (`cobra.ExactArgs(1)` etc.). Padrão
repetido: resolver o dir em `~/.ray`, chamar a lógica do pacote, formatar a saída
em `cmd.OutOrStdout()`. `openutil` usa `exec` pra abrir caminho no app padrão
(`xdg-open` no Linux, `open` no macOS). **Commit:** `feat(cmd): profile, vault and
docs management commands`.

### Fase 10 · `run` + `new` (com as melhorias ⭐)

`runfile.Load(workdir)` mescla global + `ray.yaml` (achado **subindo a árvore** —
um `for` que vai pra `filepath.Dir(d)` até a raiz). Projeto sobrescreve global.

⭐ **Args no `run`:** o Cobra te entrega tudo após `--` em `args`; repasse ao
último passo. ⭐ **`.gitignore` no `new`:** depois de `git init`, escreva um
`.gitignore` (template base + bloco por stack: `graphify-out/`, `.env`,
`node_modules/`, `.dart_tool/`, etc.) — o bug que faltava resolver. `project.Create`
roda os `create` da receita e aborta se algum falhar; depois `cmd/new.go` chama
`initai.Run`. **Commits:** `feat(run): named aliases with argument passing` ·
`feat(new): scaffold a project (+ .gitignore) and set up its AI env`.

---

## M6 — Acabamento

### Fase 11 · `ray update` + Makefile/CI + README/completion
⭐ `update`: roda `npx skills update` e `uv tool upgrade` via runner. Makefile com
`build/test/vet/fmt/ci`; CI roda `make ci`. README com uso + seção de
`ray completion` (o Cobra gera os scripts de bash/zsh/fish de graça).

### Fase 12 · Validação manual
Com `RAY_HOME=$(mktemp -d)` pra não sujar o `~/.ray` real: `ray doctor [--fix]`,
`ray vault init`, `ray init ai --profile go --dry-run` e depois real, `ray new go
/tmp/x` (conferir `.gitignore` + `git status` limpo), e o `--mode learn` (o guard
bloqueia editar código).

---

# Parte 3 — Testes em Go

Testar não é opcional aqui — é o que torna o ray confiável e refatorável.

## 3.1 O básico
- Arquivo `xxx_test.go` no **mesmo pacote** (acessa o não-exportado) ou em
  `xxx_test` (testa só a API pública).
- `func TestFoo(t *testing.T)`. Rode com `go test ./...` (`-v` verboso, `-run
  TestFoo` filtra, `-cover` mostra cobertura).
- `t.Errorf` reporta e continua; `t.Fatalf` reporta e aborta **aquele** teste.

## 3.2 Table-driven tests (o idioma dominante)
Você já viu na Fase 3. A estrutura: um slice de casos, um `for` com `t.Run` pra
**subteste nomeado** (aparece individualmente na saída, dá pra filtrar):

```go
for _, tc := range cases {
	t.Run(tc.name, func(t *testing.T) {
		got := f(tc.in)
		if got != tc.want {
			t.Errorf("f(%v) = %v, want %v", tc.in, got, tc.want)
		}
	})
}
```

## 3.3 Helpers do `testing` que o ray usa muito
- `t.TempDir()` — diretório temporário **limpo automaticamente** ao fim. É como o
  ray testa escrita de arquivos sem sujar nada.
- `t.Setenv("RAY_HOME", dir)` — seta env só durante o teste, restaurando depois.
  Combinado com `t.TempDir()`, isola completamente o `~/.ray`.
- `t.Helper()` — marque funções auxiliares de teste pra que falhas apontem a
  linha do **chamador**, não a do helper.
- `t.Cleanup(fn)` — agenda limpeza (alternativa a `defer` em setup compartilhado).

## 3.4 Fakes via interface
A lição central do ray: **dependa de interfaces, injete fakes**. `Runner`→
`FakeRunner`, `Looker`→ função mock, `io.Writer`→ `bytes.Buffer`. Nenhum teste
toca rede, processo de verdade (salvo o `echo`) ou seu `~/.ray` real. É isso que
permite rodar a suíte em milissegundos, no CI, offline.

---

# Parte 4 — Empacotar e distribuir

```sh
go build -o ray .                 # binário local
go install github.com/murilopmr/ray@latest   # instala em ~/go/bin

# cross-compile: só mudar GOOS/GOARCH (sem toolchain extra)
GOOS=darwin GOARCH=arm64 go build -o ray-mac .
GOOS=windows GOARCH=amd64 go build -o ray.exe .
```

**Makefile** (alvos do guia §18) padroniza o fluxo; **CI** roda `make ci`
(`fmt-check + vet + test`) a cada push/PR. Como o binário é único e estático,
"distribuir" é literalmente copiar o arquivo.

---

# Apêndice — referência rápida

## Comandos go do dia a dia
```sh
go run .                 # roda o pacote atual
go build ./...           # compila tudo
go test ./... -cover     # testa com cobertura
go test -run TestX ./internal/profile/   # um teste específico
go vet ./...             # linter embutido
gofmt -w .               # formata e grava
go mod tidy              # acerta dependências
go doc fmt.Println       # docs no terminal
```

## Tabela mental de conceitos (Go → ray)
| Conceito Go | Onde aparece no ray |
|---|---|
| interface + injeção | `Runner`/`FakeRunner`, `Looker` — testabilidade |
| múltiplos retornos + `error` | toda função de IO; `valor, err :=` |
| `%w` + `errors.Is/As` | embrulhar erro de profile; separar exit≠0 de exec-falha |
| zero value | `State{}`/`Config{}` vazios já úteis |
| `map[string]any` | JSON dinâmico de `.mcp.json`/`settings.json` |
| struct tags | YAML de profile/config/state |
| `go:embed` | templates de scaffold no binário |
| `text/template` | renderizar CLAUDE.md, regras, hooks |
| receiver ponteiro | `State.AddGlobal` muta o estado |
| `io.Writer` | `cmd.OutOrStdout()` → saída testável |
| `t.TempDir`/`t.Setenv` | isolar `~/.ray` nos testes |

## Onde aprender mais (oficial, gratuito)
- **A Tour of Go** — <https://go.dev/tour> (interativo, comece aqui se algo acima ficou nebuloso).
- **Effective Go** — <https://go.dev/doc/effective_go> (idiomas e estilo).
- **Go by Example** — <https://gobyexample.com> (receitas curtas por tópico).
- **Documentação da stdlib** — <https://pkg.go.dev/std> (`os`, `os/exec`, `io`, `encoding/json`, `text/template`, `testing`).
- **Cobra** — <https://pkg.go.dev/github.com/spf13/cobra> e o user guide no repo.
- **Release notes do Go 1.25** — <https://go.dev/doc/go1.25>.

---

> **Como progredir sem se perder:** faça um milestone por vez, sempre verde,
> sempre commitado. Se travar num conceito de Go, pare e faça aquele trecho do
> Tour. O ray é grande, mas cada peça é pequena e isolada de propósito — essa é a
> recompensa do design por fronteiras.

---

### Fontes (consultadas para alinhar versões e práticas atuais)
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25)
- [Go 1.25 is released — The Go Blog](https://go.dev/blog/go1.25)
- [spf13/cobra — pkg.go.dev](https://pkg.go.dev/github.com/spf13/cobra)
- [spf13/cobra — user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)

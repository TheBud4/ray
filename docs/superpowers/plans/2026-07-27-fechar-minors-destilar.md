# Fechar os minors do rollout do `/destilar` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar os dois cabos soltos que a revisão final do `/destilar` deixou: a guarda "brain não configurado" que nenhum teste protege, e a posição indefinida do `.claude/handoff.md` no `.gitignore`.

**Architecture:** Nenhuma lógica nova. Task 1 é uma asserção sobre string de template. Task 2 acrescenta uma linha ao bloco que o scaffold escreve em `.gitignore`, mais o teste de espelho.

**Tech Stack:** Go 1.25, `testing` puro, `t.TempDir()`.

## Como este plano difere dos anteriores

**O RED não pertence à task.** Nos planos anteriores cada task começava em
"escreva o teste que falha", e o implementador escrevia teste e implementação no
mesmo commit — o vermelho nunca era visto por ninguém além de quem tinha
interesse em fazê-lo passar. Aqui:

- O **coordenador** escreve o teste da seção `RED` de cada task, roda, e confirma
  que a falha é pela razão certa. Isso acontece **antes** do dispatch.
- O **implementador** recebe a árvore com o teste já falhando. O trabalho dele
  começa em GREEN.
- O commit sai com teste e implementação juntos, como sempre — a diferença é
  quem escreveu cada metade e quem viu o vermelho.

Um implementador que receber esta plano e não encontrar o teste falhando na
árvore deve **parar e reportar**: o passo do coordenador não aconteceu, e
escrever o próprio RED anularia o ponto.

## Global Constraints

- Idioma: doc de pacote e comentário em **português**; identificador e nome de teste em **inglês**. Mensagem de commit em **inglês**, Conventional Commits.
- **Nunca** incluir trailer `Co-Authored-By` em commit.
- **Nunca** `git push` sem pedido explícito.
- `git add` sempre seletivo, nunca `git add .`/`-A`.
- Gate: `make ci` (roda `fmt-check`, `vet`, `test`) — verde, 20 pacotes `ok`.
- Mensagem de falha de teste no formato `got = %v, want %v`.

## File Structure

| Arquivo | Responsabilidade | Task |
|---|---|---|
| `internal/scaffold/scaffold_test.go` | asserção da guarda do brain | 1 |
| `internal/scaffold/scaffold.go` (`gitignoreBaseLines`, ~linha 173) | posição do handoff no bloco de `.gitignore` | 2 |
| `internal/scaffold/scaffold_test.go` | espelho da linha nova | 2 |

---

### Task 1: Proteger a guarda "brain não configurado"

**Contexto:** a revisão final do rollout acrescentou, em `destilar.md.tmpl` e
`revisar.md.tmpl`, um parágrafo que manda parar quando o MCP `brain` não está
disponível. Sem ele os dois comandos falham de forma confusa em projeto sem
cérebro configurado. Nenhuma asserção protege esse texto: apagá-lo hoje não
quebra a suíte.

**Files:**
- Test: `internal/scaffold/scaffold_test.go` — acrescentar ao fim
- Nenhum arquivo de produção muda nesta task. Se o teste passar de primeira, veja "Se o RED não ficar vermelho" abaixo.

**Interfaces:**
- Consumes: `WriteFiles`, `Options`, `Data` — já existentes em `internal/scaffold`.
- Produces: nada para tasks posteriores.

#### RED — escrito pelo COORDENADOR, antes do dispatch

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestBrainCommandsGuardMissingBrain(t *testing.T) {
	target := t.TempDir()

	if _, err := WriteFiles([]profile.ScaffoldFile{
		{Path: ".claude/commands/destilar.md"},
		{Path: ".claude/commands/revisar.md"},
	}, Options{
		Target: target,
		Data:   Data{ProjectName: "demo", Stack: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	// Os dois comandos leem a spec pelo MCP brain. Num projeto sem cérebro
	// configurado, sem esta guarda eles falham no meio da leitura, sem dizer
	// o que fazer. A guarda é a única saída acionável que o usuário tem.
	for _, p := range []string{".claude/commands/destilar.md", ".claude/commands/revisar.md"} {
		body, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		txt := string(body)
		for _, want := range []string{
			"não estiver disponível",
			"ray brain set",
		} {
			if !strings.Contains(txt, want) {
				t.Errorf("%s perdeu a guarda de brain ausente: falta %q", p, want)
			}
		}
	}
}
```

Rode e registre a saída:

```bash
cd /home/thebud4/www/Projetos/ray
go test ./internal/scaffold/ -run TestBrainCommandsGuardMissingBrain -v
```

**Este teste afirma texto que já existe**, então passa de primeira — é
regressão, não feature. Para provar que a asserção morde, o coordenador removeu
temporariamente as três linhas da guarda de `destilar.md.tmpl`, rodou, e
restaurou (`git diff --stat` vazio depois, confirmando restauração byte a byte).

**Saída real da falha, com a guarda removida:**

```
--- FAIL: TestBrainCommandsGuardMissingBrain (0.00s)
    scaffold_test.go:563: .claude/commands/destilar.md perdeu a guarda de brain ausente: falta "não estiver disponível"
    scaffold_test.go:563: .claude/commands/destilar.md perdeu a guarda de brain ausente: falta "ray brain set"
FAIL
FAIL	github.com/TheBud4/ray/internal/scaffold	0.002s
```

Com a guarda restaurada: `ok github.com/TheBud4/ray/internal/scaffold 0.002s`.

- [ ] **Step 1: Ler a saída do RED acima**

Já foi executada pelo coordenador — a saída está registrada. **Não repita a
remoção temporária**, e não reescreva o teste: ele já está na árvore.

- [ ] **Step 2: Rodar o teste na árvore recebida**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ -run TestBrainCommandsGuardMissingBrain -v`

Expected: PASS — a guarda está nos dois templates, e o teste agora a protege.

Se der FAIL, **pare e reporte**: significa que a árvore que você recebeu não tem
a guarda, e o problema é outro.

- [ ] **Step 3: Rodar o gate completo**

Run: `cd /home/thebud4/www/Projetos/ray && make ci`

Expected: `fmt-check`, `vet` e `test` sem falha; 20 pacotes `ok`.

- [ ] **Step 4: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/scaffold_test.go
git commit -F - <<'EOF'
test(scaffold): pin the missing-brain guard in both spec commands

/destilar and /revisar both read the spec over the brain MCP. The whole-branch
review added a guard that stops with an actionable message when the brain is
not configured, and nothing asserted it: deleting those two lines left the
suite green and every new project got a command that fails mid-read with no
instruction.
EOF
```

---

### Task 2: Fixar a posição do `.claude/handoff.md` no `.gitignore`

**Contexto e a decisão que ela carrega:** o bloco que `MergeGitignore` escreve
tem duas listas — o whitelist do conteúdo de IA que **é** commitado
(`.claude/skills/`, `.claude/agents/`, `.claude/commands/`, `settings.json`,
`.mcp.json`, `docs/`) e o blacklist do que **nunca** é (`.claude/.local/`,
`.claude/.ray-metrics/`, `.env`, `*.local`). O `.claude/handoff.md` não está em
nenhuma das duas. O resultado é que ele aparece como untracked para sempre, e
cada projeto decide por acidente.

> **Suposição adotada pelo coordenador — o usuário pode reverter em uma linha.**
> O handoff vai para o **blacklist**. Razão: a regra de roteamento deste projeto
> é binária — descreve o estado atual do sistema → repo; qualquer outra coisa →
> pessoal. O handoff descreve **onde o trabalho parou**, que é processo, não
> estado. Some-se a isso que ele é regenerado por inteiro a cada `/handoff` e é
> imune a `--force` justamente por ser gerido pela IA, não pelo time.
> Se a decisão for a oposta (handoff commitado, para viajar entre máquinas do
> mesmo autor), a mudança é trocar a linha de lista e inverter a asserção do
> teste.

**Files:**
- Modify: `internal/scaffold/scaffold.go` — `gitignoreBaseLines`, ~linha 185-191
- Test: `internal/scaffold/scaffold_test.go` — acrescentar ao fim

**Interfaces:**
- Consumes: `MergeGitignore(target string, stackLines []string, data Data, dryRun bool, out io.Writer) error` e a var `gitignoreBaseLines []string`, ambos em `internal/scaffold`.
- Produces: nada para tasks posteriores.

#### RED — escrito pelo COORDENADOR, antes do dispatch

Acrescente ao fim de `internal/scaffold/scaffold_test.go`:

```go
func TestGitignoreIgnoresHandoff(t *testing.T) {
	target := t.TempDir()

	if err := MergeGitignore(target, nil, Data{}, false, nil); err != nil {
		t.Fatalf("MergeGitignore() error = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)

	// O handoff descreve onde o trabalho parou — processo, não estado do
	// sistema. Ele é regenerado inteiro a cada /handoff e é gerido pela IA.
	// Sem uma linha explícita ele fica untracked para sempre e cada projeto
	// decide por acidente.
	if !strings.Contains(got, ".claude/handoff.md") {
		t.Errorf("bloco do .gitignore não menciona .claude/handoff.md:\n%s", got)
	}
	// Precisa estar na blacklist, não negado no whitelist.
	if strings.Contains(got, "!.claude/handoff.md") {
		t.Error("got = handoff no whitelist, want na blacklist")
	}
}
```

O `nil` no último parâmetro segue o padrão dos testes vizinhos de
`MergeGitignore` — evita importar `io` só para um `Discard`.

Rode e registre a saída:

```bash
cd /home/thebud4/www/Projetos/ray
go test ./internal/scaffold/ -run TestGitignoreIgnoresHandoff -v
```

**Saída real da falha:**

```
--- FAIL: TestGitignoreIgnoresHandoff (0.00s)
    scaffold_test.go:587: bloco do .gitignore não menciona .claude/handoff.md:
        # >>> ray
        # Conteúdo de IA vendorizado — commitado (não editar as negações abaixo)
        !.claude/skills/
        !.claude/agents/
        !.claude/commands/
        !.claude/settings.json
        !.mcp.json
        !docs/
        !.claude/.ray-profile
        !**/.ray-origin
        !**/LICENSE

        # Runtime, segredos e pessoal — nunca commitados
        graphify-out/
        .claude/.ray-metrics/
        .claude/.local/
        .env
        *.local
```

O bloco impresso é a evidência do diagnóstico: o handoff não está em nenhuma das
duas listas.

- [ ] **Step 1: Ler a saída do RED acima**

Já foi executada pelo coordenador. Confirme que a falha na sua árvore é essa.
**Não escreva o teste** — ele já está em `internal/scaffold/scaffold_test.go`.

- [ ] **Step 2: Acrescentar a linha à blacklist**

Em `internal/scaffold/scaffold.go`, no slice `gitignoreBaseLines`, dentro do
grupo que começa em `"# Runtime, segredos e pessoal — nunca commitados"`,
acrescente a linha depois de `".claude/.local/"`:

```go
	"# Runtime, segredos e pessoal — nunca commitados",
	"graphify-out/",
	".claude/.ray-metrics/",
	".claude/.local/",
	".claude/handoff.md",
	".env",
	"*.local",
}
```

- [ ] **Step 3: Rodar o teste**

Run: `cd /home/thebud4/www/Projetos/ray && go test ./internal/scaffold/ -run TestGitignoreIgnoresHandoff -v`

Expected: PASS.

- [ ] **Step 4: Rodar o gate completo**

Run: `cd /home/thebud4/www/Projetos/ray && make ci`

Expected: `fmt-check`, `vet` e `test` sem falha; 20 pacotes `ok`.

Atenção: `TestMergeGitignoreIdempotent` e `TestMergeGitignoreCreatesWhitelistAndBlacklist`
exercitam o mesmo bloco. Se algum deles quebrar, é porque afirma o conteúdo
literal do bloco — leia a asserção e ajuste-a para incluir a linha nova, sem
afrouxá-la.

- [ ] **Step 5: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add internal/scaffold/scaffold.go internal/scaffold/scaffold_test.go
git commit -F - <<'EOF'
feat(scaffold): keep the handoff out of git

The gitignore block ray writes names what is committed and what never is, and
the handoff was in neither list - so it stayed untracked forever and every
project settled the question by accident.

It goes in the blacklist. The routing rule this project follows is binary:
what describes the system's current state travels with the repo, everything
else does not. A handoff describes where work stopped, which is process. It is
also regenerated whole on every run and is AI-managed, which is why --force
already refuses to touch it.
EOF
```

---

### Task 3: Aplicar a mesma decisão ao `.gitignore` deste repositório

**Contexto:** a Task 2 muda o que o `ray` escreve nos projetos **dos outros**. O
`.gitignore` deste repo é escrito à mão e não passa pelo `MergeGitignore` — ele
tem `.claude/.local/`, `.vscode/` e `.superpowers/`, mas não o handoff. Sem esta
task, o `ray` passa a recomendar aos outros uma regra que ele mesmo não segue:
a lacuna de dogfooding que `c106699` acabou de fechar do lado do `CLAUDE.md`.

**Files:**
- Modify: `.gitignore` (raiz do repo)

**Interfaces:**
- Consumes: a decisão da Task 2.
- Produces: nada.

**Sem RED:** não há teste que afirme o conteúdo do `.gitignore` do próprio repo,
e criar um seria testar configuração local em vez de comportamento do programa.
A verificação é o `git status` do Step 2.

- [ ] **Step 1: Acrescentar a linha**

Em `.gitignore`, no fim do arquivo:

```
# Estado vivo entre sessões — gerido pela IA, não viaja com o repo
.claude/handoff.md
```

- [ ] **Step 2: Confirmar que o handoff sumiu do status**

Run: `cd /home/thebud4/www/Projetos/ray && git status --short`

Expected: `.claude/handoff.md` não aparece mais. (O arquivo `aa.txt`, se ainda
existir, continua aparecendo — não é desta linha de trabalho, não toque nele.)

- [ ] **Step 3: Confirmar que ele nunca foi rastreado**

Run: `cd /home/thebud4/www/Projetos/ray && git ls-files --error-unmatch .claude/handoff.md 2>&1 | head -1`

Expected: erro `did not match any file(s) known to git` — o arquivo nunca entrou
no índice, então não é preciso `git rm --cached`.

Se o comando **não** der erro, o arquivo está rastreado: rode
`git rm --cached .claude/handoff.md` antes de commitar, e diga isso no relatório.

- [ ] **Step 4: Commit**

```bash
cd /home/thebud4/www/Projetos/ray
git add .gitignore
git commit -F - <<'EOF'
chore: ignore this repo's own handoff

ray now tells scaffolded projects to keep the handoff out of git. This repo's
.gitignore is hand-written and never passed through MergeGitignore, so without
this it would recommend a rule it does not follow.
EOF
```

---

## Verificação final

- [ ] `cd /home/thebud4/www/Projetos/ray && make ci` — 20 pacotes `ok`
- [ ] `git log --oneline -3` mostra os três commits
- [ ] `git log -3 --format=%B | grep -ci co-authored` devolve `0`
- [ ] `git status --short` não lista `.claude/handoff.md`
- [ ] Nenhum push feito

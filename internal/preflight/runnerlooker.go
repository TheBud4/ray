package preflight

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/TheBud4/ray/internal/runner"
)

const pythonWantMajor, pythonWantMinor = 3, 10

// lookTimeout limita cada `<nome> --version`. Sem ele um `npx` que não
// responde trava o processo inteiro — inclusive o `ray` sem subcomando, que é
// a tela mais vista do CLI e a que menos pode ficar pendurada. Estourar o
// prazo conta como ausente: uma dependência que não responde em 3s não serve
// para rodar nada, e Look só sabe dizer sim ou não.
const lookTimeout = 3 * time.Second

// RunnerLooker é a implementação real de Looker: verifica cada dependência
// rodando "<nome> --version" através de runner.Runner — a única fronteira de
// processos externos do ray (docs/architecture.md). Um binário ausente faz
// Runner.Run devolver err != nil (o ExecRunner distingue "não achou o
// executável" de "rodou e falhou"), o que aqui vira Look = false.
type RunnerLooker struct {
	Runner runner.Runner
}

// Look reporta se name está disponível. python3.10+ é o único caso especial:
// roda `python3 --version` e exige major.minor >= 3.10.
func (l RunnerLooker) Look(name string) bool {
	bin, args := "", []string{"--version"}
	if name == "python3.10+" {
		bin = "python3"
	} else {
		bin = name
	}

	ctx, cancel := context.WithTimeout(context.Background(), lookTimeout)
	defer cancel()

	res, err := l.Runner.Run(ctx, runner.Command{Name: bin, Args: args})
	if err != nil {
		return false
	}
	if res.ExitCode != 0 {
		return false
	}
	if name == "python3.10+" {
		return pythonVersionAtLeast(res.Stdout, pythonWantMajor, pythonWantMinor)
	}
	return true
}

// pythonVersionAtLeast faz parsing de saídas como "Python 3.11.4\n" e checa
// major.minor >= (wantMajor, wantMinor).
func pythonVersionAtLeast(output string, wantMajor, wantMinor int) bool {
	fields := strings.Fields(output)
	if len(fields) < 2 {
		return false
	}
	parts := strings.SplitN(fields[1], ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	return minor >= wantMinor
}

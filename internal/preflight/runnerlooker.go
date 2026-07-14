package preflight

import (
	"context"
	"strconv"
	"strings"

	"github.com/TheBud4/ray/internal/runner"
)

const pythonWantMajor, pythonWantMinor = 3, 10

// RunnerLooker é a implementação real de Looker: verifica cada dependência
// rodando "<nome> --version" através de runner.Runner — a única fronteira de
// processos externos do ray (§3 do build guide). Um binário ausente faz
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

	res, err := l.Runner.Run(context.Background(), runner.Command{Name: bin, Args: args})
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

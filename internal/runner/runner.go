// Package runner is the only frontier from ray to external processes.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command represents a process to execute.
// The zero-value Command{} is invalid (Name is empty), but Args/Dir are optional.
type Command struct {
	Name string
	Args []string
	Dir  string // Working directory ("" = actual).
	// Env são variáveis extras injetadas no ambiente do subprocesso (ex.
	// DO_NOT_TRACK=1 para o CliAcquirer). nil = herda só o ambiente do
	// processo pai, sem adições.
	Env map[string]string
	// Stdin, Stdout e Stderr, quando definidos, ligam o processo direto a esses
	// fluxos: a saída chega em tempo real, o processo pode ler a entrada, e
	// stdout e stderr ficam separados. Quando nulos, o processo roda sem entrada
	// e a saída é acumulada em Result.Stdout/Result.Stderr (o comportamento de
	// sempre). Um Stdout definido deixa Result.Stdout vazio — a saída já foi para
	// onde foi pedida.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// String gives a legible form to logs and error messages.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Result is what is left after running a Command.
type Result struct {
	Stdout, Stderr string
	ExitCode       int
}

// Runner is the contract: it knows how to execute a Command.
// Everything in ray depends on this interface rather than on exec directly — that's what enables FakeRunner in tests.
// Única exceção: spawnEditor (internal/cmd/profile.go) — um editor interativo precisa herdar o terminal cru,
// e não só receber streams (Command.Stdin/Stdout/Stderr).
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}

// waitDelay é quanto o Run ainda espera pelo fim da E/S depois que o processo
// morreu ou o contexto foi cancelado.
const waitDelay = time.Second

// ExecRunner runs the actual commands. If the DryRun flag is active, it only prints them.
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

	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	// O cancelamento do contexto mata o processo, não os netos; um neto que
	// segura o pipe de saída prenderia o Run até terminar sozinho. Depois do
	// cancelamento, esperar pelo pipe mais que isto não traz saída útil.
	cmd.WaitDelay = waitDelay
	if c.Env != nil {
		cmd.Env = os.Environ()
		for k, v := range c.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	// Fluxo dado no Command: o processo escreve/lê direto nele, sem buffer.
	// Os que faltam continuam sendo acumulados em Result.
	var stdout, stderr bytes.Buffer
	cmd.Stdin = c.Stdin
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	}
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	}

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}

	return res, nil
}

// FailureReason resume em uma linha por que um comando falhou: vazio quando
// deu certo, o erro quando o processo nem chegou a rodar e, quando saiu com
// código diferente de zero, "exit N" seguido da última linha não vazia do
// stderr (cortada em 200 bytes, com "..." ao fim).
func FailureReason(res Result, err error) string {
	if err != nil {
		return err.Error()
	}
	if res.ExitCode == 0 {
		return ""
	}
	reason := fmt.Sprintf("exit %d", res.ExitCode)
	lines := strings.Split(res.Stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if len(line) > maxReasonBytes {
			line = line[:maxReasonBytes] + "..."
		}
		return reason + ": " + line
	}
	return reason
}

// maxReasonBytes limita a linha de stderr que FailureReason cita.
const maxReasonBytes = 200

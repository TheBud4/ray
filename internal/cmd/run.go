package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runfile"
	"github.com/TheBud4/ray/internal/runner"
)

var flagRunList bool

func newRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run [alias] [-- extra args]",
		Short: "Run a project or global command alias",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sem `--`, só o alias é consumido: qualquer outro argumento seria
			// descartado em silêncio e o comando sairia 0 sem fazer o pedido.
			if cmd.ArgsLenAtDash() == -1 && len(args) > 1 {
				return fmt.Errorf("unexpected argument(s) %q after alias %q: put the arguments for the command after `--` (ray run %s -- ...)", args[1:], args[0], args[0])
			}
			alias, extra := splitAliasArgs(args, cmd.ArgsLenAtDash())

			workdir, err := os.Getwd()
			if err != nil {
				return err
			}
			globalPath, err := raypaths.CommandsPath()
			if err != nil {
				return err
			}
			commands, err := runfile.Load(workdir, globalPath)
			if err != nil {
				return err
			}

			r := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			return runRunCmd(commands, alias, extra, flagRunList, r, flagVerbose, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&flagRunList, "list", false, "list available aliases")
	return c
}

// splitAliasArgs separa o alias dos args após `--` (dashAt = -1 quando não há
// `--`, vem de cmd.ArgsLenAtDash()).
func splitAliasArgs(args []string, dashAt int) (alias string, extra []string) {
	if dashAt == -1 {
		if len(args) > 0 {
			alias = args[0]
		}
		return alias, nil
	}
	if dashAt > 0 {
		alias = args[0]
	}
	return alias, args[dashAt:]
}

func runRunCmd(commands map[string]runfile.Resolved, alias string, extra []string, list bool, r runner.Runner, verbose bool, out io.Writer) error {
	if list || alias == "" {
		printAliasList(out, commands)
		return nil
	}

	res, ok := commands[alias]
	if !ok {
		return fmt.Errorf("unknown alias %q (see `ray run --list`)", alias)
	}

	// Todos os passos são divididos antes de rodar o primeiro: um passo
	// inválido não pode aparecer depois de os anteriores já terem executado.
	parsed := make([][]string, len(res.Steps))
	for i, step := range res.Steps {
		fields, err := splitCommand(step)
		if err != nil {
			return err
		}
		parsed[i] = fields
	}

	for i, step := range res.Steps {
		fields := parsed[i]
		if len(fields) == 0 {
			continue
		}
		if i == len(res.Steps)-1 {
			fields = append(fields, extra...)
		}
		if verbose {
			fmt.Fprintf(out, "> %s\n", step)
		}
		result, err := r.Run(context.Background(), runner.Command{Name: fields[0], Args: fields[1:], Dir: res.BaseDir})
		if err != nil {
			return err
		}
		io.WriteString(out, result.Stdout)
		io.WriteString(out, result.Stderr)
		if result.ExitCode != 0 {
			return fmt.Errorf("step %q exited with code %d", step, result.ExitCode)
		}
	}
	return nil
}

// splitCommand divide um passo de alias em argumentos como um shell faria com
// as aspas: espaço e tab separam; aspas simples são literais; aspas duplas
// aceitam `\"` e `\\` como escapes (outras barras ficam literais); fora de
// aspas, a barra invertida escapa o caractere seguinte; `""` gera um argumento
// vazio. Não há expansão de variável nem de curinga — o ray não é um shell, e
// o passo é executado direto, sem `sh -c`. Aspas sem fechar são erro.
func splitCommand(step string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inToken := false
	runes := []rune(step)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case ' ', '\t':
			if inToken {
				args = append(args, cur.String())
				cur.Reset()
				inToken = false
			}
		case '\'':
			inToken = true
			i++
			for ; i < len(runes) && runes[i] != '\''; i++ {
				cur.WriteRune(runes[i])
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("step `%s`: unterminated quote", step)
			}
		case '"':
			inToken = true
			i++
			for ; i < len(runes) && runes[i] != '"'; i++ {
				if runes[i] == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\') {
					i++
				}
				cur.WriteRune(runes[i])
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("step `%s`: unterminated quote", step)
			}
		case '\\':
			inToken = true
			if i+1 < len(runes) {
				i++
			}
			cur.WriteRune(runes[i])
		default:
			inToken = true
			cur.WriteRune(r)
		}
	}
	if inToken {
		args = append(args, cur.String())
	}
	return args, nil
}

func printAliasList(out io.Writer, commands map[string]runfile.Resolved) {
	// O cabeçalho só faz sentido com linha embaixo: sozinho, ele sugere que
	// algo deveria estar listado. Nomear o arquivo evita a pergunta seguinte.
	if len(commands) == 0 {
		fmt.Fprintln(out, "no runs defined (add a commands: block to ray.yaml)")
		return
	}

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSOURCE\tDESCRIPTION")
	for _, name := range names {
		r := commands[name]
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Source, r.Description)
	}
	w.Flush()
}

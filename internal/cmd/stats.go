package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/metrics"
)

// statsLabels traduz MetricKey conhecidos (internal/economy.Mechanism) em
// rótulos legíveis. Fica local a este comando — é uma preocupação de
// apresentação, não de identidade do mecanismo (não engorda economy.Mechanism).
var statsLabels = map[string]string{
	"graph_queries": "graph queries",
	"compressions":  "context compressions",
	"handoffs":      "handoffs",
}

func newStatsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stats [path]",
		Short: "Show proxy activity for the Token Economy mechanisms (measured activity only, never invented tokens)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return runStats(target, cmd.OutOrStdout())
		},
	}
	return c
}

// runStats agrega os proxies de metrics.Aggregate e imprime uma linha
// legível — ou uma mensagem de estado vazio, honesta, se nada foi medido
// ainda.
func runStats(target string, out io.Writer) error {
	counts, err := metrics.Aggregate(metrics.Dir(target))
	if err != nil {
		return err
	}
	if len(counts) == 0 {
		fmt.Fprintln(out, "no metrics recorded yet")
		return nil
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		label, ok := statsLabels[k]
		if !ok {
			label = k
		}
		parts[i] = fmt.Sprintf("%d %s", counts[k], label)
	}
	fmt.Fprintln(out, strings.Join(parts, " · "))
	return nil
}

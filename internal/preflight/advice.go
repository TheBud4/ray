package preflight

import (
	"bytes"
	"fmt"
	"text/tabwriter"
)

// Origin diz de onde o gate falhou. É o que muda o conselho e o rodapé, e
// existe como tipo em vez de dois booleanos porque os três casos são
// mutuamente exclusivos — booleanos deixariam representar o estado impossível
// "estou no doctor e não estou".
type Origin int

const (
	// FromGate: o erro veio de um comando que não é o doctor (hoje só o
	// `init ai`). Vale apontar a tabela completa no rodapé.
	FromGate Origin = iota
	// FromDoctor: já estamos no doctor, com a tabela impressa logo acima.
	FromDoctor
	// FromDoctorFix: o `--fix` já rodou. Apontar para ele de novo mandaria
	// repetir o comando que acabou de não funcionar.
	FromDoctorFix
)

const missingFooter = "run `ray doctor` for the full table"

// Advice diz o que fazer com um Check que falhou: aponta o `ray doctor --fix`
// quando o ray sabe instalar sozinho, e o Hint da tabela quando não sabe.
// Vazio quando não há nada a dizer — nem todo Check tem Hint.
func Advice(c Check, from Origin) string {
	if len(c.Fix) > 0 {
		if from == FromDoctorFix {
			return "automatic fix ran; install it manually"
		}
		return "ray doctor --fix"
	}
	return c.Hint
}

// MissingRequiredError é o gate de dependência falhando. Carrega os Checks em
// vez de uma string pronta para que este Error() seja a única renderização —
// dois formatadores para a mesma pergunta divergem, e este projeto já pagou
// isso duas vezes (countInventory duplicado, gitScope contra a whitelist).
//
// Erro multilinha não é idiomático em Go, e é aceito aqui de propósito: este
// erro é saída de topo de um CLI, não um valor que outro código inspeciona.
type MissingRequiredError struct {
	Missing []Check
	From    Origin
}

func (e *MissingRequiredError) Error() string {
	var b bytes.Buffer
	b.WriteString("missing required dependencies\n\n")

	w := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	for _, c := range e.Missing {
		fmt.Fprintf(w, "  %s\t%s\n", c.Name, Advice(c, e.From))
	}
	w.Flush()

	if e.From == FromGate {
		fmt.Fprintf(&b, "\n%s", missingFooter)
	}
	return b.String()
}

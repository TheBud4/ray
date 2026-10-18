package catalog

import (
	"bytes"
	"strings"

	"github.com/TheBud4/ray/internal/linediff"
)

// Outcome é o que uma edição fez ao template.
type Outcome string

const (
	OutcomeNoChanges Outcome = "no changes"
	OutcomeCurrent   Outcome = "current" // igual ao padrão embutido
	// Qualquer outro valor é string(State): "edited" ou "custom".
)

// Classify compara o conteúdo de antes e de depois de uma edição e diz o que ela
// fez. Bytes iguais são OutcomeNoChanges, sem consultar o overlay; senão vale o
// estado que o template tem agora.
func (t Templates) Classify(name string, before, after []byte) (Outcome, error) {
	if bytes.Equal(before, after) {
		return OutcomeNoChanges, nil
	}
	entry, err := t.Lookup(name)
	if err != nil {
		return "", err
	}
	if entry.State == Current {
		return OutcomeCurrent, nil
	}
	return Outcome(entry.State), nil
}

// CountChanged conta as linhas que diferem entre o overlay e o embutido; -1 se o
// diff não pôde ser calculado.
func CountChanged(overlay, embedded []byte) int {
	d, err := linediff.Unified(string(embedded), string(overlay), "a", "b")
	if err != nil {
		return -1
	}
	n := 0
	for l := range strings.SplitSeq(d, "\n") {
		if (strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++ ")) || (strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "--- ")) {
			n++
		}
	}
	return n
}

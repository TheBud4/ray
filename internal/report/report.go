// Package report define os itens tipados que initai e update devolvem em seus
// resumos e que o cmd renderiza. Fica num pacote próprio para que um domínio
// não precise importar o outro só para falar o mesmo vocabulário.
package report

// Kind diz o que um item é.
type Kind string

const (
	Component Kind = "component"
	Global    Kind = "global"
	File      Kind = "file"
	Command   Kind = "command"
)

// Outcome diz o que aconteceu com o item.
type Outcome string

const (
	Installed Outcome = "installed"
	Failed    Outcome = "failed"
	Created   Outcome = "created"
	Updated   Outcome = "updated"
	Unchanged Outcome = "unchanged"
	Kept      Outcome = "kept"
)

// Entry é um item do resumo. Detail traz o porquê quando há um (o motivo da
// falha, da preservação); vazio quando o resultado já diz tudo.
type Entry struct {
	Kind    Kind
	Name    string
	Outcome Outcome
	Detail  string
}

// Tally conta as entradas por resultado. Só aparecem os resultados presentes.
func Tally(entries []Entry) map[Outcome]int {
	counts := map[Outcome]int{}
	for _, e := range entries {
		counts[e.Outcome]++
	}
	return counts
}

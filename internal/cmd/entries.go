package cmd

import (
	"strings"

	"github.com/TheBud4/ray/internal/report"
	"github.com/TheBud4/ray/internal/ui"
)

// keptEditedPrefix é o começo do motivo com que o domínio marca um componente
// preservado por edição local.
const keptEditedPrefix = "edited locally"

// forceHint é a dica única que explica os componentes preservados por edição.
const forceHint = "use --force to overwrite the components kept above"

// entrySection é uma seção da tela de resumo e os tipos de entrada que ela lista.
type entrySection struct {
	name  string
	kinds []report.Kind
}

// entrySections é a ordem fixa das seções: ferramentas, componentes, arquivos.
var entrySections = []entrySection{
	{"Tools", []report.Kind{report.Global, report.Command}},
	{"Components", []report.Kind{report.Component}},
	{"Files", []report.Kind{report.File}},
}

// uiKinds traduz o resultado de uma entrada no estado de um item da lista.
var uiKinds = map[report.Outcome]ui.Kind{
	report.Installed: ui.OK,
	report.Created:   ui.Added,
	report.Updated:   ui.Updated,
	report.Failed:    ui.Fail,
	report.Kept:      ui.Kept,
	report.Unchanged: ui.Kept,
}

// printEntrySections imprime uma seção por grupo de entradas, cada uma
// precedida de linha em branco; seção sem entrada não aparece.
func printEntrySections(p *ui.Printer, entries []report.Entry) {
	for _, sec := range entrySections {
		var items []ui.Item
		for _, e := range entries {
			if sectionHas(sec, e.Kind) {
				items = append(items, entryItem(e))
			}
		}
		if len(items) == 0 {
			continue
		}
		p.Blank()
		p.Section(sec.name)
		p.Items(items)
	}
}

func sectionHas(sec entrySection, k report.Kind) bool {
	for _, kind := range sec.kinds {
		if kind == k {
			return true
		}
	}
	return false
}

// entryItem converte uma entrada em item de lista, com o detalhe já pronto
// para exibição.
func entryItem(e report.Entry) ui.Item {
	return ui.Item{Kind: uiKinds[e.Outcome], Name: e.Name, Detail: entryDetail(e)}
}

// entryDetail escolhe o que aparece ao lado do nome: o motivo do domínio com a
// home abreviada, ou um texto próprio quando o resultado não traz motivo.
func entryDetail(e report.Entry) string {
	switch {
	case e.Outcome == report.Kept && isKeptEdited(e):
		// O motivo completo repete o --force em cada linha; a dica sai uma
		// vez só, no bloco de avisos.
		reason, _, _ := strings.Cut(e.Detail, " (")
		return "kept — " + reason
	case e.Detail == "" && e.Outcome == report.Updated:
		return "updated"
	case e.Detail == "" && e.Outcome == report.Unchanged:
		return "unchanged"
	}
	return shortHome(e.Detail)
}

func isKeptEdited(e report.Entry) bool {
	return e.Outcome == report.Kept && strings.HasPrefix(e.Detail, keptEditedPrefix)
}

// printWarnings imprime os avisos do domínio e, depois deles, a dica do
// --force quando algum componente foi preservado por edição local. Tudo no
// mesmo bloco; sem aviso nem dica, não imprime nada.
func printWarnings(p *ui.Printer, warnings []string, entries []report.Entry) {
	hint := false
	for _, e := range entries {
		if isKeptEdited(e) {
			hint = true
			break
		}
	}
	if len(warnings) == 0 && !hint {
		return
	}
	p.Blank()
	for _, w := range warnings {
		p.Warning(w, "")
	}
	if hint {
		p.Warning(forceHint, "")
	}
}

// printEntryTally imprime o resumo com a contagem de todas as entradas, na
// ordem fixa. Em dry-run nada foi feito, então os rótulos de instalar, criar
// e preservar vão para o condicional.
func printEntryTally(p *ui.Printer, entries []report.Entry, dryRun bool) {
	t := report.Tally(entries)
	installed, created, kept := "installed", "created", "kept"
	if dryRun {
		installed, created, kept = "would install", "would create", "would skip"
	}
	counts := []ui.Count{
		{N: t[report.Failed], Label: "failed", Color: ui.Red},
		{N: t[report.Installed], Label: installed, Color: ui.Green},
		{N: t[report.Updated], Label: "updated", Color: ui.Yellow},
		{N: t[report.Created], Label: created, Color: ui.Green},
		{N: t[report.Unchanged], Label: "unchanged", Color: ui.Gray},
		{N: t[report.Kept], Label: kept, Color: ui.Gray},
	}
	hasAny := false
	for _, c := range counts {
		if c.N > 0 {
			hasAny = true
		}
	}
	if !hasAny {
		return
	}
	p.Blank()
	p.Summary(counts...)
}

package catalog

import (
	"strings"

	"github.com/TheBud4/ray/internal/linediff"
)

// DiffKind é a forma da resposta de Diff.
type DiffKind string

const (
	DiffUnified  DiffKind = "unified"   // há um diff unificado em Text
	DiffSame     DiffKind = "same"      // o diff saiu vazio
	DiffCRLFOnly DiffKind = "crlf-only" // só o fim de linha (CRLF) difere
	DiffCurrent  DiffKind = "current"   // idêntico ao embutido
	DiffNew      DiffKind = "new"       // ainda não está no overlay
	DiffCustom   DiffKind = "custom"    // não há embutido para comparar
)

// DiffResult é o resultado de comparar um template do overlay com o embutido.
type DiffResult struct {
	Kind     DiffKind
	Text     string // diff unificado quando Kind == DiffUnified
	Outdated bool   // o embutido mudou desde a linha-base (DiffUnified e DiffSame)
}

// Diff compara o template do overlay com o embutido. Current, New e Custom são
// respondidos só pelo Kind; CRLF é ignorado na comparação, e quando só isso
// difere o Kind é DiffCRLFOnly. Os rótulos do diff são embedded/<nome> e
// overlay/<nome>.
func (t Templates) Diff(name string) (DiffResult, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return DiffResult{}, err
	}
	switch e.State {
	case Current:
		return DiffResult{Kind: DiffCurrent}, nil
	case New:
		return DiffResult{Kind: DiffNew}, nil
	case Custom:
		return DiffResult{Kind: DiffCustom}, nil
	}
	embedded, err := t.Embedded(e.Name)
	if err != nil {
		return DiffResult{}, err
	}
	overlay, err := t.Overlay(e.Name)
	if err != nil {
		return DiffResult{}, err
	}
	if lf := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }; lf(embedded) == lf(overlay) {
		return DiffResult{Kind: DiffCRLFOnly}, nil
	}
	d, err := linediff.Unified(string(embedded), string(overlay), "embedded/"+e.Name, "overlay/"+e.Name)
	if err != nil {
		return DiffResult{}, err
	}
	outdated := e.State == Outdated
	if d == "" {
		return DiffResult{Kind: DiffSame, Outdated: outdated}, nil
	}
	return DiffResult{Kind: DiffUnified, Text: d, Outdated: outdated}, nil
}

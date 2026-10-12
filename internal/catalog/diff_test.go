package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestDiffAnswersEveryState(t *testing.T) {
	f := newFixture(t)
	if r, _ := f.cat.Diff(sample); r.Kind != "new" {
		t.Errorf("new template Kind = %q, want new", r.Kind)
	}

	f.overlay(sample, "# mine\n")
	r, err := f.cat.Diff(sample)
	if err != nil || r.Kind != "unified" || !strings.Contains(r.Text, "overlay/"+sample) || !strings.Contains(r.Text, "embedded/"+sample) {
		t.Errorf("edited = %+v, %v; want a unified diff labelled embedded/ and overlay/", r, err)
	}

	emb := f.embedded(sample)
	f.overlay(sample, string(emb))
	f.baseline(sample, emb)
	if r, _ := f.cat.Diff(sample); r.Kind != "current" {
		t.Errorf("current template Kind = %q, want current", r.Kind)
	}

	f.overlay("mine.tmpl", "x\n")
	if r, _ := f.cat.Diff("mine.tmpl"); r.Kind != "custom" {
		t.Errorf("custom template Kind = %q, want custom", r.Kind)
	}

	if _, err := f.cat.Diff("nope.tmpl"); !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown name error = %v, want ErrUnknown", err)
	}
}

func TestDiffSaysWhenOnlyTheLineEndingsDiffer(t *testing.T) {
	f := newFixture(t)
	emb := f.embedded(sample)
	f.overlay(sample, strings.ReplaceAll(string(emb), "\n", "\r\n"))
	r, err := f.cat.Diff(sample)
	if err != nil || r.Kind != "crlf-only" {
		t.Errorf("Diff = %+v, %v; want Kind crlf-only", r, err)
	}
}

func TestDiffFlagsAnOutdatedTemplate(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "old\n")
	f.baseline(sample, []byte("old\n"))
	r, err := f.cat.Diff(sample)
	if err != nil || !r.Outdated {
		t.Errorf("Diff = %+v, %v; want Outdated (the embedded moved on from the baseline)", r, err)
	}
}

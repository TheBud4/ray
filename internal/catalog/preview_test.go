package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// hook é um template de hook embutido: o nome termina em .sh.tmpl.
const hook = "claude/hooks/guard-add.sh.tmpl"

func TestPreviewRendersADraftWithoutWriting(t *testing.T) {
	f := newFixture(t)
	before := f.tree()
	res, err := f.cat.Preview(sample, []byte("# {{.ProjectName}}\n"), false)
	if err != nil || res.Err != nil || res.Output != "# example\n" {
		t.Fatalf("Preview = %+v, %v; want output %q", res, err, "# example\n")
	}
	if after := f.tree(); !reflect.DeepEqual(before, after) {
		t.Errorf("Preview wrote to disk: %v -> %v", before, after)
	}
}

func TestPreviewUsesTheStackDataWhenAsked(t *testing.T) {
	f := newFixture(t)
	draft := []byte("{{if .Stack}}stack={{.Stack}}{{else}}no stack{{end}}")
	if res, _ := f.cat.Preview(sample, draft, false); res.Output != "no stack" {
		t.Errorf("empty stack output = %q, want %q", res.Output, "no stack")
	}
	if res, _ := f.cat.Preview(sample, draft, true); res.Output != "stack=example" {
		t.Errorf("filled stack output = %q, want %q", res.Output, "stack=example")
	}
}

func TestPreviewReportsTheLineOfARenderError(t *testing.T) {
	f := newFixture(t)
	res, err := f.cat.Preview(sample, []byte("ok\n{{.Stak}}\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Err == nil || res.Err.Line != 2 || res.Err.Message == "" {
		t.Fatalf("Err = %+v, want a RenderError on line 2 with a message", res.Err)
	}
	if res.Output != "" {
		t.Errorf("Output = %q, want empty when the render fails", res.Output)
	}
}

func TestPreviewStopsAtTheOutputCeiling(t *testing.T) {
	f := newFixture(t)
	// Cada nível chama o anterior duas vezes: 2^24 execuções, longe do teto de
	// profundidade, e a saída passa dos 4 MiB muito antes do fim.
	var b strings.Builder
	b.WriteString(`{{define "l0"}}xxxxxxxxxx{{end}}`)
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&b, `{{define "l%d"}}{{template "l%d"}}{{template "l%d"}}{{end}}`, i, i-1, i-1)
	}
	b.WriteString(`{{template "l24"}}`)
	res, err := f.cat.Preview(sample, []byte(b.String()), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Message, "ceiling") {
		t.Errorf("Err = %+v, want the output ceiling error", res.Err)
	}
}

func TestPreviewRejectsANameOutsideTheList(t *testing.T) {
	f := newFixture(t)
	for _, n := range []string{"../x.tmpl", "/etc/passwd", "nope.tmpl"} {
		if _, err := f.cat.Preview(n, []byte("x"), false); !errors.Is(err, ErrUnknown) {
			t.Errorf("Preview(%q) error = %v, want ErrUnknown", n, err)
		}
	}
}

func TestInfoNormalizesCRLFAndReportsIt(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "a\r\nb\r\n")
	info, err := f.cat.Info(sample)
	if err != nil || info.EOL != "crlf" || info.Content != "a\nb\n" || !info.UTF8 {
		t.Fatalf("Info = %+v, %v; want eol=crlf content=%q utf8", info, err, "a\nb\n")
	}
	f.overlay(sample, "a\nb\n")
	if info, _ := f.cat.Info(sample); info.EOL != "lf" {
		t.Errorf("EOL = %q, want lf", info.EOL)
	}
}

func TestInfoFlagsInvalidUTF8(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "ok \xff\xfe")
	info, err := f.cat.Info(sample)
	if err != nil || info.UTF8 {
		t.Errorf("Info = %+v, %v; want UTF8=false for invalid bytes", info, err)
	}
}

func TestInfoFlagsHooksAsReadOnly(t *testing.T) {
	f := newFixture(t)
	info, err := f.cat.Info(hook)
	if err != nil || !info.ReadOnly {
		t.Errorf("Info(hook) = %+v, %v; want ReadOnly", info, err)
	}
	if info, _ := f.cat.Info(sample); info.ReadOnly {
		t.Errorf("Info(%s).ReadOnly = true, want false for a text template", sample)
	}
	if !IsHook(hook) || IsHook(sample) {
		t.Errorf("IsHook(%q)=%v IsHook(%q)=%v, want true/false", hook, IsHook(hook), sample, IsHook(sample))
	}
}

func TestInfoHashIsTheHashOfWhatTheNextInitAIWouldUse(t *testing.T) {
	f := newFixture(t)
	raw, err := f.cat.Read(sample)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	info, err := f.cat.Info(sample)
	if err != nil || info.Hash != hex.EncodeToString(sum[:]) {
		t.Errorf("Info.Hash = %q, want the sha256 of Read: %q (%v)", info.Hash, hex.EncodeToString(sum[:]), err)
	}
}

func TestInfoRejectsANameOutsideTheList(t *testing.T) {
	f := newFixture(t)
	if _, err := f.cat.Info("../x.tmpl"); !errors.Is(err, ErrUnknown) {
		t.Errorf("Info error = %v, want ErrUnknown", err)
	}
}

package scaffold

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderStringRendersAnInMemoryTemplate(t *testing.T) {
	out, err := RenderString("x.tmpl", []byte("# {{.ProjectName}}\n"), Data{ProjectName: "demo"}, 0)
	if err != nil || string(out) != "# demo\n" {
		t.Errorf("RenderString = %q, %v; want %q", out, err, "# demo\n")
	}
}

func TestRenderStringStopsAtTheOutputCeiling(t *testing.T) {
	_, err := RenderString("x.tmpl", []byte(strings.Repeat("a", 100)), Data{}, 10)
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Errorf("error = %v, want ErrOutputTooLarge", err)
	}
	if _, err := RenderString("x.tmpl", []byte(strings.Repeat("a", 10)), Data{}, 10); err != nil {
		t.Errorf("output exactly at the ceiling failed: %v", err)
	}
}

func TestRenderStringReportsAParseErrorWithItsName(t *testing.T) {
	_, err := RenderString("x.tmpl", []byte("{{"), Data{}, 0)
	if err == nil || !strings.Contains(err.Error(), "x.tmpl") {
		t.Errorf("error = %v, want one naming x.tmpl", err)
	}
}

package linediff

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestUnifiedEqualTextsGiveNoDiff(t *testing.T) {
	for _, s := range []string{"", "a\n", "a\nb", "a\r\nb\r\n"} {
		got, err := Unified(s, s, "a", "b")
		if err != nil || got != "" {
			t.Errorf("Unified(%q, %q) = %q, %v; want empty", s, s, got, err)
		}
	}
}

func TestUnifiedExactOutput(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	b := "1\n2\n3\n4\nfive\n6\n7\n8\n9\n10\n"
	got, err := Unified(a, b, "embedded/x", "overlay/x")
	if err != nil {
		t.Fatal(err)
	}
	want := "--- embedded/x\n+++ overlay/x\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n"
	if got != want {
		t.Errorf("Unified() =\n%s\nwant\n%s", got, want)
	}
}

func TestUnifiedOneEmptySide(t *testing.T) {
	got, _ := Unified("", "a\nb\n", "x", "y")
	if want := "--- x\n+++ y\n@@ -0,0 +1,2 @@\n+a\n+b\n"; got != want {
		t.Errorf("empty→text =\n%q\nwant\n%q", got, want)
	}
	got, _ = Unified("a\nb\n", "", "x", "y")
	if want := "--- x\n+++ y\n@@ -1,2 +0,0 @@\n-a\n-b\n"; got != want {
		t.Errorf("text→empty =\n%q\nwant\n%q", got, want)
	}
}

func TestUnifiedMissingFinalNewlineIsADifference(t *testing.T) {
	got, _ := Unified("x", "x\n", "a", "b")
	want := "--- a\n+++ b\n@@ -1 +1 @@\n-x\n\\ No newline at end of file\n+x\n"
	if got != want {
		t.Errorf("Unified() =\n%q\nwant\n%q", got, want)
	}
}

func TestUnifiedCarriageReturnIsContent(t *testing.T) {
	got, _ := Unified("a\r\nb\r\n", "a\nb\n", "x", "y")
	if !strings.Contains(got, "-a\r\n") || !strings.Contains(got, "+a\n") {
		t.Errorf("CRLF vs LF should differ line by line, got %q", got)
	}
}

func TestUnifiedTooLarge(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	if _, err := Unified(a.String(), b.String(), "x", "y"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

// apply aplica um diff unificado a a e devolve o resultado: se reconstrói b,
// o diff está certo, seja qual for o formato dos hunks.
func apply(t *testing.T, a, diff string) string {
	t.Helper()
	src := strings.SplitAfter(a, "\n")
	if len(src) > 0 && src[len(src)-1] == "" {
		src = src[:len(src)-1]
	}
	var out []string
	pos := 0
	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "--- "), strings.HasPrefix(l, "+++ "):
		case strings.HasPrefix(l, "@@"):
			var as, al, bs, bl int
			rest := strings.TrimPrefix(l, "@@ -")
			parts := strings.Fields(rest)
			as, al = parseRange(t, parts[0])
			bs, bl = parseRange(t, strings.TrimPrefix(parts[1], "+"))
			_, _ = bs, bl
			start := as - 1
			if al == 0 {
				start = as
			}
			out = append(out, src[pos:start]...)
			pos = start
		case strings.HasPrefix(l, "\\ "):
			last := len(out) - 1
			if last >= 0 && strings.HasSuffix(out[last], "\n") && !(i > 0 && strings.HasPrefix(lines[i-1], "-")) {
				out[last] = strings.TrimSuffix(out[last], "\n")
			}
		case l[0] == ' ':
			out = append(out, src[pos])
			pos++
		case l[0] == '-':
			pos++
		case l[0] == '+':
			out = append(out, l[1:]+"\n")
		}
	}
	out = append(out, src[pos:]...)
	return strings.Join(out, "")
}

func parseRange(t *testing.T, s string) (start, n int) {
	t.Helper()
	p := strings.SplitN(s, ",", 2)
	start, _ = strconv.Atoi(p[0])
	n = 1
	if len(p) == 2 {
		n, _ = strconv.Atoi(p[1])
	}
	return start, n
}

func TestUnifiedApplyingTheDiffRebuildsTheOtherSide(t *testing.T) {
	cases := [][2]string{
		{"a\nb\nc\n", "a\nB\nc\n"},
		{"a\nb\nc\n", "c\n"},
		{"", "x\ny\n"},
		{"x\ny\n", ""},
		{"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", "1\n2\nX\n4\n5\n6\n7\n8\n9\n10\nY\n12\n"},
		{"a\nb\n", "a\nb\nc"},
		{"a\nb", "a\nb\n"},
		{"all\ndifferent\n", "totally\nnew\nstuff\n"},
		{"a\n\nb\n", "a\nb\n"},
		{"a\nb", "a\nc"},
		{"x", "y"},
	}
	for _, c := range cases {
		d, err := Unified(c[0], c[1], "a", "b")
		if err != nil {
			t.Fatal(err)
		}
		if got := apply(t, c[0], d); got != c[1] {
			t.Errorf("apply(%q, diff) = %q, want %q\ndiff:\n%s", c[0], got, c[1], d)
		}
	}
}

// Duas mudanças separadas por até 2×contexto linhas iguais saem num hunk só;
// com uma linha a mais, em dois.
func TestUnifiedMergesHunksOnlyWhenTheirContextsTouch(t *testing.T) {
	build := func(gap int) (string, string) {
		var a, b strings.Builder
		a.WriteString("A\n")
		b.WriteString("a\n")
		for i := 0; i < gap; i++ {
			fmt.Fprintf(&a, "m%d\n", i)
			fmt.Fprintf(&b, "m%d\n", i)
		}
		a.WriteString("Z\n")
		b.WriteString("z\n")
		return a.String(), b.String()
	}
	for gap, wantHunks := range map[int]int{6: 1, 7: 2} {
		a, b := build(gap)
		d, err := Unified(a, b, "x", "y")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(d, "@@ -"); got != wantHunks {
			t.Errorf("gap %d: %d hunks, want %d\n%s", gap, got, wantHunks, d)
		}
		if got := apply(t, a, d); got != b {
			t.Errorf("gap %d: apply did not rebuild b", gap)
		}
	}
}

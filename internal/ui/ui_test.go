package ui

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

const (
	esc   = "\x1b"
	reset = esc + "[0m"
)

func green(s string) string  { return esc + "[32m" + s + reset }
func yellow(s string) string { return esc + "[33m" + s + reset }
func red(s string) string    { return esc + "[31m" + s + reset }
func cyan(s string) string   { return esc + "[36m" + s + reset }
func gray(s string) string   { return esc + "[90m" + s + reset }
func bold(s string) string   { return esc + "[1m" + s + esc + "[22m" }

// out roda f sobre um Printer e devolve o que foi escrito.
func out(color bool, f func(p *Printer)) string {
	var b bytes.Buffer
	f(New(&b, color))
	return b.String()
}

func TestTitleAndSection(t *testing.T) {
	title := func(p *Printer) { p.Title("init ai", "demo") }
	if got, want := out(false, title), "init ai · demo\n"; got != want {
		t.Errorf("plain Title = %q, want %q", got, want)
	}
	if got, want := out(true, title), bold("init ai")+" "+gray("· demo")+"\n"; got != want {
		t.Errorf("color Title = %q, want %q", got, want)
	}
	noDetail := func(p *Printer) { p.Title("ray", "") }
	if got, want := out(true, noDetail), bold("ray")+"\n"; got != want {
		t.Errorf("Title without detail = %q, want %q", got, want)
	}
	section := func(p *Printer) { p.Section("Components") }
	if got, want := out(false, section), "Components\n"; got != want {
		t.Errorf("plain Section = %q", got)
	}
	if got, want := out(true, section), bold("Components")+"\n"; got != want {
		t.Errorf("color Section = %q, want %q", got, want)
	}
}

func TestItemsEveryKindPlainAndColored(t *testing.T) {
	items := []Item{
		{OK, "s", ""},
		{Fail, "gone", "not found"},
		{Warn, "w", "careful"},
		{Added, "CLAUDE.md", ""},
		{Updated, "t", "updated"},
		{Kept, "u", "kept"},
	}
	plain := "" +
		"  ✓ s\n" +
		"  ✗ gone       not found\n" +
		"  ⚠ w          careful\n" +
		"  + CLAUDE.md\n" +
		"  ~ t          updated\n" +
		"  = u          kept\n"
	if got := out(false, func(p *Printer) { p.Items(items) }); got != plain {
		t.Errorf("plain Items =\n%q\nwant\n%q", got, plain)
	}
	colored := "" +
		"  " + green("✓") + " s\n" +
		"  " + red("✗") + " " + red("gone") + "       " + gray("not found") + "\n" +
		"  " + yellow("⚠") + " w          " + gray("careful") + "\n" +
		"  " + green("+") + " CLAUDE.md\n" +
		"  " + yellow("~") + " t          " + gray("updated") + "\n" +
		"  " + gray("=") + " " + gray("u") + "          " + gray("kept") + "\n"
	if got := out(true, func(p *Printer) { p.Items(items) }); got != colored {
		t.Errorf("color Items =\n%q\nwant\n%q", got, colored)
	}
}

// O preenchimento usa a largura visível: um nome com acento ocupa uma coluna
// por letra, não um por byte.
func TestItemsPadByVisibleWidth(t *testing.T) {
	got := out(false, func(p *Printer) {
		p.Items([]Item{{OK, "ação", "a"}, {OK, "ab", "b"}})
	})
	want := "  ✓ ação  a\n  ✓ ab    b\n"
	if got != want {
		t.Errorf("Items = %q, want %q", got, want)
	}
}

func TestSummarySkipsZeroAndKeepsTheGivenOrder(t *testing.T) {
	counts := []Count{
		{1, "failed", Red}, {0, "installed", Green}, {3, "created", Green}, {1, "kept", Gray},
	}
	if got, want := out(false, func(p *Printer) { p.Summary(counts...) }), "1 failed · 3 created · 1 kept\n"; got != want {
		t.Errorf("plain Summary = %q, want %q", got, want)
	}
	sep := gray(" · ")
	want := red("1 failed") + sep + green("3 created") + sep + gray("1 kept") + "\n"
	if got := out(true, func(p *Printer) { p.Summary(counts...) }); got != want {
		t.Errorf("color Summary = %q, want %q", got, want)
	}
	if got := out(true, func(p *Printer) { p.Summary(Count{0, "x", Green}) }); got != "" {
		t.Errorf("an all-zero Summary must print nothing, got %q", got)
	}
}

func TestTablePlainAndColoredHaveTheSameVisibleColumns(t *testing.T) {
	header := []string{"NAME", "STATE"}
	rows := [][]Cell{
		{{"CLAUDE.md.tmpl", Plain}, {"outdated", Yellow}},
		{{"a", Plain}, {"new", Green}},
	}
	plain := "NAME            STATE\nCLAUDE.md.tmpl  outdated\na               new\n"
	if got := out(false, func(p *Printer) { p.Table(header, rows) }); got != plain {
		t.Errorf("plain Table =\n%q\nwant\n%q", got, plain)
	}
	colored := out(true, func(p *Printer) { p.Table(header, rows) })
	want := gray("NAME") + "            " + gray("STATE") + "\n" +
		"CLAUDE.md.tmpl  " + yellow("outdated") + "\n" +
		"a               " + green("new") + "\n"
	if colored != want {
		t.Errorf("color Table =\n%q\nwant\n%q", colored, want)
	}
	if stripped := stripANSI(colored); stripped != plain {
		t.Errorf("stripping the escapes from the colored Table must give the plain one:\n%q\n%q", stripped, plain)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestDiffLineColorsByPrefix(t *testing.T) {
	cases := []struct{ line, want string }{
		{"--- embedded/x", gray("--- embedded/x")},
		{"+++ overlay/x", gray("+++ overlay/x")},
		{"@@ -1,2 +1,3 @@", cyan("@@ -1,2 +1,3 @@")},
		{"+added", green("+added")},
		{"-removed", red("-removed")},
		{" context", " context"},
	}
	for _, c := range cases {
		if got := out(true, func(p *Printer) { p.DiffLine(c.line) }); got != c.want+"\n" {
			t.Errorf("DiffLine(%q) = %q, want %q", c.line, got, c.want+"\n")
		}
		if got := out(false, func(p *Printer) { p.DiffLine(c.line) }); got != c.line+"\n" {
			t.Errorf("plain DiffLine(%q) = %q", c.line, got)
		}
	}
}

func TestErrorMarksOnlyTheFirstLineAndTrimsTheTrailingNewline(t *testing.T) {
	msg := "saved, but it does not render:\n  parse error\nfix it with `ray template edit x`\n"
	if got, want := out(false, func(p *Printer) { p.Error(msg) }), "✗ saved, but it does not render:\n  parse error\nfix it with `ray template edit x`\n"; got != want {
		t.Errorf("plain Error = %q, want %q", got, want)
	}
	want := red("✗ saved, but it does not render:") + "\n  parse error\nfix it with `ray template edit x`\n"
	if got := out(true, func(p *Printer) { p.Error(msg) }); got != want {
		t.Errorf("color Error = %q, want %q", got, want)
	}
	if got, want := out(false, func(p *Printer) { p.Error("one line\n\n") }), "✗ one line\n"; got != want {
		t.Errorf("Error must trim every trailing newline, got %q", got)
	}
}

// Write escreve os bytes recebidos sem tocar neles, com ou sem cor.
func TestWriteIsTransparent(t *testing.T) {
	for _, color := range []bool{false, true} {
		var b bytes.Buffer
		p := New(&b, color)
		payload := "+ mkdir -p /x\n\x1b[31mnot ours\x1b[0m\n"
		n, err := p.Write([]byte(payload))
		if err != nil || n != len(payload) || b.String() != payload {
			t.Errorf("color=%v: Write changed the bytes: n=%d err=%v got=%q", color, n, err, b.String())
		}
	}
}

func TestForReusesAPrinterAndRawUnwrapsIt(t *testing.T) {
	var b bytes.Buffer
	p := New(&b, true)
	if For(p) != p {
		t.Error("For(*Printer) must return the same Printer")
	}
	plain := For(&b)
	if plain.Color() || plain.w != &b {
		t.Errorf("For(buffer) = color %v, want a plain Printer over the buffer", plain.Color())
	}
	if Raw(p) != &b {
		t.Error("Raw(*Printer) must return the writer underneath")
	}
	if Raw(&b) != &b {
		t.Error("Raw(non-Printer) must return the writer itself")
	}
	if New(p, false).w != &b {
		t.Error("New over a Printer must not stack Printers")
	}
}

// Guarda: método novo no Printer obriga a decidir se ele emite escape e a
// cobri-lo aqui. Quem emite não pode emitir ESC sem cor.
func TestEveryEmittingMethodIsPlainWithoutColor(t *testing.T) {
	emitters := map[string]func(p *Printer){
		"Line":    func(p *Printer) { p.Line("x") },
		"Blank":   func(p *Printer) { p.Blank() },
		"Title":   func(p *Printer) { p.Title("a", "b") },
		"Section": func(p *Printer) { p.Section("a") },
		"Items": func(p *Printer) {
			p.Items([]Item{{OK, "a", "b"}, {Fail, "c", "d"}, {Warn, "e", ""}, {Added, "f", ""}, {Updated, "g", ""}, {Kept, "h", ""}})
		},
		"Summary":  func(p *Printer) { p.Summary(Count{1, "a", Red}, Count{2, "b", Green}) },
		"Table":    func(p *Printer) { p.Table([]string{"A", "B"}, [][]Cell{{{"x", Red}, {"y", Green}}}) },
		"DiffLine": func(p *Printer) { p.DiffLine("+x"); p.DiffLine("-y"); p.DiffLine("@@ z") },
		"Error":    func(p *Printer) { p.Error("boom\nmore") },
		"Result":   func(p *Printer) { p.Result(OK, "a", "b"); p.Result(Fail, "c", "d") },
		"Warning":  func(p *Printer) { p.Warning("a", "b") },
		"Verdict":  func(p *Printer) { p.Verdict(OK, "ready", ""); p.Verdict(Fail, "2 problems", "fix them") },
		"Steps":    func(p *Printer) { p.Steps([][2]string{{"claude", "start"}, {"ray status", "diagnose"}}) },
	}
	// Os que devolvem texto ou só leem estado, e o Write transparente.
	others := map[string]bool{"Write": true, "Color": true, "Paint": true, "Bold": true}

	typ := reflect.TypeOf(&Printer{})
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if _, ok := emitters[name]; !ok && !others[name] {
			t.Errorf("Printer.%s is not covered by this guard: add it to emitters (or others, if it emits nothing)", name)
		}
	}
	for name, f := range emitters {
		if got := out(false, f); strings.Contains(got, esc) {
			t.Errorf("%s emitted an escape without color: %q", name, got)
		}
	}
	p := New(&bytes.Buffer{}, false)
	if p.Paint(Red, "x") != "x" || p.Bold("x") != "x" {
		t.Error("Paint and Bold must return the text untouched without color")
	}
}

func TestResultWarningVerdictAndSteps(t *testing.T) {
	cases := []struct {
		name         string
		f            func(p *Printer)
		plain, color string
	}{
		{"Result ok", func(p *Printer) { p.Result(OK, "CLAUDE.md.tmpl", "restored") },
			"✓ CLAUDE.md.tmpl  restored\n", green("✓") + " CLAUDE.md.tmpl  " + gray("restored") + "\n"},
		{"Result kept without detail", func(p *Printer) { p.Result(Kept, "x", "") },
			"= x\n", gray("=") + " x\n"},
		{"Result fail", func(p *Printer) { p.Result(Fail, "gone", "missing") },
			"✗ gone  missing\n", red("✗") + " " + red("gone") + "  " + gray("missing") + "\n"},
		{"Warning with hint", func(p *Printer) { p.Warning("s: edited locally", "`ray update` will preserve it") },
			"⚠ s: edited locally\n    `ray update` will preserve it\n",
			yellow("⚠") + " s: edited locally\n    " + gray("`ray update` will preserve it") + "\n"},
		{"Warning without hint", func(p *Printer) { p.Warning("careful", "") },
			"⚠ careful\n", yellow("⚠") + " careful\n"},
		{"Verdict ok", func(p *Printer) { p.Verdict(OK, "ready", "") },
			"✓ ready\n", green("✓ ready") + "\n"},
		{"Verdict fail with detail", func(p *Printer) { p.Verdict(Fail, "2 problems", "`ray init ai` would refuse them") },
			"✗ 2 problems — `ray init ai` would refuse them\n",
			red("✗ 2 problems") + " " + gray("— `ray init ai` would refuse them") + "\n"},
		{"Steps", func(p *Printer) {
			p.Steps([][2]string{{"claude", "start the session"}, {"ray status", "diagnose the environment"}})
		},
			"  claude      start the session\n  ray status  diagnose the environment\n",
			"  " + cyan("claude") + "      start the session\n  " + cyan("ray status") + "  diagnose the environment\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := out(false, c.f); got != c.plain {
				t.Errorf("plain = %q, want %q", got, c.plain)
			}
			if got := out(true, c.f); got != c.color {
				t.Errorf("color = %q, want %q", got, c.color)
			}
		})
	}
}

func TestStepsWithoutDescriptionHaveNoTrailingSpace(t *testing.T) {
	got := out(false, func(p *Printer) { p.Steps([][2]string{{"cd proj", ""}, {"git add .claude", ""}, {"claude", "start"}}) })
	want := "  cd proj\n  git add .claude\n  claude           start\n"
	if got != want {
		t.Errorf("Steps = %q, want %q", got, want)
	}
}

func TestShortenHome(t *testing.T) {
	cases := []struct{ in, home, want string }{
		{"not found at /home/me/.ray/components/gone", "/home/me", "not found at ~/.ray/components/gone"},
		{"/home/me", "/home/me", "~"},
		{"/home/mellon/x", "/home/me", "/home/mellon/x"},
		{`C:\Users\me\.ray\x`, `C:\Users\me`, `~\.ray\x`},
		{"/home/me/a and /home/me/b", "/home/me", "~/a and ~/b"},
		{"nothing to do", "/home/me", "nothing to do"},
		{"/home/me/x", "", "/home/me/x"},
	}
	for _, c := range cases {
		if got := ShortenHome(c.in, c.home); got != c.want {
			t.Errorf("ShortenHome(%q, %q) = %q, want %q", c.in, c.home, got, c.want)
		}
	}
}

// A tabela alinha pela largura visível: um nome com acento ocupa uma coluna por
// letra, e uma última célula vazia não deixa espaço no fim da linha.
func TestTableAlignsAccentedNamesAndTrimsAnEmptyLastCell(t *testing.T) {
	header := []string{"NAME", "FOUND", "HINT"}
	rows := [][]Cell{
		{{"ação", Plain}, {"yes", Green}, {"", Plain}},
		{{"ab", Plain}, {"no", Red}, {"install it", Plain}},
	}
	want := "NAME  FOUND  HINT\nação  yes\nab    no     install it\n"
	if got := out(false, func(p *Printer) { p.Table(header, rows) }); got != want {
		t.Errorf("plain Table = %q, want %q", got, want)
	}
	colored := out(true, func(p *Printer) { p.Table(header, rows) })
	if stripped := stripANSI(colored); stripped != want {
		t.Errorf("stripping the escapes must give the plain Table:\n%q\n%q", stripped, want)
	}
}

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/report"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/ui"
)

func resetInitAIFlags(t *testing.T) {
	t.Helper()
	prevProfile := flagProfile
	prevForce := flagForce
	prevNoGlobal, prevReinstall := flagNoGlobal, flagReinstallGlobal
	prevDryRun := flagDryRun
	t.Cleanup(func() {
		flagProfile = prevProfile
		flagForce = prevForce
		flagNoGlobal, flagReinstallGlobal = prevNoGlobal, prevReinstall
		flagDryRun = prevDryRun
	})
}

func TestBuildInitAIOptionsMapsFlags(t *testing.T) {
	resetInitAIFlags(t)
	flagProfile = "go"
	flagForce = true
	flagNoGlobal = true
	flagReinstallGlobal = true
	flagDryRun = true

	opts := buildInitAIOptions("/tmp/project", &bytes.Buffer{})

	if opts.Profile != "go" || opts.Target != "/tmp/project" {
		t.Fatalf("opts = %+v, want Profile=go Target=/tmp/project", opts)
	}
	if !opts.Force || !opts.NoGlobal || !opts.ReinstallGlobal || !opts.DryRun {
		t.Fatalf("opts = %+v, want all bool flags true", opts)
	}
}

func TestInitAiRejectsRemovedLevelFlag(t *testing.T) {
	// Flag de uma versão anterior do modo learn, já removida antes do modo
	// learn inteiro sair. Aceitar a flag calada faria o ray prometer de novo
	// um comportamento que ele não tem.
	// Atenção ao nome: o construtor é newInitAICmd, com AI maiúsculo.
	c := newInitAICmd()
	c.SetArgs([]string{"--level", "beginner", t.TempDir()})
	// bytes.Buffer e não io.Discard: o pacote de teste já importa bytes e não
	// importa io.
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})

	err := c.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want erro de flag desconhecida")
	}
	if !strings.Contains(err.Error(), "level") {
		t.Errorf("erro = %v, want mencionar a flag level", err)
	}
}

// Editado — RF-07 seguiu além do erro amigável: init ai passou a ser de fato
// independente de profile, caindo no perfil `base` em vez de exigir escolha.
// TestBuildInitAIOptionsDefaultsProfileToBase cobre o novo comportamento.
func TestBuildInitAIOptionsDefaultsProfileToBase(t *testing.T) {
	resetInitAIFlags(t)
	flagProfile = ""

	opts := buildInitAIOptions("/tmp/project", &bytes.Buffer{})

	if opts.Profile != "base" {
		t.Errorf("opts.Profile = %q, want %q when --profile is omitted", opts.Profile, "base")
	}
}

func TestRunInitAIPrintsSummaryAndErrorsOnFailure(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prof := &profile.Profile{
		Name:       "test",
		Components: []profile.Component{{Name: "s", Dest: ".claude/skills"}},
		Scaffold:   profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "CLAUDE.md"}}},
	}
	data, err := yaml.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "test.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	home := initai.Home{
		ProfilesDir:  profilesDir,
		TemplatesDir: filepath.Join(base, "templates"),
		ConfigPath:   filepath.Join(base, "config.yaml"),
		StatePath:    filepath.Join(base, "state.yaml"),
		StoreDir:     filepath.Join(base, "store"),
		// ComponentsDir sem "s": é assim que o componente falha agora, sem rede.
		ComponentsDir: filepath.Join(base, "components"),
	}
	target := t.TempDir()

	l := stubLooker{"npx": true, "node": true}
	fr := &runner.FakeRunner{}
	var out bytes.Buffer
	opts := initai.Options{Profile: "test", Target: target, Out: &out}

	err = runInitAI(fr, l, opts, home, &out)
	if err == nil {
		t.Fatal("runInitAI() = nil error, want error when a component fails")
	}
	got := out.String()
	if !strings.Contains(got, "✗ s  not found at") || !strings.Contains(got, "1 failed") {
		t.Errorf("output = %q, want the failed component line and the failed count", got)
	}
}

func TestPrintInitAISummaryFooterInsideGitRepo(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Entries: []report.Entry{
			{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
			{Kind: report.File, Name: ".claude/x", Outcome: report.Created},
			{Kind: report.File, Name: ".mcp.json", Outcome: report.Created},
		},
		VersionedPaths: []string{".claude", ".mcp.json", "CLAUDE.md"},
		InGitRepo:      true,
	})
	got := out.String()
	for _, want := range []string{
		"Next steps",
		"git add .claude .mcp.json CLAUDE.md",
		"git commit -m",
		"claude",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary = %q, want it to contain %q", got, want)
		}
	}
	// O proibido leva o fim de linha junto: "git add .claude" contém
	// "git add ." como substring, e sem a âncora a checagem daria falso positivo.
	for _, forbidden := range []string{"git add -A", "git add --all", "git add .\n"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("summary suggests blind %q, which guard-add.sh warns against", forbidden)
		}
	}
}

func TestPrintInitAISummaryFooterOutsideGitRepo(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		VersionedPaths: []string{".claude", "CLAUDE.md"},
		InGitRepo:      false,
	})
	got := out.String()
	if strings.Contains(got, "git ") {
		t.Errorf("summary = %q, want no git advice outside a repo", got)
	}
	if !strings.Contains(got, "claude") {
		t.Errorf("summary = %q, want it to still suggest running claude", got)
	}
}

func TestPrintInitAISummaryNoFooterOnFailure(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Entries:        []report.Entry{{Kind: report.Component, Name: "some/component", Outcome: report.Failed, Detail: "boom"}},
		VersionedPaths: []string{".claude"},
		InGitRepo:      true,
		HadFailure:     true,
	})
	if strings.Contains(out.String(), "Next steps") {
		t.Error("summary shows next steps after a failure; the environment is half-written")
	}
}

// Numa pasta nova o índice do graphify não tem o que indexar. O comando termina
// sem erro e o rodapé com os próximos passos aparece — é o que a pessoa precisa
// para commitar o ambiente que acabou de nascer.
func TestRunInitAIOnAnEmptyFolderSucceedsAndShowsNextSteps(t *testing.T) {
	home := newTestHome(t)
	// CodeGraph ligado: é o que faz o plano incluir `graphify update .`. Sem ele
	// o comando nunca rodaria e o teste não exercitaria o caminho.
	p := newTestProfile(nil)
	p.Integrations = profile.Integrations{CodeGraph: true}
	writeTestProfile(t, home.ProfilesDir, p)
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"graphify update .": {ExitCode: 1}}}
	var out bytes.Buffer
	opts := initai.Options{Profile: "test", Target: t.TempDir(), NoGlobal: true, Out: &out}

	if err := runInitAI(fr, allFound, opts, home, &out); err != nil {
		t.Fatalf("runInitAI() error = %v, want nil: %s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "\nNext steps\n") {
		t.Errorf("output = %q, want the next-steps footer", got)
	}
	if !strings.Contains(got, "⚠ ") || !strings.Contains(got, "graphify update .") {
		t.Errorf("output = %q, want a ⚠ warning about `graphify update .`", got)
	}
	assertNoEscape(t, got)
}

// Os próximos passos (`git add`, `claude`) valem DENTRO do projeto. Quando o
// alvo não é o diretório atual — o caso de `ray new go app` — o rodapé começa
// com o `cd`; sem isso o `git add` sugerido roda na pasta errada.
func TestNextStepsStartWithCdWhenTheTargetIsNotTheCurrentDir(t *testing.T) {
	cases := []struct {
		name   string
		target func(base string) string
		wantCd string // "" = não deve haver cd
	}{
		{"child of the current dir", func(b string) string { return filepath.Join(b, "app") }, "cd app"},
		{"current dir itself", func(b string) string { return b }, ""},
		{"sibling outside the current dir", func(b string) string { return filepath.Join(filepath.Dir(b), "elsewhere") }, "cd "},
		{"name with spaces is quoted", func(b string) string { return filepath.Join(b, "my app") }, `cd "my app"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			t.Chdir(base)
			var out bytes.Buffer
			printInitAISummary(&out, "init ai", initai.Summary{
				Entries:        []report.Entry{{Kind: report.File, Name: ".claude/.ray-profile", Outcome: report.Created}},
				VersionedPaths: []string{".claude"},
				InGitRepo:      true,
				Target:         tc.target(base),
			})
			got := out.String()
			cdAt := strings.Index(got, "  cd ")
			if tc.wantCd == "" {
				if cdAt >= 0 {
					t.Errorf("output = %q, want no `cd` line", got)
				}
				return
			}
			if !strings.Contains(got, "  "+tc.wantCd) {
				t.Errorf("output = %q, want a line starting with %q", got, "  "+tc.wantCd)
			}
			if addAt := strings.Index(got, "  git add"); cdAt < 0 || addAt < cdAt {
				t.Errorf("output = %q, want the cd BEFORE the git add", got)
			}
		})
	}
}

// Sem alvo conhecido (resumo montado por quem não o informa), nada de cd.
func TestNextStepsHaveNoCdWithoutAKnownTarget(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{VersionedPaths: []string{".claude"}, InGitRepo: true})
	if strings.Contains(out.String(), "  cd ") {
		t.Errorf("output = %q, want no cd when the target is unknown", out.String())
	}
}

// A tela aprovada do init ai: seções na ordem Tools, Components, Files; o
// motivo da falha à direita; o resumo na ordem fixa, e sem Next steps porque
// algo falhou.
func TestPrintInitAISummaryShowsSectionsItemsAndTheSummary(t *testing.T) {
	fixHome(t, "")
	src := filepath.Join("components", "gone")
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "s", Outcome: report.Installed},
			{Kind: report.Component, Name: "gone", Outcome: report.Failed, Detail: "not found at " + src},
			{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
			{Kind: report.File, Name: ".claude/settings.json", Outcome: report.Created},
			{Kind: report.File, Name: ".gitignore", Outcome: report.Created},
			{Kind: report.File, Name: ".claude/handoff.md", Outcome: report.Kept, Detail: "already there"},
		},
		HadFailure:     true,
		VersionedPaths: []string{".claude", "CLAUDE.md"},
		InGitRepo:      true,
	})
	// O nome mais largo da seção Files é ".claude/settings.json" (21 colunas):
	// o detalhe vem depois do preenchimento até ele, mais 2 espaços.
	want := "init ai · demo\n" +
		"\n" +
		"Components\n" +
		"  ✓ s\n" +
		"  ✗ gone  not found at " + src + "\n" +
		"\n" +
		"Files\n" +
		"  + CLAUDE.md\n" +
		"  + .claude/settings.json\n" +
		"  + .gitignore\n" +
		"  = .claude/handoff.md" + strings.Repeat(" ", 21-18+2) + "already there\n" +
		"\n" +
		"1 failed · 1 installed · 3 created · 1 kept\n"
	if got := out.String(); got != want {
		t.Errorf("summary =\n%q\nwant\n%q", got, want)
	}
	assertNoEscape(t, out.String())
}

// Globais e comandos por projeto são a seção Tools, que vem antes das demais;
// uma seção sem item não aparece.
func TestPrintInitAISummaryListsToolsFirstAndOmitsEmptySections(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
			{Kind: report.Global, Name: "headroom", Outcome: report.Installed},
			{Kind: report.Global, Name: "uv", Outcome: report.Failed, Detail: "`uv tool install x`: exit 1"},
			{Kind: report.Command, Name: "graphify update .", Outcome: report.Installed},
		},
		HadFailure: true,
	})
	want := "init ai · demo\n" +
		"\n" +
		"Tools\n" +
		"  ✓ headroom\n" +
		"  ✗ uv" + strings.Repeat(" ", 17-2+2) + "`uv tool install x`: exit 1\n" +
		"  ✓ graphify update .\n" +
		"\n" +
		"Files\n" +
		"  + CLAUDE.md\n" +
		"\n" +
		"1 failed · 2 installed · 1 created\n"
	if got := out.String(); got != want {
		t.Errorf("summary =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(out.String(), "Components") {
		t.Errorf("summary = %q, want no Components section when there is no component", out.String())
	}
}

// O caminho da home vira ~ só na exibição, no motivo de um item.
func TestPrintInitAISummaryAbbreviatesTheHomeInTheDetail(t *testing.T) {
	home := t.TempDir()
	fixHome(t, home)
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "gone", Outcome: report.Failed, Detail: "not found at " + filepath.Join(home, "components", "gone")},
		},
		HadFailure: true,
	})
	want := "  ✗ gone  not found at ~" + string(filepath.Separator) + filepath.Join("components", "gone") + "\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("summary = %q, want the line %q", out.String(), want)
	}
	if strings.Contains(out.String(), home) {
		t.Errorf("summary = %q, must not show the full home path", out.String())
	}
}

// Componente editado e preservado: o motivo é cortado em "edited locally", a
// linha ganha "kept —" e a dica do --force aparece uma vez, depois das
// seções e antes do resumo.
func TestPrintInitAISummaryExplainsAKeptComponentOnce(t *testing.T) {
	reason := "edited locally (differs from last pristine); use --force to overwrite"
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "u", Outcome: report.Kept, Detail: reason},
			{Kind: report.Component, Name: "v", Outcome: report.Kept, Detail: reason},
		},
	})
	got := out.String()
	if !strings.Contains(got, "  = u  kept — edited locally\n") || !strings.Contains(got, "  = v  kept — edited locally\n") {
		t.Errorf("summary = %q, want both kept components with the short reason", got)
	}
	if n := strings.Count(got, "use --force to overwrite the components kept above"); n != 1 {
		t.Errorf("the --force hint appears %d times in %q, want exactly once", n, got)
	}
	hint := strings.Index(got, "⚠ use --force to overwrite the components kept above\n")
	sum := strings.Index(got, "2 kept")
	if hint < 0 || sum < hint {
		t.Errorf("summary = %q, want the ⚠ hint before the summary line", got)
	}
	if strings.Contains(got, "(differs from last pristine)") {
		t.Errorf("summary = %q, want the long reason cut at the first parenthesis", got)
	}
}

// Avisos que não são de um item (o .gitignore que ignora .claude/, a linha-base
// não gravada, um comando por projeto que falhou) saem como ⚠ depois das
// seções e antes do resumo.
func TestPrintInitAISummaryShowsLeftoverWarningsBeforeTheSummary(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile:  "demo",
		Entries:  []report.Entry{{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created}},
		Warnings: []string{"`graphify update .` failed (exit 1); run it again once the project has content"},
	})
	want := "init ai · demo\n" +
		"\n" +
		"Files\n" +
		"  + CLAUDE.md\n" +
		"\n" +
		"⚠ `graphify update .` failed (exit 1); run it again once the project has content\n" +
		"\n" +
		"1 created\n" +
		"\n" +
		"Next steps\n" +
		"  claude\n"
	if got := out.String(); got != want {
		t.Errorf("summary =\n%q\nwant\n%q", got, want)
	}
}

// O título leva o perfil resolvido; o `new` herda o resumo com o próprio título.
func TestPrintInitAISummaryTitleCarriesTheCommandAndTheProfile(t *testing.T) {
	for _, title := range []string{"init ai", "new"} {
		var out bytes.Buffer
		printInitAISummary(&out, title, initai.Summary{Profile: "go"})
		if want := title + " · go\n"; !strings.HasPrefix(out.String(), want) {
			t.Errorf("summary = %q, want it to start with %q", out.String(), want)
		}
	}
}

// Next steps é uma seção; os passos saem sem descrição, como hoje.
func TestPrintInitAISummaryNextStepsAreASectionOfBareCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Profile:        "demo",
		Entries:        []report.Entry{{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created}},
		VersionedPaths: []string{".claude", ".gitignore", "CLAUDE.md"},
		InGitRepo:      true,
	})
	want := "\nNext steps\n" +
		"  git add .claude .gitignore CLAUDE.md\n" +
		"  git commit -m \"chore: vendor ai environment\"\n" +
		"  claude\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Errorf("summary = %q, want it to end with %q", out.String(), want)
	}
}

// Em --dry-run nada foi escrito: o resumo diz o que seria feito, no
// condicional, e não afirma o passado.
func TestPrintInitAISummaryUsesTheConditionalInDryRun(t *testing.T) {
	sum := initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "s", Outcome: report.Installed},
			{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
			{Kind: report.File, Name: "docs/README.md", Outcome: report.Kept, Detail: "already there"},
		},
	}

	var real bytes.Buffer
	printInitAISummary(&real, "init ai", sum)
	if want := "1 installed · 1 created · 1 kept\n"; !strings.Contains(real.String(), want) {
		t.Errorf("real run output = %q, want %q", real.String(), want)
	}

	sum.DryRun = true
	var dry bytes.Buffer
	printInitAISummary(&dry, "init ai", sum)
	if want := "1 would install · 1 would create · 1 would skip\n"; !strings.Contains(dry.String(), want) {
		t.Errorf("dry-run output = %q, want %q", dry.String(), want)
	}
	for _, bad := range []string{"installed", "created", "kept"} {
		if strings.Contains(dry.String(), bad) {
			t.Errorf("dry-run output = %q, want no past-tense %q", dry.String(), bad)
		}
	}
}

// Em --dry-run nada foi escrito: mandar `git add` dos caminhos e rodar o
// `claude` num ambiente que não existe seria o rodapé afirmando o contrário do
// que o resumo acabou de dizer ("would create").
func TestPrintInitAISummaryNoNextStepsOnDryRun(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, "init ai", initai.Summary{
		Entries:        []report.Entry{{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created}},
		VersionedPaths: []string{".claude", "CLAUDE.md"},
		InGitRepo:      true,
		DryRun:         true,
	})
	got := out.String()
	if !strings.Contains(got, "would create") {
		t.Fatalf("summary = %q, want the dry-run wording", got)
	}
	for _, forbidden := range []string{"Next steps", "git add", "git commit"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("dry-run summary = %q, must not contain %q: nothing was written", got, forbidden)
		}
	}
}

// A cor reforça o marcador, nunca o substitui: tirados os escapes, a saída
// colorida é a mesma do texto simples; e cada papel tem a sua cor.
func TestPrintInitAISummaryColorsEachRole(t *testing.T) {
	fixHome(t, "")
	sum := initai.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "s", Outcome: report.Installed},
			{Kind: report.Component, Name: "gone", Outcome: report.Failed, Detail: "not found at x"},
			{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
			{Kind: report.File, Name: "h.md", Outcome: report.Kept, Detail: "already there"},
		},
		HadFailure: true,
	}
	var plain, colored bytes.Buffer
	printInitAISummary(&plain, "init ai", sum)
	printInitAISummary(ui.New(&colored, true), "init ai", sum)

	if got := stripEscapes(colored.String()); got != plain.String() {
		t.Errorf("colored output without escapes =\n%q\nwant the plain output\n%q", got, plain.String())
	}
	for _, want := range []string{
		esc + "[1minit ai" + esc + "[22m " + esc + "[90m· demo" + esc + "[0m\n",
		esc + "[32m✓" + esc + "[0m s\n",
		esc + "[31m✗" + esc + "[0m " + esc + "[31mgone" + esc + "[0m  " + esc + "[90mnot found at x" + esc + "[0m\n",
		esc + "[32m+" + esc + "[0m CLAUDE.md\n",
		esc + "[90m=" + esc + "[0m " + esc + "[90mh.md" + esc + "[0m" + strings.Repeat(" ", 9-4+2) + esc + "[90malready there" + esc + "[0m\n",
		esc + "[31m1 failed" + esc + "[0m",
		esc + "[32m1 installed" + esc + "[0m",
		esc + "[32m1 created" + esc + "[0m",
		esc + "[90m1 kept" + esc + "[0m",
	} {
		if !strings.Contains(colored.String(), want) {
			t.Errorf("colored output = %q, want it to contain %q", colored.String(), want)
		}
	}
}

// Repetir `init ai` no mesmo projeto: o componente intocado é recopiado (✓), os
// arquivos de scaffold que já existem saem "= … already there", e nada sai "+".
func TestRunInitAIRepeatedRunShowsInstalledComponentsAndKeptFiles(t *testing.T) {
	fixHome(t, "")
	home := newTestHome(t)
	home.ComponentsDir = filepath.Join(t.TempDir(), "components")
	if err := os.MkdirAll(filepath.Join(home.ComponentsDir, "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home.ComponentsDir, "s", "SKILL.md"), []byte("# s"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := newTestProfile(nil)
	p.Components = []profile.Component{{Name: "s", Dest: ".claude/skills"}}
	writeTestProfile(t, home.ProfilesDir, p)
	target := t.TempDir()

	runOnce := func() string {
		var out bytes.Buffer
		opts := initai.Options{Profile: "test", Target: target, NoGlobal: true, Out: &out}
		if err := runInitAI(&runner.FakeRunner{}, allFound, opts, home, &out); err != nil {
			t.Fatalf("runInitAI() error = %v: %s", err, out.String())
		}
		return out.String()
	}

	first := runOnce()
	if !strings.Contains(first, "  ✓ s\n") || !strings.Contains(first, "  + CLAUDE.md\n") {
		t.Fatalf("first run = %q, want the component installed and CLAUDE.md created", first)
	}

	second := runOnce()
	assertNoEscape(t, second)
	if !strings.Contains(second, "  ✓ s\n") {
		t.Errorf("second run = %q, want the untouched component installed again", second)
	}
	// O detalhe vem depois do preenchimento até o maior nome da seção Files.
	if !regexp.MustCompile(`\n  = CLAUDE\.md +already there\n`).MatchString(second) {
		t.Errorf("second run = %q, want CLAUDE.md kept as already there", second)
	}
	for _, line := range strings.Split(second, "\n") {
		if strings.HasPrefix(line, "  + ") {
			t.Errorf("second run lists %q as created; nothing is new in a repeated run", line)
		}
	}
}

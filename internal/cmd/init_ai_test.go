package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
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
	if !strings.Contains(out.String(), "Failed") {
		t.Errorf("output = %q, want it to contain the Failed summary section", out.String())
	}
}

func TestPrintInitAISummaryFooterInsideGitRepo(t *testing.T) {
	var out bytes.Buffer
	printInitAISummary(&out, initai.Summary{
		Created:        []string{"CLAUDE.md", ".claude/x", ".mcp.json"},
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
	printInitAISummary(&out, initai.Summary{
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
	printInitAISummary(&out, initai.Summary{
		Failed:         []string{"some/component"},
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
	if !strings.Contains(got, "Next steps:") {
		t.Errorf("output = %q, want the next-steps footer", got)
	}
	if !strings.Contains(got, "Warnings:") || !strings.Contains(got, "graphify update .") {
		t.Errorf("output = %q, want a warning about `graphify update .`", got)
	}
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
			printInitAISummary(&out, initai.Summary{
				Created:        []string{".claude/.ray-profile"},
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
	printInitAISummary(&out, initai.Summary{VersionedPaths: []string{".claude"}, InGitRepo: true})
	if strings.Contains(out.String(), "  cd ") {
		t.Errorf("output = %q, want no cd when the target is unknown", out.String())
	}
}

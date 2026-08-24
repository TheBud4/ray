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

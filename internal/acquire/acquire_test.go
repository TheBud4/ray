package acquire

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// stubRunner grava as chamadas e, se seed != nil, escreve arquivos em disco
// simulando o efeito de um `npx`/`git` real antes de devolver sucesso — é o
// que torna Acquire testável sem rede (o FakeRunner comum só registra
// chamadas, não escreve nada).
type stubRunner struct {
	calls []runner.Command
	seed  func(c runner.Command) error
	err   error
}

func (s *stubRunner) Run(_ context.Context, c runner.Command) (runner.Result, error) {
	s.calls = append(s.calls, c)
	if s.err != nil {
		return runner.Result{}, s.err
	}
	if s.seed != nil {
		if err := s.seed(c); err != nil {
			return runner.Result{}, err
		}
	}
	return runner.Result{ExitCode: 0}, nil
}

func TestCliAcquireCommandSkillsForcesCopyAndTelemetryOff(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}
	cmd, destRel, name, err := cliAcquireCommand(comp, "/tmp/proj")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd.String(), "--copy") {
		t.Errorf("command = %q, want --copy (I2: prevents symlink vendoring)", cmd.String())
	}
	if cmd.Env["DO_NOT_TRACK"] != "1" || cmd.Env["DISABLE_TELEMETRY"] != "1" {
		t.Errorf("Env = %v, want DO_NOT_TRACK and DISABLE_TELEMETRY set", cmd.Env)
	}
	if destRel != filepath.Join(".claude", "skills") {
		t.Errorf("destRel = %q, want %q", destRel, filepath.Join(".claude", "skills"))
	}
	if name != "s" {
		t.Errorf("name = %q, want %q", name, "s")
	}
}

func TestCliAcquireCommandAitmplForcesTelemetryOff(t *testing.T) {
	comp := profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "development-tools/code-reviewer"}
	cmd, destRel, name, err := cliAcquireCommand(comp, "/tmp/proj")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Env["DO_NOT_TRACK"] != "1" || cmd.Env["DISABLE_TELEMETRY"] != "1" {
		t.Errorf("Env = %v, want DO_NOT_TRACK and DISABLE_TELEMETRY set", cmd.Env)
	}
	if destRel != filepath.Join(".claude", "agents") {
		t.Errorf("destRel = %q, want %q", destRel, filepath.Join(".claude", "agents"))
	}
	if name != "code-reviewer.md" {
		t.Errorf("name = %q, want %q", name, "code-reviewer.md")
	}
}

func TestCliAcquireCommandUnknownAitmplType(t *testing.T) {
	comp := profile.Component{Via: profile.ViaAitmpl, Type: "tool", Ref: "x"}
	if _, _, _, err := cliAcquireCommand(comp, "/tmp"); err == nil {
		t.Fatal("cliAcquireCommand() = nil error, want error for unknown aitmpl type")
	}
}

func TestGitFetchCommand(t *testing.T) {
	comp := profile.Component{Via: profile.ViaGit, Repo: "acme/skills", Path: "skills/widget"}
	cmd := gitFetchCommand(comp, "/tmp/clone")
	want := "git clone --depth 1 --branch main https://github.com/acme/skills.git /tmp/clone"
	if cmd.String() != want {
		t.Errorf("gitFetchCommand() = %q, want %q", cmd.String(), want)
	}
}

func TestGitFetchCommandUsesPinnedRef(t *testing.T) {
	comp := profile.Component{Via: profile.ViaGit, Repo: "acme/skills", Ref: "v2", Path: "skills/widget"}
	cmd := gitFetchCommand(comp, "/tmp/clone")
	if !strings.Contains(cmd.String(), "--branch v2") {
		t.Errorf("gitFetchCommand() = %q, want it to use the pinned ref v2", cmd.String())
	}
}

func TestKeyNamespacing(t *testing.T) {
	cases := []struct {
		name string
		acq  Acquirer
		comp profile.Component
		want string
	}{
		{
			name: "skills",
			acq:  CliAcquirer{},
			comp: profile.Component{Via: profile.ViaSkills, Source: "o/r", Skill: "s"},
			want: "skills:o/r#s",
		},
		{
			name: "aitmpl",
			acq:  CliAcquirer{},
			comp: profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "o/r"},
			want: "aitmpl:agent:o/r",
		},
		{
			name: "git pinned",
			acq:  GitAcquirer{},
			comp: profile.Component{Via: profile.ViaGit, Repo: "o/r", Ref: "v1", Path: "skills/x"},
			want: "git:o/r@v1#skills/x",
		},
		{
			name: "git defaults to main",
			acq:  GitAcquirer{},
			comp: profile.Component{Via: profile.ViaGit, Repo: "o/r", Path: "skills/x"},
			want: "git:o/r@main#skills/x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.acq.Key(tc.comp); got != tc.want {
				t.Errorf("Key() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestForFactory(t *testing.T) {
	r := &stubRunner{}
	cases := []struct {
		name    string
		comp    profile.Component
		wantOK  bool
		wantAcq string // "cli", "git", ou "" se wantOK=false
	}{
		{"skills", profile.Component{Via: profile.ViaSkills}, true, "cli"},
		{"aitmpl agent", profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent}, true, "cli"},
		{"aitmpl command", profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeCommand}, true, "cli"},
		{"aitmpl mcp is not content", profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeMCP}, false, ""},
		{"git", profile.Component{Via: profile.ViaGit}, true, "git"},
		{"unknown via", profile.Component{Via: "bogus"}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acq, ok := For(tc.comp, r)
			if ok != tc.wantOK {
				t.Fatalf("For() ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			switch tc.wantAcq {
			case "cli":
				if _, isCli := acq.(CliAcquirer); !isCli {
					t.Errorf("For() = %T, want CliAcquirer", acq)
				}
			case "git":
				if _, isGit := acq.(GitAcquirer); !isGit {
					t.Errorf("For() = %T, want GitAcquirer", acq)
				}
			}
		})
	}
}

func TestCliAcquirerAcquireRoundTrip(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "prompt-engineer", Source: "jeffallan/claude-skills"}
	stub := &stubRunner{seed: func(c runner.Command) error {
		skillDir := filepath.Join(c.Dir, ".claude", "skills", "prompt-engineer")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# prompt engineer"), 0o644)
	}}

	acq := CliAcquirer{Runner: stub}
	res, err := acq.Acquire(context.Background(), comp)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if res.DestRel != filepath.Join(".claude", "skills") {
		t.Errorf("DestRel = %q, want %q", res.DestRel, filepath.Join(".claude", "skills"))
	}
	got, err := os.ReadFile(filepath.Join(res.Dir, "prompt-engineer", "SKILL.md"))
	if err != nil {
		t.Fatalf("stat acquired SKILL.md: %v", err)
	}
	if string(got) != "# prompt engineer" {
		t.Errorf("SKILL.md = %q, want %q", got, "# prompt engineer")
	}
	if res.Origin != "jeffallan/claude-skills" {
		t.Errorf("Origin = %q, want %q", res.Origin, "jeffallan/claude-skills")
	}
	if res.HasLicense {
		t.Error("HasLicense = true, but no LICENSE was seeded")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "prompt-engineer", ".ray-origin")); err != nil {
		t.Errorf(".ray-origin not written: %v", err)
	}
}

func TestCliAcquirerAcquireCapturesLicense(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}
	stub := &stubRunner{seed: func(c runner.Command) error {
		skillDir := filepath.Join(c.Dir, ".claude", "skills", "s")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("x"), 0o644); err != nil {
			return err
		}
		// LICENSE at the project root, as npx would leave it alongside the
		// skill (not inside the skill dir itself).
		return os.WriteFile(filepath.Join(c.Dir, "LICENSE"), []byte("MIT"), 0o644)
	}}

	acq := CliAcquirer{Runner: stub}
	res, err := acq.Acquire(context.Background(), comp)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if !res.HasLicense {
		t.Fatal("HasLicense = false, want true: a LICENSE was seeded at the project root")
	}
	data, err := os.ReadFile(filepath.Join(res.Dir, "s", "LICENSE"))
	if err != nil {
		t.Fatalf("LICENSE not captured into the content dir: %v", err)
	}
	if string(data) != "MIT" {
		t.Errorf("LICENSE = %q, want %q", data, "MIT")
	}
}

func TestCliAcquirerAcquireFailsOnNonZeroExit(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}
	// Non-zero exit without seeding any file at all — Acquire must surface an
	// error, not silently succeed with missing content.
	acq := CliAcquirer{Runner: &fakeExitRunner{exit: 1}}
	if _, err := acq.Acquire(context.Background(), comp); err == nil {
		t.Fatal("Acquire() = nil error, want error on non-zero exit")
	}
}

type fakeExitRunner struct{ exit int }

func (f *fakeExitRunner) Run(_ context.Context, _ runner.Command) (runner.Result, error) {
	return runner.Result{ExitCode: f.exit}, nil
}

func TestGitAcquirerAcquireRoundTrip(t *testing.T) {
	comp := profile.Component{Via: profile.ViaGit, Repo: "acme/skills", Ref: "v1", Path: "skills/widget"}
	stub := &stubRunner{seed: func(c runner.Command) error {
		clone := c.Args[len(c.Args)-1]
		widgetDir := filepath.Join(clone, "skills", "widget")
		if err := os.MkdirAll(widgetDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(widgetDir, "SKILL.md"), []byte("# widget"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(clone, "LICENSE"), []byte("Apache-2.0"), 0o644)
	}}

	acq := GitAcquirer{Runner: stub}
	res, err := acq.Acquire(context.Background(), comp)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if res.Origin != "acme/skills@v1" {
		t.Errorf("Origin = %q, want %q", res.Origin, "acme/skills@v1")
	}
	if !res.HasLicense {
		t.Fatal("HasLicense = false, want true")
	}
	got, err := os.ReadFile(filepath.Join(res.Dir, "widget", "SKILL.md"))
	if err != nil {
		t.Fatalf("stat acquired SKILL.md: %v", err)
	}
	if string(got) != "# widget" {
		t.Errorf("SKILL.md = %q, want %q", got, "# widget")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "widget", ".ray-origin")); err != nil {
		t.Errorf(".ray-origin not written: %v", err)
	}
}

func TestGitAcquirerAcquirePathNotFound(t *testing.T) {
	comp := profile.Component{Via: profile.ViaGit, Repo: "acme/skills", Path: "skills/missing"}
	stub := &stubRunner{seed: func(c runner.Command) error { return nil }} // clone "succeeds" but path never materializes
	acq := GitAcquirer{Runner: stub}
	if _, err := acq.Acquire(context.Background(), comp); err == nil {
		t.Fatal("Acquire() = nil error, want error when comp.Path is absent from the clone")
	}
}

func TestGlobalInstallCommand(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}
	cmd, err := GlobalInstallCommand(comp)
	if err != nil {
		t.Fatal(err)
	}
	want := "npx skills add o/r --skill s -a claude-code -y -g"
	if cmd.String() != want {
		t.Errorf("GlobalInstallCommand() = %q, want %q", cmd.String(), want)
	}
	if strings.Contains(cmd.String(), "--copy") {
		t.Error("personal/global installs should not force --copy (symlinks are fine outside the vendored project tree)")
	}
}

func TestGlobalInstallCommandRejectsNonSkills(t *testing.T) {
	comp := profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "o/r"}
	if _, err := GlobalInstallCommand(comp); err == nil {
		t.Fatal("GlobalInstallCommand() = nil error, want error: aitmpl has no personal/global install path")
	}
}

func TestDestRel(t *testing.T) {
	cases := []struct {
		name string
		comp profile.Component
		want string
	}{
		{"skills", profile.Component{Via: profile.ViaSkills}, filepath.Join(".claude", "skills")},
		{"git", profile.Component{Via: profile.ViaGit}, filepath.Join(".claude", "skills")},
		{"aitmpl agent", profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent}, filepath.Join(".claude", "agents")},
		{"aitmpl command", profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeCommand}, filepath.Join(".claude", "commands")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DestRel(tc.comp)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("DestRel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDestRelMatchesAcquireResult(t *testing.T) {
	comp := profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}
	stub := &stubRunner{seed: func(c runner.Command) error {
		dir := filepath.Join(c.Dir, ".claude", "skills", "s")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644)
	}}
	acq := CliAcquirer{Runner: stub}
	res, err := acq.Acquire(context.Background(), comp)
	if err != nil {
		t.Fatal(err)
	}
	want, err := DestRel(comp)
	if err != nil {
		t.Fatal(err)
	}
	if res.DestRel != want {
		t.Errorf("Acquire().DestRel = %q, want it to match DestRel() = %q", res.DestRel, want)
	}
}

func TestLeafName(t *testing.T) {
	cases := []struct {
		name string
		comp profile.Component
		want string
	}{
		{"git", profile.Component{Via: profile.ViaGit, Path: "skills/widget"}, "widget"},
		{"skills", profile.Component{Via: profile.ViaSkills, Skill: "prompt-engineer"}, "prompt-engineer"},
		{"aitmpl", profile.Component{Via: profile.ViaAitmpl, Ref: "development-tools/code-reviewer"}, "code-reviewer.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LeafName(tc.comp)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("LeafName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLeafNameUnknownVia(t *testing.T) {
	if _, err := LeafName(profile.Component{Via: "bogus"}); err == nil {
		t.Fatal("LeafName() = nil error, want error for unknown via")
	}
}

func TestPreviewCommand(t *testing.T) {
	cases := []struct {
		name string
		comp profile.Component
		want string
	}{
		{
			name: "skills",
			comp: profile.Component{Via: profile.ViaSkills, Skill: "s", Source: "o/r"},
			want: "npx skills add o/r --skill s -a claude-code -y --copy",
		},
		{
			name: "git",
			comp: profile.Component{Via: profile.ViaGit, Repo: "o/r", Path: "skills/x"},
			want: "git clone --depth 1 --branch main https://github.com/o/r.git <clone>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := PreviewCommand(tc.comp)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.String() != tc.want {
				t.Errorf("PreviewCommand() = %q, want %q", cmd.String(), tc.want)
			}
		})
	}
}

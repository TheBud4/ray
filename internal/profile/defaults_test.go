package profile

import "testing"

func TestDefaultsValidate(t *testing.T) {
	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			if err := p.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestDefaultsComponentCounts(t *testing.T) {
	want := map[string]int{"go": 11, "web": 11, "flutter": 12}

	got := map[string]int{}
	for _, p := range Defaults() {
		got[p.Name] = len(p.Components)
	}

	if len(got) != len(want) {
		t.Fatalf("Defaults() has %d profiles, want %d (%v)", len(got), len(want), got)
	}
	for name, wantCount := range want {
		if got[name] != wantCount {
			t.Errorf("profile %q has %d components, want %d", name, got[name], wantCount)
		}
	}
}

func TestDefaultsIntegrationsAndSettings(t *testing.T) {
	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			i := p.Integrations
			if !(i.Headroom && i.Brain && i.CodeGraph) {
				t.Errorf("Integrations = %+v, want all true", i)
			}
			if p.Scaffold.Settings["model"] != "opus" {
				t.Errorf("Settings[model] = %v, want opus", p.Scaffold.Settings["model"])
			}
			if p.Scaffold.Settings["effortLevel"] != "high" {
				t.Errorf("Settings[effortLevel] = %v, want high", p.Scaffold.Settings["effortLevel"])
			}
		})
	}
}

func TestDefaultsScaffoldFiles(t *testing.T) {
	wantPaths := []string{
		"CLAUDE.md",
		"SECURITY.md",
		"docs/README.md",
		"docs/architecture.md",
		"docs/conventions.md",
		"docs/specs/TEMPLATE.md",
		".claude/commands/document.md",
		".claude/commands/handoff.md",
		".claude/commands/revisar.md",
		".claude/handoff.md",
	}

	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			if len(p.Scaffold.Files) != len(wantPaths) {
				t.Fatalf("len(Scaffold.Files) = %d, want %d", len(p.Scaffold.Files), len(wantPaths))
			}
			for i, f := range p.Scaffold.Files {
				if f.Path != wantPaths[i] {
					t.Errorf("Scaffold.Files[%d].Path = %q, want %q", i, f.Path, wantPaths[i])
				}
			}
		})
	}
}

func TestDefaultsGitignoreStack(t *testing.T) {
	want := map[string][]string{
		"go":      {"/{{.ProjectName}}"},
		"web":     {"node_modules/", ".next/"},
		"flutter": {".dart_tool/", "build/"},
	}

	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			wantLines, ok := want[p.Name]
			if !ok {
				t.Fatalf("unexpected default profile %q", p.Name)
			}
			if len(p.Scaffold.GitignoreStack) != len(wantLines) {
				t.Fatalf("GitignoreStack = %v, want %v", p.Scaffold.GitignoreStack, wantLines)
			}
			for i, l := range wantLines {
				if p.Scaffold.GitignoreStack[i] != l {
					t.Errorf("GitignoreStack[%d] = %q, want %q", i, p.Scaffold.GitignoreStack[i], l)
				}
			}
		})
	}
}

func TestDefaultsCreateCommands(t *testing.T) {
	want := map[string]string{
		"go":      "go mod init {{.Name}}",
		"web":     "npx create-next-app@latest . --yes",
		"flutter": "flutter create .",
	}

	for _, p := range Defaults() {
		wantCmd, ok := want[p.Name]
		if !ok {
			t.Fatalf("unexpected default profile %q", p.Name)
		}
		if len(p.Create) != 1 || p.Create[0] != wantCmd {
			t.Errorf("profile %q Create = %v, want [%q]", p.Name, p.Create, wantCmd)
		}
	}
}

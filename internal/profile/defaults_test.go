package profile

import "testing"

// RF-07: `ray init ai` deixou de exigir --profile e passou a cair no
// perfil `base` quando nenhum é passado — este precisa existir de fábrica,
// com o scaffold universal (mesmo build() dos outros) e nada específico de
// stack.
func TestDefaultsIncludesBaseProfile(t *testing.T) {
	defs := Defaults()
	if len(defs) != 4 {
		t.Fatalf("len(Defaults()) = %d, want 4 (go, web, flutter, base)", len(defs))
	}
	for _, p := range defs {
		if p.Name == "base" {
			return
		}
	}
	t.Errorf("Defaults() = %v, want one profile named %q", defs, "base")
}

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

// O ray não baixa nem embute conteúdo de terceiros: nenhum perfil default
// declara componente, porque nenhum tem como saber o que o usuário já
// colocou em internal/raypaths.ComponentsDir().
func TestDefaultsHaveNoComponents(t *testing.T) {
	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			if len(p.Components) != 0 {
				t.Errorf("profile %q has %d components, want 0", p.Name, len(p.Components))
			}
		})
	}
}

func TestDefaultsIntegrationsAndSettings(t *testing.T) {
	for _, p := range Defaults() {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			i := p.Integrations
			if !(i.Headroom && i.CodeGraph) {
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
		".claude/commands/document.md",
		".claude/commands/handoff.md",
		".claude/commands/revisar.md",
		".claude/commands/destilar.md",
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

// base (RF-07: init ai independente de profile) não tem stack — Create e
// GitignoreStack vazios por design, não um esquecimento.
func TestDefaultsGitignoreStack(t *testing.T) {
	want := map[string][]string{
		"go":      {"/{{.ProjectName}}"},
		"web":     {"node_modules/", ".next/"},
		"flutter": {".dart_tool/", "build/"},
		"base":    {},
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

// base (RF-07) não roda create: nenhum — quem quer o scaffold de um projeto
// de verdade usa `ray new <stack>`, não `init ai`.
func TestDefaultsCreateCommands(t *testing.T) {
	want := map[string]string{
		"go":      "go mod init {{.ProjectName}}",
		"web":     "npx create-next-app@latest . --yes",
		"flutter": "flutter create .",
		"base":    "",
	}

	for _, p := range Defaults() {
		wantCmd, ok := want[p.Name]
		if !ok {
			t.Fatalf("unexpected default profile %q", p.Name)
		}
		if wantCmd == "" {
			if len(p.Create) != 0 {
				t.Errorf("profile %q Create = %v, want empty", p.Name, p.Create)
			}
			continue
		}
		if len(p.Create) != 1 || p.Create[0] != wantCmd {
			t.Errorf("profile %q Create = %v, want [%q]", p.Name, p.Create, wantCmd)
		}
	}
}

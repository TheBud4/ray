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
			if !(i.Headroom && i.KnowledgeVault && i.SecondBrain && i.ObsidianFormats && i.CodeGraph && i.UserDocsVault) {
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

package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
		wantErr string // substring expected in the error; "" means no error
	}{
		{
			name: "valid skills component",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaSkills, Skill: "s", Source: "o/r"}},
			},
		},
		{
			name: "valid aitmpl component",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaAitmpl, Type: TypeAgent, Ref: "o/r"}},
			},
		},
		{
			name:    "missing name",
			profile: Profile{},
			wantErr: "name is required",
		},
		{
			name:    "whitespace-only name",
			profile: Profile{Name: "   "},
			wantErr: "name is required",
		},
		{
			name: "unknown via",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: "foo"}},
			},
			wantErr: "unknown 'via'",
		},
		{
			name: "empty via",
			profile: Profile{
				Name:       "go",
				Components: []Component{{}},
			},
			wantErr: "'via' is required",
		},
		{
			name: "skills missing skill",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaSkills, Source: "o/r"}},
			},
			wantErr: "requires both 'skill' and 'source'",
		},
		{
			name: "skills missing source",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaSkills, Skill: "s"}},
			},
			wantErr: "requires both 'skill' and 'source'",
		},
		{
			name: "aitmpl missing type",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaAitmpl, Ref: "o/r"}},
			},
			wantErr: "requires type agent|command|mcp",
		},
		{
			name: "aitmpl invalid type",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaAitmpl, Type: "tool", Ref: "o/r"}},
			},
			wantErr: "requires type agent|command|mcp",
		},
		{
			name: "aitmpl missing ref",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaAitmpl, Type: TypeAgent}},
			},
			wantErr: "requires 'ref'",
		},
		{
			name: "valid git component",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaGit, Repo: "o/r", Ref: "main", Path: "skills/x"}},
			},
		},
		{
			name: "git missing repo",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaGit, Path: "skills/x"}},
			},
			wantErr: "requires 'repo' and 'path'",
		},
		{
			name: "git missing path",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaGit, Repo: "o/r"}},
			},
			wantErr: "requires 'repo' and 'path'",
		},
		{
			name: "git ref is optional",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Via: ViaGit, Repo: "o/r", Path: "skills/x"}},
			},
		},
		{
			name: "scaffold file empty path",
			profile: Profile{
				Name:     "go",
				Scaffold: Scaffold{Files: []ScaffoldFile{{Path: ""}}},
			},
			wantErr: "path is required",
		},
		{
			name: "error on second component preserves index",
			profile: Profile{
				Name: "go",
				Components: []Component{
					{Via: ViaSkills, Skill: "s", Source: "o/r"},
					{Via: "bogus"},
				},
			},
			wantErr: "component 1",
		},
		{
			name: "valid milestone",
			profile: Profile{
				Name:       "go",
				Milestones: []Milestone{{Goal: "Skeleton compiles", Verify: "go build ./..."}},
			},
		},
		{
			name: "milestone missing goal",
			profile: Profile{
				Name:       "go",
				Milestones: []Milestone{{Verify: "go build ./..."}},
			},
			wantErr: "goal is required",
		},
		{
			name: "milestone missing verify",
			profile: Profile{
				Name:       "go",
				Milestones: []Milestone{{Goal: "Skeleton compiles"}},
			},
			wantErr: "verify is required",
		},
		{
			name: "error on second milestone preserves index",
			profile: Profile{
				Name: "go",
				Milestones: []Milestone{
					{Goal: "a", Verify: "true"},
					{Goal: "b"},
				},
			},
			wantErr: "milestone 1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %q, want error containing %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.yaml")

	want := Profile{
		Name:        "x",
		Description: "test profile",
		Components:  []Component{{Via: ViaSkills, Skill: "s", Source: "o/r"}},
		Scaffold:    Scaffold{Settings: map[string]any{"model": "opus"}},
	}
	data, err := yaml.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if len(got.Components) != len(want.Components) {
		t.Errorf("len(Components) = %d, want %d", len(got.Components), len(want.Components))
	}
	if got.Scaffold.Settings["model"] != "opus" {
		t.Errorf("Scaffold.Settings[model] = %v, want opus", got.Scaffold.Settings["model"])
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Run("nonexistent path", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		if err == nil {
			t.Fatal("Load() = nil, want error")
		}
		if !strings.Contains(err.Error(), "missing.yaml") {
			t.Errorf("Load() error = %q, want it to contain the path", err.Error())
		}
	})

	t.Run("invalid component", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		data, err := yaml.Marshal(Profile{
			Name:       "bad",
			Components: []Component{{Via: "nope"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}

		_, err = Load(path)
		if err == nil {
			t.Fatal("Load() = nil, want error")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("Load() error = %q, want it to contain the path", err.Error())
		}
		if !strings.Contains(err.Error(), "unknown 'via'") {
			t.Errorf("Load() error = %q, want it to contain the validation reason", err.Error())
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "malformed.yaml")
		if err := os.WriteFile(path, []byte(":\n  - ["), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() = nil, want error")
		}
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("Load() error = %q, want it to contain \"parsing\"", err.Error())
		}
	})
}

func writeTestProfile(t *testing.T, profilesDir, name string) {
	t.Helper()
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(Profile{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, name+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadForTargetOverrideWins(t *testing.T) {
	profilesDir := t.TempDir()
	writeTestProfile(t, profilesDir, "go")
	target := t.TempDir() // no record file at all

	got, err := LoadForTarget(profilesDir, target, "go")
	if err != nil {
		t.Fatalf("LoadForTarget() error = %v", err)
	}
	if got.Name != "go" {
		t.Errorf("Name = %q, want %q", got.Name, "go")
	}
}

func TestLoadForTargetFallsBackToRecord(t *testing.T) {
	profilesDir := t.TempDir()
	writeTestProfile(t, profilesDir, "web")
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProfileRecordPath(target), []byte("web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadForTarget(profilesDir, target, "")
	if err != nil {
		t.Fatalf("LoadForTarget() error = %v", err)
	}
	if got.Name != "web" {
		t.Errorf("Name = %q, want %q", got.Name, "web")
	}
}

func TestLoadForTargetMissingBothErrors(t *testing.T) {
	profilesDir := t.TempDir()
	target := t.TempDir()

	if _, err := LoadForTarget(profilesDir, target, ""); err == nil {
		t.Fatal("LoadForTarget() = nil error, want error when no record and no override")
	}
}

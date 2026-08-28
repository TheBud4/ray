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
			name: "valid component",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Name: "prompt-engineer", Dest: ".claude/skills"}},
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
			name: "component missing name",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Dest: ".claude/skills"}},
			},
			wantErr: "name is required",
		},
		{
			name: "component whitespace-only name",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Name: "   ", Dest: ".claude/skills"}},
			},
			wantErr: "name is required",
		},
		{
			name: "component missing dest",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Name: "prompt-engineer"}},
			},
			wantErr: "dest is required",
		},
		{
			name: "component whitespace-only dest",
			profile: Profile{
				Name:       "go",
				Components: []Component{{Name: "prompt-engineer", Dest: "   "}},
			},
			wantErr: "dest is required",
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
					{Name: "prompt-engineer", Dest: ".claude/skills"},
					{Name: "bogus"},
				},
			},
			wantErr: "component 1",
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
		Components:  []Component{{Name: "prompt-engineer", Dest: ".claude/skills"}},
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
			Components: []Component{{Dest: ".claude/skills"}},
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
		if !strings.Contains(err.Error(), "name is required") {
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

	// RF-03: uma chave de topo errada ("scaffhold" em vez de "scaffold") não
	// pode virar Scaffold{} vazia em silêncio — a receita "funcionava" e
	// simplesmente não escrevia nenhum arquivo, sem nada avisando por quê.
	t.Run("unknown top-level key", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "typo.yaml")
		body := "name: typo\ndescription: test\nscaffhold:\n  files:\n    - path: CLAUDE.md\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() = nil, want error for an unknown top-level key")
		}
		if !strings.Contains(err.Error(), "scaffhold") {
			t.Errorf("Load() error = %q, want it to name the unknown field", err.Error())
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

	_, err := LoadForTarget(profilesDir, target, "")
	if err == nil {
		t.Fatal("LoadForTarget() = nil error, want error when no record and no override")
	}

	msg := err.Error()
	// O erro do os.ReadFile já carrega o caminho, e envolvê-lo repetia o
	// caminho inteiro duas vezes na mesma linha.
	if n := strings.Count(msg, ProfileRecordPath(target)); n != 1 {
		t.Errorf("error names the path %d times, want once: %q", n, msg)
	}
	// Sem registro e sem override, a saída é uma só: dizer qual.
	if !strings.Contains(msg, "--profile") {
		t.Errorf("error = %q, want it to point at --profile", msg)
	}
}

// Três chamadores montavam `filepath.Join(dir, name+".yaml")` na mão e
// entregavam o erro cru do os.ReadFile, que fala de arquivo — o usuário digitou
// um nome de receita e recebia um caminho.
func TestLoadByNameNamesTheProfile(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadByName(dir, "naoexiste")
	if err == nil {
		t.Fatal("LoadByName() = nil error, want error for a profile that is not there")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"naoexiste"`) {
		t.Errorf("error = %q, want it to quote the profile name", msg)
	}
	if strings.Contains(msg, "no such file or directory") {
		t.Errorf("error = %q, want the recipe vocabulary, not the raw file error", msg)
	}
}

func TestLoadByNameLoadsAndValidates(t *testing.T) {
	dir := t.TempDir()
	writeTestProfile(t, dir, "go")

	p, err := LoadByName(dir, "go")
	if err != nil {
		t.Fatalf("LoadByName() error = %v", err)
	}
	if p.Name != "go" {
		t.Errorf("Name = %q, want %q", p.Name, "go")
	}
}

// Um erro de leitura que não seja "não existe" não pode virar a mesma frase:
// "nenhum perfil registrado" seria mentira se o arquivo existe e não pôde ser
// lido.
func TestLoadForTargetSurfacesRealReadErrors(t *testing.T) {
	profilesDir := t.TempDir()
	target := t.TempDir()
	// Um diretório no lugar do arquivo de registro: existe, e ReadFile falha
	// com algo que não é IsNotExist.
	if err := os.MkdirAll(ProfileRecordPath(target), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadForTarget(profilesDir, target, "")
	if err == nil {
		t.Fatal("LoadForTarget() = nil error, want the underlying read error")
	}
	if strings.Contains(err.Error(), "no profile recorded") {
		t.Errorf("error = %q, want it to report the read failure, not a missing record", err)
	}
}

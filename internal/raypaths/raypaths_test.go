package raypaths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeUsesRayHome(t *testing.T) {
	t.Setenv("RAY_HOME", "/custom/root")

	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if home != "/custom/root" {
		t.Errorf("Home() = %q, want %q", home, "/custom/root")
	}

	cases := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"ProfilesDir", ProfilesDir, "/custom/root/profiles"},
		{"TemplatesDir", TemplatesDir, "/custom/root/templates"},

		{"ConfigPath", ConfigPath, "/custom/root/config.yaml"},
		{"StatePath", StatePath, "/custom/root/state.yaml"},
		{"CommandsPath", CommandsPath, "/custom/root/commands.yaml"},
		{"StoreDir", StoreDir, "/custom/root/store"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("%s() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestHomeFallsBackToDotRay(t *testing.T) {
	t.Setenv("RAY_HOME", "")

	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(userHome, ".ray")
	if home != want {
		t.Errorf("Home() = %q, want %q", home, want)
	}
}

// Um RAY_HOME relativo aponta para onde o ray foi rodado, não para um lugar
// fixo: o estado iria parar em outra pasta a cada diretório corrente. E um `~`
// que o shell não expandiu (variável entre aspas, arquivo de env) viraria uma
// pasta literal chamada "~" dentro do projeto.
func TestHomeNormalizesRayHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, env, want string
	}{
		{"relative", "state/ray", filepath.Join(cwd, "state", "ray")},
		{"dot", ".", cwd},
		{"tilde alone", "~", user},
		{"tilde slash", "~/ray-state", filepath.Join(user, "ray-state")},
		{"tilde inside a name is kept", "/srv/~backup", "/srv/~backup"},
		{"unclean absolute", "/custom//root/../root", "/custom/root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RAY_HOME", tc.env)
			got, err := Home()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Home() with RAY_HOME=%q = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}

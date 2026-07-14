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
		{"VaultDir", VaultDir, "/custom/root/vault"},
		{"ConfigPath", ConfigPath, "/custom/root/config.yaml"},
		{"StatePath", StatePath, "/custom/root/state.yaml"},
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

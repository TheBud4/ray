package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	cases := []struct {
		name       string
		modVersion string
		revision   string
		when       string
		dirty      bool
		want       string
	}{
		{
			// Instalado de uma tag: a versão do módulo já diz tudo, e o
			// toolchain nem grava vcs.* nesse caminho (o módulo veio do proxy,
			// não de um clone).
			name:       "tagged release",
			modVersion: "v1.2.3",
			want:       "v1.2.3",
		},
		{
			// `go install .` num clone: a versão do módulo é uma
			// pseudo-versão que já embute o hash, então repeti-la seria ruído.
			name:       "local build from a clean tree",
			modVersion: "v0.0.0-20260801003324-e78af0370056",
			revision:   "e78af0370056736270c63d5faa3da481155d37df",
			when:       "2026-08-01T00:33:24Z",
			want:       "devel (e78af03, 2026-08-01T00:33:24Z)",
		},
		{
			// Árvore suja é a informação mais importante das três: o binário
			// não corresponde a commit nenhum, e um relato de bug feito com
			// ele não é reproduzível.
			name:       "local build from a dirty tree",
			modVersion: "v0.0.0-20260801003324-e78af0370056+dirty",
			revision:   "e78af0370056736270c63d5faa3da481155d37df",
			when:       "2026-08-01T00:33:24Z",
			dirty:      true,
			want:       "devel (e78af03, 2026-08-01T00:33:24Z, dirty)",
		},
		{
			name:       "no vcs stamp at all",
			modVersion: "(devel)",
			want:       "devel",
		},
		{
			name: "nothing known",
			want: "devel",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatVersion(tc.modVersion, tc.revision, tc.when, tc.dirty)
			if got != tc.want {
				t.Errorf("formatVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

// O achado que originou isto: `ray --version` respondia "unknown flag". O
// Cobra só registra a flag se o comando raiz preencher Version — não vem de
// graça, ao contrário do que o build guide afirmava.
func TestRootVersionFlagIsRegistered(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--version) error = %v, want nil", err)
	}
	got := out.String()
	if !strings.Contains(got, "ray version") {
		t.Errorf("output = %q, want it to name the binary and its version", got)
	}
}

package cmd

import (
	"runtime/debug"
	"strings"
)

// buildVersion compõe o que `ray --version` responde, a partir do que o
// toolchain já embute no binário (`debug.ReadBuildInfo`).
//
// Não há `-ldflags -X`, e a razão é o caminho de instalação: o README manda
// `go install .` / `make install`, e ldflags só alcança quem passa a flag —
// a versão ficaria `dev` justamente para quem instalou do jeito documentado.
// O `go install` já grava `vcs.revision`, `vcs.time` e `vcs.modified` de um
// clone git, e a versão do módulo quando vem de uma tag. Se um dia houver
// release com binário pronto, ldflags pode entrar por cima sem desfazer isto.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return formatVersion("", "", "", false)
	}
	var revision, when string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			when = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return formatVersion(info.Main.Version, revision, when, dirty)
}

// formatVersion existe separada de buildVersion para ser testável: o
// debug.ReadBuildInfo responde sobre o binário de teste, não sobre o `ray`.
//
// Build de tag devolve a versão do módulo e para aí. Build local devolve
// `devel` mais revisão, data e — o que mais importa — se a árvore estava
// suja: um binário de árvore suja não corresponde a commit nenhum, e um
// relato de bug feito com ele não é reproduzível.
//
// Sujo inclui arquivo não rastreado, e o excesso é deliberado: um `.go` novo
// ainda não commitado entra na compilação, e o `vcs.modified` não tem como
// separá-lo de um rascunho solto. O marcador existe para desconfiar do
// binário, então errar para `dirty` é o lado seguro.
func formatVersion(modVersion, revision, when string, dirty bool) string {
	base := "devel"
	// Pseudo-versão (`v0.0.0-<data>-<hash>`) é build local, não release: ela
	// já embute o hash que os parênteses vão mostrar, e repeti-la é ruído.
	if modVersion != "" && modVersion != "(devel)" && !strings.HasPrefix(modVersion, "v0.0.0-") {
		base = modVersion
	}

	var parts []string
	if revision != "" {
		if len(revision) > 7 {
			revision = revision[:7]
		}
		parts = append(parts, revision)
	}
	if when != "" {
		parts = append(parts, when)
	}
	if dirty {
		parts = append(parts, "dirty")
	}
	if len(parts) == 0 {
		return base
	}
	return base + " (" + strings.Join(parts, ", ") + ")"
}

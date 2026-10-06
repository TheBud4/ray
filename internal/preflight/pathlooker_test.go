package preflight

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeExecutable cria um executável vazio em dir e devolve o nome pelo qual
// ele é procurado. No Windows o exec.LookPath só acha arquivos com extensão do
// PATHEXT, então o arquivo ganha ".exe" e o nome devolvido continua sem ela.
func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	p := filepath.Join(dir, file)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestPathLookerFindsExecutable(t *testing.T) {
	dir := t.TempDir()
	name := writeExecutable(t, dir, "raytestbin")
	t.Setenv("PATH", dir)

	if !(PathLooker{}).Look(name) {
		t.Errorf("Look(%s) = false, want true for an executable on PATH", name)
	}
}

func TestPathLookerMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if (PathLooker{}).Look("raytestbin") {
		t.Error("Look(raytestbin) = true, want false when nothing on PATH matches")
	}
}

// Um arquivo sem bit de execução não conta: é a diferença entre existir e
// poder ser rodado, e o .mcp.json aponta comandos para rodar.
func TestPathLookerNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("o Windows não tem bit de execução: executável é questão de extensão")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "raytestbin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if (PathLooker{}).Look("raytestbin") {
		t.Error("Look(raytestbin) = true, want false for a non-executable file")
	}
}

func TestPathLookerPythonAlias(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "python3")
	t.Setenv("PATH", dir)

	if !(PathLooker{}).Look("python3.10+") {
		t.Error("Look(python3.10+) = false, want true — it resolves to python3")
	}
}

// No Windows o instalador oficial expõe "python", não "python3" — o
// runtime.GOOS de quem roda o teste é fixo (Linux), então o fallback é
// verificado direto na função interna parametrizada por goos.
func TestPathLookerPythonWindowsFallback(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "python")
	t.Setenv("PATH", dir)

	if !lookPath("python3.10+", "windows") {
		t.Error("lookPath(python3.10+, windows) = false, want true — falls back to python when python3 is absent")
	}
}

func TestPathLookerPythonNoFallbackOutsideWindows(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "python")
	t.Setenv("PATH", dir)

	if lookPath("python3.10+", "linux") {
		t.Error("lookPath(python3.10+, linux) = true, want false — no fallback to python outside windows")
	}
}

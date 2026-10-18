package serve

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLauncherIsPrivateAndRedirects(t *testing.T) {
	const target = "http://127.0.0.1:52411/#token=abc"
	path, cleanup, err := WriteLauncher(target)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if runtime.GOOS != "windows" {
		if st.Mode().Perm() != 0o600 {
			t.Errorf("launcher mode = %v, want 0600", st.Mode().Perm())
		}
		if d, _ := os.Stat(dir); d.Mode().Perm() != 0o700 {
			t.Errorf("launcher dir mode = %v, want 0700", d.Mode().Perm())
		}
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), target) {
		t.Errorf("launcher does not redirect to the URL: %s", body)
	}
	if !strings.HasSuffix(path, ".html") {
		t.Errorf("launcher %q must be an .html file so the browser opens it as a page", path)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup left the launcher behind: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup left the launcher directory behind: %v", err)
	}
	cleanup() // idempotente
}

func TestEachLauncherGetsItsOwnPrivateDirectory(t *testing.T) {
	a, cleanA, err := WriteLauncher("http://127.0.0.1:1/#token=a")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanA()
	b, cleanB, err := WriteLauncher("http://127.0.0.1:2/#token=b")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanB()
	if filepath.Dir(a) == filepath.Dir(b) {
		t.Errorf("two launchers share a directory: %q", filepath.Dir(a))
	}
}

func TestLauncherEscapesTheURLInTheHTML(t *testing.T) {
	// A URL é nossa, mas o arquivo é HTML: nada de aspas soltas dentro do atributo.
	path, cleanup, err := WriteLauncher(`http://127.0.0.1:1/#token="><script>x</script>`)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "<script>x</script>") {
		t.Errorf("launcher did not escape the URL: %s", body)
	}
}

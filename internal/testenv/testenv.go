// Package testenv concentra o que os testes decidem pelo ambiente em que
// rodam. Só testes o importam.
package testenv

import "os"

// RequireSymlinksEnv, definida, faz a falta de symlink falhar o teste em vez de
// pulá-lo. O CI a liga: o guard de symlink é a garantia de segurança do ray, e
// um pulo silencioso no runner a faria parecer medida onde nunca rodou.
const RequireSymlinksEnv = "RAY_REQUIRE_SYMLINKS"

// T é o recorte de testing.TB que SymlinkUnavailable usa; existe para o
// próprio comportamento ser testável sem derrubar o teste que o testa.
type T interface {
	Helper()
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// SymlinkUnavailable trata o erro de os.Symlink: pula o teste, ou o falha se o
// ambiente exige symlinks.
func SymlinkUnavailable(t T, err error) {
	t.Helper()
	if os.Getenv(RequireSymlinksEnv) != "" {
		t.Fatalf("symlinks are required here (%s is set) but unavailable: %v", RequireSymlinksEnv, err)
		return
	}
	t.Skipf("symlink not supported here: %v", err)
}

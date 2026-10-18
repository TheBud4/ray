package serve

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// launcherName é o nome do arquivo dentro do diretório privado. A extensão
// .html faz o navegador abri-lo como página.
const launcherName = "open.html"

// WriteLauncher grava um HTML 0600 num diretório 0700 que redireciona para url
// e devolve o caminho e a função que apaga tudo. Falha se o diretório não puder
// ser privado.
//
// É assim que o token chega ao navegador: quem abre o navegador recebe só o
// caminho deste arquivo, nunca a URL — o argv de um processo é legível por
// qualquer usuário da máquina em /proc/<pid>/cmdline, o arquivo não.
func WriteLauncher(url string) (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "ray-serve-")
	if err != nil {
		return "", nil, fmt.Errorf("creating the launcher directory: %w", err)
	}
	var once sync.Once
	cleanup = func() { once.Do(func() { _ = os.RemoveAll(dir) }) }

	if err := makePrivate(dir); err != nil {
		cleanup()
		return "", nil, err
	}

	path = filepath.Join(dir, launcherName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("creating the launcher: %w", err)
	}
	_, werr := f.WriteString(launcherPage(url))
	cerr := f.Close()
	if werr != nil || cerr != nil {
		cleanup()
		return "", nil, fmt.Errorf("writing the launcher: %w", firstErr(werr, cerr))
	}
	return path, cleanup, nil
}

// makePrivate deixa dir acessível só ao dono e confere que ficou. No Windows o
// modo Unix não se aplica: o diretório temporário do usuário já é dele.
func makePrivate(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("making the launcher directory private: %w", err)
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("checking the launcher directory: %w", err)
	}
	if !st.IsDir() || st.Mode().Perm() != 0o700 {
		return fmt.Errorf("the launcher directory %s is %v, not private (0700)", dir, st.Mode())
	}
	return nil
}

// launcherPage é a página que só redireciona: um meta refresh e, se o
// navegador não o seguir, o link. Sem script, e a URL escapada para o atributo.
func launcherPage(url string) string {
	u := html.EscapeString(url)
	return "<!doctype html>\n" +
		"<html><head><meta charset=\"utf-8\">\n" +
		"<meta name=\"referrer\" content=\"no-referrer\">\n" +
		"<meta http-equiv=\"refresh\" content=\"0; url=" + u + "\">\n" +
		"<title>ray serve</title></head>\n" +
		"<body><a href=\"" + u + "\">open ray serve</a></body></html>\n"
}

// firstErr devolve o primeiro erro não nulo.
func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

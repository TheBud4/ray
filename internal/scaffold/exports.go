package scaffold

import (
	"io/fs"

	"github.com/TheBud4/ray/internal/profile"
)

// EmbeddedTemplates devolve os templates embutidos no binário, com a raiz em
// templates/ — os mesmos nomes que o overlay usa.
func EmbeddedTemplates() fs.FS {
	sub, err := fs.Sub(embedded, templatesRoot)
	if err != nil {
		panic(err) // templatesRoot é constante e está embutido: não falha
	}
	return sub
}

// Render renderiza o template tmplName (overlay em templatesDir antes do
// embutido) com data.
func Render(tmplName, templatesDir string, data Data) ([]byte, error) {
	return render(tmplName, templatesDir, data)
}

// TemplateFor devolve o template padrão do arquivo path de uma receita.
func TemplateFor(path string) (string, bool) {
	name, ok := templateFor[path]
	return name, ok
}

// TemplateExists diz se tmplName resolve, no overlay ou no embutido, pela mesma
// leitura que WriteFiles faz.
func TemplateExists(tmplName, templatesDir string) bool {
	_, err := readTemplate(tmplName, templatesDir)
	return err == nil
}

// DedupFiles mantém base inteiro e acrescenta de extra só os paths ausentes em
// base — a receita "ganha". Com SystemFiles em extra é a lista que o init ai
// escreve.
func DedupFiles(base, extra []profile.ScaffoldFile) []profile.ScaffoldFile {
	seen := make(map[string]bool, len(base))
	out := make([]profile.ScaffoldFile, len(base))
	copy(out, base)
	for _, f := range base {
		seen[f.Path] = true
	}
	for _, f := range extra {
		if seen[f.Path] {
			continue
		}
		out = append(out, f)
		seen[f.Path] = true
	}
	return out
}

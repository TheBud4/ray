package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/scaffold"
)

// checkGitignore verifica se o bloco do ray segue inteiro. Negação removida é
// falha silenciosa: o conteúdo vendorizado volta a ser ignorado, o `git add`
// do rodapé passa a não adicionar nada, e nada avisa.
//
// Sem .gitignore nenhum não há problema a reportar: o projeto pode não usar
// git, e o próprio `ray init ai` cria o arquivo quando roda.
func checkGitignore(target string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	begin, end := scaffold.GitignoreMarkers()
	body := string(data)
	b := strings.Index(body, begin)
	if b < 0 {
		return []string{"the ray block is missing from .gitignore; vendored content is being ignored"}, nil
	}
	e := strings.Index(body[b:], end)
	if e < 0 {
		return []string{"the ray block in .gitignore has no closing marker"}, nil
	}
	block := body[b : b+e]

	var problems []string
	for _, want := range scaffold.GitignoreBaseLines() {
		if strings.TrimSpace(want) == "" || strings.HasPrefix(want, "#") {
			continue
		}
		if !strings.Contains(block, want) {
			problems = append(problems, fmt.Sprintf(".gitignore: the ray block is missing %s", want))
		}
	}
	return problems, nil
}

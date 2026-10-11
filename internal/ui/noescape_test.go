package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Só este pacote conhece escape ANSI: nenhum literal de string do código de
// produção fora de internal/ui contém ESC (\x1b, \033, \u001b). Os comandos
// dizem o que a linha é; quem pinta é o ui.
func TestNoProductionStringLiteralOutsideUIHoldsAnEscape(t *testing.T) {
	root := filepath.Join("..") // internal/
	fset := token.NewFileSet()
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Base(path) == "ui" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && strings.ContainsRune(s, 0x1b) {
				offenders = append(offenders, fset.Position(lit.Pos()).String())
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("string literals with an ESC outside internal/ui (use the ui roles instead):\n  %s", strings.Join(offenders, "\n  "))
	}
}

// Package safepath confere que um caminho de escrita, mesmo atravessando
// symlinks, continua dentro do projeto. Um repositório clonado pode trazer
// `.claude/.ray-profile`, `settings.json` ou diretórios inteiros como links
// para fora; gravar por cima deles escreveria fora do projeto. Só filesystem,
// sem rede e sem processo externo.
package safepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxHops limita quantos symlinks são seguidos numa resolução; passar disso
// (um laço, por exemplo) é erro.
const maxHops = 40

// ResolveInside confere que path, depois de seguir todo symlink que existe no
// caminho, fica na própria raiz ou estritamente abaixo dela. path é absoluto e
// deve estar sob root lexicalmente.
//
// Os componentes que não existem são aceitos (destino novo, pasta nova). Um
// symlink pendente é resolvido pelo texto do alvo, porque o os.WriteFile
// criaria o alvo: um link pendente para fora da raiz é recusado. O caminho
// completo nunca vai ao EvalSymlinks nem ao os.Root, pois ambos falham quando
// a cauda ainda não existe.
//
// A raiz é comparada já resolvida (EvalSymlinks), para que um projeto atrás de
// um symlink — como /tmp em alguns sistemas — não conte como fuga; se ela não
// existe, vale o caminho limpo.
func ResolveInside(root, path string) error {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)

	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || !within(rel) {
		return fmt.Errorf("%s is not under the project %s", path, root)
	}

	resolvedRoot := cleanRoot
	if r, err := filepath.EvalSymlinks(cleanRoot); err == nil {
		resolvedRoot = r
	}

	// cur é sempre um caminho já resolvido; pending, o que falta andar.
	cur := resolvedRoot
	pending := splitPath(rel)
	hops := 0
	var lastLink, lastTarget string

	for len(pending) > 0 {
		comp := pending[0]
		pending = pending[1:]

		switch comp {
		case ".", "":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}

		candidate := filepath.Join(cur, comp)
		info, err := os.Lstat(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
				cur = candidate
				continue
			}
			return fmt.Errorf("checking %s: %w", candidate, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			cur = candidate
			continue
		}

		hops++
		if hops > maxHops {
			return fmt.Errorf("%s: too many symbolic links (loop?)", path)
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", candidate, err)
		}
		lastLink, lastTarget = candidate, target

		if filepath.IsAbs(target) {
			vol := filepath.VolumeName(target)
			cur = vol + string(filepath.Separator)
			target = target[len(vol):]
		}
		// Alvo relativo parte do diretório do link, que é o cur atual.
		pending = append(splitPath(target), pending...)
	}

	rel, err = filepath.Rel(resolvedRoot, cur)
	if err != nil || !within(rel) {
		if lastLink != "" {
			if lastLink == cleanPath {
				return fmt.Errorf("%s is a symlink to %s, outside the project %s", lastLink, lastTarget, root)
			}
			return fmt.Errorf("%s is a symlink to %s, so %s leaves the project %s", lastLink, lastTarget, path, root)
		}
		return fmt.Errorf("%s resolves to %s, outside the project %s", path, cur, root)
	}
	return nil
}

// within informa se um caminho relativo (resultado de filepath.Rel) não sai
// do ponto de partida.
func within(rel string) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// splitPath quebra p em componentes, aceitando o separador da plataforma.
func splitPath(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == os.PathSeparator || r == '/' })
}

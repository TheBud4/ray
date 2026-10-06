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
// A raiz é comparada já resolvida, para que um projeto atrás de um symlink —
// como /tmp em alguns sistemas — não conte como fuga; a parte dela que ainda não
// existe fica como está.
func ResolveInside(root, path string) error {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)

	// A raiz pode ainda não existir (`ray new`, `init ai` numa pasta nova): o
	// EvalSymlinks falharia, então canonicaliza a parte que existe, do mesmo
	// jeito que o caminho é canonicalizado mais abaixo.
	resolvedRoot := canonical(cleanRoot)

	// O projeto pode ser alcançado por mais de um nome (/var → /private/var no
	// macOS, nome 8.3 no Windows), e quem chama pode ter passado a raiz por um e
	// o caminho por outro. Vale o nome que o caminho de fato usa; só se nenhum
	// dos dois serve é que se canonicaliza o caminho.
	rel, ok := relUnder(cleanRoot, cleanPath)
	if !ok {
		rel, ok = relUnder(resolvedRoot, cleanPath)
	}
	if !ok {
		rel, ok = relUnder(resolvedRoot, canonical(cleanPath))
	}
	if !ok {
		return fmt.Errorf("%s is not under the project %s", path, root)
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

	// cur pode ter cauda que não existe (link pendente) e, por isso, o nome do
	// projeto que o link guardou; canonicaliza o que existe antes de comparar.
	if _, ok := relUnder(resolvedRoot, canonical(cur)); !ok {
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

// relUnder devolve path relativo a root e se ele fica na própria raiz ou abaixo.
func relUnder(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || !within(rel) {
		return "", false
	}
	return rel, true
}

// canonical resolve os symlinks (e normaliza o nome) da parte de p que existe
// e acrescenta a cauda que ainda não existe, que o EvalSymlinks não aceita.
func canonical(p string) string {
	tail := ""
	for cur := filepath.Clean(p); ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, tail)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(p)
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
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

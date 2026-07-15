// Package store implementa o cache content-addressed de conteúdo de IA
// adquirido (I2): só filesystem, sem rede, agnóstico ao adquiridor
// (internal/acquire) — só sabe guardar e devolver bytes por coordenada.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Store é o cache em <root>/objects/<hash>/… + <root>/index.yaml
// (coordenada → hash).
type Store struct {
	root string
}

// New devolve um Store enraizado em root (ex. raypaths.StoreDir()). Não toca
// disco até Put/Get serem chamados.
func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) objectsDir() string { return filepath.Join(s.root, "objects") }
func (s *Store) indexPath() string  { return filepath.Join(s.root, "index.yaml") }

// Put hasheia o conteúdo de dir (por bytes, não por coord) e o guarda em
// <root>/objects/<hash> se ainda não existir; registra coord→hash no índice.
// Put do mesmo coord com o mesmo conteúdo é idempotente; conteúdo idêntico de
// coords diferentes compartilha um único objeto (dedup por bytes).
func (s *Store) Put(coord, dir string) (string, error) {
	hash, err := hashTree(dir)
	if err != nil {
		return "", err
	}

	objDir := filepath.Join(s.objectsDir(), hash)
	if _, err := os.Stat(objDir); os.IsNotExist(err) {
		if err := CopyTree(dir, objDir); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	index, err := s.loadIndex()
	if err != nil {
		return "", err
	}
	index[coord] = hash
	if err := s.saveIndex(index); err != nil {
		return "", err
	}
	return hash, nil
}

// Get resolve coord no índice e devolve o diretório do objeto correspondente.
func (s *Store) Get(coord string) (string, bool) {
	index, err := s.loadIndex()
	if err != nil {
		return "", false
	}
	hash, ok := index[coord]
	if !ok {
		return "", false
	}
	objDir := filepath.Join(s.objectsDir(), hash)
	if _, err := os.Stat(objDir); err != nil {
		return "", false
	}
	return objDir, true
}

// Has reporta se coord tem uma entrada resolvível no cache.
func (s *Store) Has(coord string) bool {
	_, ok := s.Get(coord)
	return ok
}

func (s *Store) loadIndex() (map[string]string, error) {
	index := map[string]string{}
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return index, nil
}

func (s *Store) saveIndex(index map[string]string) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o644)
}

// hashTree calcula um sha256 determinístico sobre (rel-path, conteúdo) de
// todo arquivo regular em dir, em ordem lexicográfica de rel-path.
func hashTree(dir string) (string, error) {
	var rels []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(rels)

	h := sha256.New()
	for _, rel := range rels {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		io.WriteString(h, rel)
		h.Write([]byte{0})
		h.Write(content)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyTree copia src (arquivo ou diretório, com estrutura + permissões) para
// dst. Exportada para reuso por internal/acquire (montagem do conteúdo
// adquirido) e internal/initai (restauração do store para o projeto).
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}

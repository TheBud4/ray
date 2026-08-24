// Package store guarda linha-base pristina (hash) de arquivo/árvore por
// coordenada — usado pelo scaffold (templates), initai/update (componentes
// locais) e status (detecção de fork) para decidir se algo foi editado à
// mão. Só filesystem, sem rede.
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

func (s *Store) objectsDir() string   { return filepath.Join(s.root, "objects") }
func (s *Store) indexPath() string    { return filepath.Join(s.root, "index.yaml") }
func (s *Store) pristinePath() string { return filepath.Join(s.root, "pristine.yaml") }

// Put hasheia o conteúdo de dir (por bytes, não por coord) e o guarda em
// <root>/objects/<hash> se ainda não existir; registra coord→hash no índice.
// Put do mesmo coord com o mesmo conteúdo é idempotente; conteúdo idêntico de
// coords diferentes compartilha um único objeto (dedup por bytes).
func (s *Store) Put(coord, dir string) (string, error) {
	hash, err := HashTree(dir)
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

// HashTree calcula um sha256 determinístico sobre (rel-path, conteúdo) de
// todo arquivo regular sob path, em ordem lexicográfica de rel-path. path
// pode ser um diretório ou um único arquivo (aitmpl entrega um .md solto),
// caso em que rel-path é só o nome-base do arquivo.
func HashTree(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}

	var rels []string
	if err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
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

// DecideOverwrite é a política de detecção de fork por conteúdo (design §6,
// §13): pura, sem tocar disco — recebe os hashes já calculados pelo chamador.
//   - force: sempre sobrescreve.
//   - sem conteúdo no disco ainda: sobrescreve (primeira instalação do leaf).
//   - com pristino conhecido: disco == pristino → sobrescreve (não editado);
//     disco != pristino → você editou → pula.
//   - sem pristino (clone novo, degradação graciosa): disco == fresco →
//     não é fork, sobrescreve; disco != fresco → ambíguo → pula.
//
// Usada pelo `ray update` (conteúdo vendorizado) e pelo overlay de templates
// em `scaffold.EnsureTemplates`. São dois domínios com a mesma pergunta — "o
// usuário editou isto?" — e a resposta não deve divergir entre eles.
func DecideOverwrite(force, onDiskExists bool, onDiskHash, freshHash, pristineHash string, hasPristine bool) (overwrite bool, reason string) {
	if force {
		return true, ""
	}
	if !onDiskExists {
		return true, ""
	}
	if hasPristine {
		if onDiskHash == pristineHash {
			return true, ""
		}
		return false, "edited locally (differs from last pristine); use --force to overwrite"
	}
	if onDiskHash == freshHash {
		return true, ""
	}
	return false, "edited locally (no pristine baseline; differs from upstream); use --force to overwrite"
}

// HashBytes é o sha256 hex de um conteúdo solto. HashTree existe para árvore
// vendorizada, onde o rel-path entra no hash; aqui o arquivo já é identificado
// pela chave do pristino, então só o conteúdo importa.
func HashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// PristineHash devolve o hash que o store gravou por último para proj×coord
// (I3: linha-base contra a qual `ray update` detecta fork por conteúdo), ou
// ok=false se nunca foi gravado.
func (s *Store) PristineHash(proj, coord string) (string, bool) {
	index, err := s.loadPristine()
	if err != nil {
		return "", false
	}
	byCoord, ok := index[proj]
	if !ok {
		return "", false
	}
	hash, ok := byCoord[coord]
	return hash, ok
}

// SetPristine grava o hash pristino de proj×coord, sobrescrevendo qualquer
// valor anterior.
func (s *Store) SetPristine(proj, coord, hash string) error {
	index, err := s.loadPristine()
	if err != nil {
		return err
	}
	if index[proj] == nil {
		index[proj] = map[string]string{}
	}
	index[proj][coord] = hash
	return s.savePristine(index)
}

func (s *Store) loadPristine() (map[string]map[string]string, error) {
	index := map[string]map[string]string{}
	data, err := os.ReadFile(s.pristinePath())
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

func (s *Store) savePristine(index map[string]map[string]string) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(s.pristinePath(), data, 0o644)
}

// CopyTree copia src (arquivo ou diretório, com estrutura + permissões) para
// dst. Exportada para reuso por internal/initai e internal/update (cópia de
// componente local pro projeto) e internal/scaffold (overlay de templates).
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

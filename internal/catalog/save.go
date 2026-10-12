package catalog

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/TheBud4/ray/internal/fsutil"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

var (
	// ErrStale recusa a gravação quando o template mudou no disco depois de lido.
	ErrStale = errors.New("the template changed on disk since it was loaded")
	// ErrReadOnly recusa a gravação de um hook pela web.
	ErrReadOnly = errors.New("hooks are read-only on the web; use `ray template edit`")
	// ErrNotUTF8 recusa um conteúdo que não é UTF-8 válido.
	ErrNotUTF8 = errors.New("the content is not valid UTF-8")
)

// SaveResult é o que uma gravação fez. Um template que não renderiza continua
// gravado: a falha volta em Render, não como erro.
type SaveResult struct {
	Outcome     Outcome
	State       State
	Render      *RenderError // não-nil quando o template gravado não renderiza
	Warnings    []string
	BaselineErr error
	Hash        string // a base nova: sha256 hex de Read(name) depois de gravar
}

// Save grava content como o template name, se o arquivo ainda é o que o cliente
// leu (baseHash, o Hash de Info). Nada é gravado quando uma conferência falha;
// elas vêm em ordem, das baratas às que leem o disco: nome da lista, hook,
// UTF-8, linha-base legível, symlink no caminho e base defasada. content é
// gravado como veio, com o fim de linha que o cliente restaurou.
func (t Templates) Save(name string, content []byte, baseHash string) (SaveResult, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return SaveResult{}, err
	}
	if IsHook(e.Name) {
		return SaveResult{}, ErrReadOnly
	}
	if !utf8.Valid(content) {
		return SaveResult{}, ErrNotUTF8
	}
	if t.Verify != nil {
		if err := t.Verify(); err != nil {
			return SaveResult{}, err
		}
	}
	if err := t.refuseSymlinkPath(e.Name); err != nil {
		return SaveResult{}, err
	}
	// before é a base que o cliente editou: o que o próximo `init ai` usaria,
	// igual ao overlay depois de Prepare.
	before, err := t.Read(e.Name)
	if err != nil {
		return SaveResult{}, err
	}
	if store.HashBytes(before) != baseHash {
		return SaveResult{}, ErrStale
	}

	_, baselineErr, err := t.Prepare(e.Name)
	if err != nil {
		return SaveResult{}, err
	}
	dest := filepath.Join(t.Dir, filepath.FromSlash(e.Name))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return SaveResult{}, err
	}
	if err := fsutil.WriteFileAtomic(dest, content, 0o644); err != nil {
		return SaveResult{}, err
	}

	res := SaveResult{BaselineErr: baselineErr}
	if res.Outcome, err = t.Classify(e.Name, before, content); err != nil {
		return res, err
	}
	// O conteúdo gravado é renderizado da memória, com o teto do Preview: um
	// template recursivo não esgota a memória de quem o grava.
	res.Warnings, err = validate(e.Name, func(d scaffold.Data) ([]byte, error) {
		return scaffold.RenderString(e.Name, content, d, PreviewCeiling)
	})
	if err != nil {
		res.Render = renderError(e.Name, err)
	}
	after, err := t.Lookup(e.Name)
	if err != nil {
		return res, err
	}
	res.State = after.State
	saved, err := t.Read(e.Name)
	if err != nil {
		return res, err
	}
	res.Hash = store.HashBytes(saved)
	return res, nil
}

// refuseSymlinkPath recusa name se ele ou qualquer diretório entre t.Dir e ele
// for symlink, em qualquer estado: a web não grava através de link. t.Dir em si
// pode ser link (um gerenciador de dotfiles). Um componente ausente encerra a
// busca, porque nada abaixo dele existe.
func (t Templates) refuseSymlinkPath(name string) error {
	rel := ""
	for seg := range strings.SplitSeq(name, "/") {
		rel = path.Join(rel, seg)
		fi, err := os.Lstat(filepath.Join(t.Dir, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return t.symlinkError(rel)
		}
	}
	return nil
}

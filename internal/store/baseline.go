package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/fsutil"
)

// BaselineFile é o caminho, relativo ao projeto, da linha-base pristina dele.
const BaselineFile = ".claude/.ray-pristine.yaml"

const baselineHeader = "# Gerado pelo ray: hash do que ele escreveu por último em cada componente.\n" +
	"# É contra isto que `ray update` e `ray status` decidem se você editou algo.\n"

// Baseline é a linha-base pristina de um projeto: o hash do que o ray escreveu
// por último em cada componente. Mora no próprio projeto e é versionada com ele,
// então mover a pasta ou clonar o repositório não a perde — ao contrário da do
// Store, indexada por caminho absoluto numa pasta da máquina.
type Baseline struct {
	path string
}

type baselineDoc struct {
	Components map[string]string `yaml:"components"`
}

// ProjectBaseline devolve a linha-base do projeto em target. Não toca disco
// até a primeira leitura ou gravação.
func ProjectBaseline(target string) *Baseline {
	return &Baseline{path: filepath.Join(target, filepath.FromSlash(BaselineFile))}
}

// Path é onde o arquivo mora.
func (b *Baseline) Path() string { return b.path }

// PristineHash devolve o hash gravado para o componente coord, ou ok=false se
// não há (arquivo ausente, ou componente nunca gravado). Um arquivo ilegível
// também dá ok=false: quem decide algo a partir daqui chama Verify antes.
func (b *Baseline) PristineHash(coord string) (string, bool) {
	doc, err := b.load()
	if err != nil {
		return "", false
	}
	hash, ok := doc.Components[coord]
	return hash, ok
}

// SetPristine grava o hash de coord. Não regrava o arquivo quando o conteúdo
// final é o mesmo, e recusa gravar sobre um arquivo ilegível — "consertá-lo" em
// silêncio perderia o que ele guardava.
func (b *Baseline) SetPristine(coord, hash string) error {
	doc, err := b.load()
	if err != nil {
		return b.unreadable(err)
	}
	if doc.Components == nil {
		doc.Components = map[string]string{}
	}
	doc.Components[coord] = hash

	body, err := yaml.Marshal(doc) // as chaves do mapa saem ordenadas
	if err != nil {
		return err
	}
	data := append([]byte(baselineHeader), body...)

	if cur, err := os.ReadFile(b.path); err == nil && bytes.Equal(cur, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(b.path, data, 0o644)
}

// Verify diz se o arquivo é legível. Ausente é válido; ilegível não é, e quem
// vai ler ou gravar linha-base o chama antes de qualquer efeito.
func (b *Baseline) Verify() error {
	if _, err := b.load(); err != nil {
		return b.unreadable(err)
	}
	return nil
}

func (b *Baseline) load() (baselineDoc, error) {
	var doc baselineDoc
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return doc, err
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return baselineDoc{}, err
	}
	return doc, nil
}

func (b *Baseline) unreadable(err error) error {
	return fmt.Errorf("%s is unreadable (%v); delete it to start over — every component then reads as unknown provenance until the next `ray init ai` or `ray update`", b.path, err)
}

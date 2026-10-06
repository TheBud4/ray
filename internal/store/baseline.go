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
	target string
	path   string

	// legacy é o store da máquina, onde projetos montados antes de a linha-base
	// ir para o projeto a guardaram, indexada por caminho absoluto (abs). Só é
	// lido; WithLegacy o liga.
	legacy *Store
	abs    string
}

type baselineDoc struct {
	Components map[string]string `yaml:"components"`
}

// ProjectBaseline devolve a linha-base do projeto em target. Não toca disco
// até a primeira leitura ou gravação.
func ProjectBaseline(target string) *Baseline {
	return &Baseline{target: target, path: filepath.Join(target, filepath.FromSlash(BaselineFile))}
}

// Path é onde o arquivo mora.
func (b *Baseline) Path() string { return b.path }

// WithLegacy liga a leitura de fallback do store da máquina: um projeto montado
// antes da linha-base ir para o projeto continua conhecido, e a primeira
// gravação (ou Promote) leva as entradas dele para o arquivo do projeto.
func (b *Baseline) WithLegacy(s *Store) *Baseline {
	abs, err := filepath.Abs(b.target)
	if err != nil {
		abs = b.target
	}
	return &Baseline{target: b.target, path: b.path, legacy: s, abs: abs}
}

// PristineHash devolve o hash gravado para o componente coord, ou ok=false se
// não há (arquivo ausente, ou componente nunca gravado). Um arquivo ilegível
// também dá ok=false: quem decide algo a partir daqui chama Verify antes.
func (b *Baseline) PristineHash(coord string) (string, bool) {
	doc, err := b.load()
	if err != nil {
		return "", false
	}
	if hash, ok := doc.Components[coord]; ok {
		return hash, true
	}
	if b.legacy != nil {
		return b.legacy.PristineHash(b.abs, coord)
	}
	return "", false
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
	b.absorbLegacy(&doc)
	doc.Components[coord] = hash
	return b.write(doc)
}

// Promote leva para o arquivo do projeto as entradas que o store da máquina tem
// para ele e o arquivo ainda não tem. Sem nada a levar, não cria nem regrava
// nada. É o que faz um projeto antigo, em que nenhum componente mudou, passar a
// carregar a própria linha-base mesmo assim.
func (b *Baseline) Promote() error {
	doc, err := b.load()
	if err != nil {
		return b.unreadable(err)
	}
	if doc.Components == nil {
		doc.Components = map[string]string{}
	}
	if !b.absorbLegacy(&doc) {
		return nil
	}
	return b.write(doc)
}

// absorbLegacy acrescenta a doc as entradas do store que doc ainda não tem, e
// diz se acrescentou alguma. A do projeto sempre vence a antiga.
func (b *Baseline) absorbLegacy(doc *baselineDoc) bool {
	if b.legacy == nil {
		return false
	}
	added := false
	for coord, hash := range b.legacy.ProjectEntries(b.abs) {
		if _, ok := doc.Components[coord]; !ok {
			doc.Components[coord] = hash
			added = true
		}
	}
	return added
}

// write grava doc, a menos que o conteúdo final seja o que já está no disco.
func (b *Baseline) write(doc baselineDoc) error {
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
// vai ler ou gravar linha-base o chama antes de qualquer efeito. O store da
// máquina só entra enquanto o projeto não tem arquivo próprio: depois disso ele
// não é mais consultado, e um store ilegível não bloqueia o projeto.
func (b *Baseline) Verify() error {
	if _, err := b.load(); err != nil {
		return b.unreadable(err)
	}
	if b.legacy != nil {
		if _, err := os.Stat(b.path); os.IsNotExist(err) {
			return b.legacy.Verify()
		}
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

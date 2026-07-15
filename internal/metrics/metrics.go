// Package metrics agrega os proxies de atividade que os mecanismos de Token
// Economy (internal/economy) deixam em disco (design §8.3, plano §I5). Só
// lê: quem escreve é o hook de sessão em bash, não este binário — o `ray`
// nunca está rodando durante uma sessão do Claude Code.
package metrics

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const countSuffix = ".count"

// Dir devolve o diretório de métricas de target (ignorado no git, I1).
func Dir(target string) string {
	return filepath.Join(target, ".claude", ".ray-metrics")
}

// Aggregate lê todo arquivo "<key>.count" diretamente sob dir e devolve
// key→contagem. dir ausente é estado válido (nunca houve atividade) — mapa
// vazio, sem erro. Um arquivo individual ilegível ou não-numérico é
// silenciosamente ignorado: leitura best-effort de estado externo, não uma
// fonte que o `ray` controla ponta-a-ponta.
func Aggregate(dir string) (map[string]int, error) {
	counts := map[string]int{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return counts, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, countSuffix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(name, countSuffix)
		counts[key] = n
	}

	return counts, nil
}

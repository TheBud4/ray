package status

import (
	"os"
	"strings"

	"github.com/TheBud4/ray/internal/profile"
)

// Facts são os fatos baratos do ambiente vendorizado: o que se sabe lendo
// arquivo, sem subprocesso. É o que a tela de `ray` sem subcomando mostra —
// ela orienta, não diagnostica, e por isso não paga git nem MCP.
//
// Compartilha o countInventory com o Run de propósito: a regra de contagem é
// uma pergunta só ("o que é uma skill?") e duas respostas divergiriam.
type Facts struct {
	HasEnvironment bool
	Profile        string // nome registrado em .claude/.ray-profile; vazio se ausente
	Inventory      Inventory
}

// ReadFacts lê os fatos baratos de target. Devolve erro só em falha real de
// leitura: ambiente ausente é HasEnvironment falso, não erro.
func ReadFacts(target string) (Facts, error) {
	var f Facts

	if _, err := os.Stat(claudeDir(target)); err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return Facts{}, err
	}
	f.HasEnvironment = true

	inv, err := countInventory(target)
	if err != nil {
		return Facts{}, err
	}
	f.Inventory = inv

	// O nome sai do registro, não da receita: carregá-la exigiria o
	// ProfilesDir, e o Report.Profile do Run já é esse outro valor.
	data, err := os.ReadFile(profile.ProfileRecordPath(target))
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return Facts{}, err
	}
	f.Profile = strings.TrimSpace(string(data))
	return f, nil
}

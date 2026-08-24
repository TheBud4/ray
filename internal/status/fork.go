package status

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/store"
)

// checkForks compara cada componente vendorizado com a linha-base pristina
// gravada pelo `ray init ai`, e diz o que o `ray update` faria com ele.
//
// Não chama store.DecideOverwrite: ela pede o hash upstream, que só se obtém
// re-adquirindo o componente — isto é, indo à rede. Com pristino presente a
// decisão dela é exatamente `disco == pristino`, que é o que fazemos aqui;
// sem pristino ela precisaria do upstream, e é por isso que esse caso vira
// ForkUnknown em vez de um palpite.
//
// O terceiro retorno são problemas para o Report, não erros: falhar o comando
// por causa de uma receita ilegível contradiria o exit code 0 do status.
func checkForks(target string, home Home) (string, []ComponentState, []string, error) {
	// Sem registro de perfil não há o que comparar, e isso é normal: um
	// .claude/ pode ter sido copiado à mão. Com o registro presente, porém, o
	// ray montou este ambiente — aí uma receita que não carrega é achado, não
	// silêncio. Distinguir os dois exige olhar o registro antes: LoadForTarget
	// devolve um erro só para os dois casos.
	if _, err := os.Stat(profile.ProfileRecordPath(target)); err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil, nil
		}
		return "", nil, nil, err
	}

	prof, err := profile.LoadForTarget(home.ProfilesDir, target, "")
	if err != nil {
		return "", nil, []string{fmt.Sprintf("recorded profile could not be loaded: %v", err)}, nil
	}

	st := store.New(home.StoreDir)
	var out []ComponentState

	for _, c := range prof.Components {
		path := filepath.Join(target, c.Dest, c.Name)

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", nil, nil, err
		}

		pristine, hasPristine := st.PristineHash(target, c.Name)
		if !hasPristine {
			out = append(out, ComponentState{Coord: c.Name, State: ForkUnknown})
			continue
		}
		onDisk, err := store.HashTree(path)
		if err != nil {
			return "", nil, nil, err
		}
		state := ForkEdited
		if onDisk == pristine {
			state = ForkPristine
		}
		out = append(out, ComponentState{Coord: c.Name, State: state})
	}
	return prof.Name, out, nil, nil
}

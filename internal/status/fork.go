package status

import (
	"os"
	"path/filepath"

	"github.com/TheBud4/ray/internal/acquire"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
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
func checkForks(check runner.Runner, target string, home Home) (string, []ComponentState, error) {
	prof, err := profile.LoadForTarget(home.ProfilesDir, target, "")
	if err != nil {
		// Sem receita registrada não há o que comparar. Não é erro do
		// comando: um .claude/ pode ter sido copiado à mão.
		return "", nil, nil
	}

	st := store.New(home.StoreDir)
	var out []ComponentState

	for _, c := range prof.Components {
		acq, ok := acquire.For(c, check)
		if !ok {
			continue
		}
		coord := acq.Key(c)
		destRel, err := acquire.DestRel(c)
		if err != nil {
			return "", nil, err
		}
		leaf, err := acquire.LeafName(c)
		if err != nil {
			return "", nil, err
		}
		path := filepath.Join(target, destRel, leaf)

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", nil, err
		}

		pristine, hasPristine := st.PristineHash(target, coord)
		if !hasPristine {
			out = append(out, ComponentState{Coord: coord, State: ForkUnknown})
			continue
		}
		onDisk, err := store.HashTree(path)
		if err != nil {
			return "", nil, err
		}
		state := ForkEdited
		if onDisk == pristine {
			state = ForkPristine
		}
		out = append(out, ComponentState{Coord: coord, State: state})
	}
	return prof.Name, out, nil
}

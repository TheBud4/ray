package status

import (
	"os"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

// ReadFacts conta pelo mesmo critério do Run: skill é um SKILL.md, agente e
// comando são um .md, e a contagem desce em subdiretório. Se as duas
// divergirem, a tela de abertura e o `ray status` passam a mentir uma sobre a
// outra — é exatamente a dívida que o gitScope fixo tinha contra a whitelist.
func TestReadFactsCountsTheSameWayRunDoes(t *testing.T) {
	t.Setenv("RAY_BRAIN", "")
	target := newTarget(t,
		[]string{"tdd/SKILL.md", "brainstorm/SKILL.md", "README.md"},
		[]string{"reviewer.md", "notas.txt"},
		[]string{"revisar.md", "frontend/component.md"})

	f, err := ReadFacts(target)
	if err != nil {
		t.Fatalf("ReadFacts() error = %v", err)
	}
	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if f.Inventory.Skills != rep.Inventory.Skills ||
		f.Inventory.Agents != rep.Inventory.Agents ||
		f.Inventory.Commands != rep.Inventory.Commands {
		t.Errorf("ReadFacts = %+v, Run = %+v; the two counts must not diverge",
			f.Inventory, rep.Inventory)
	}
	want := Inventory{Skills: 2, Agents: 1, Commands: 2}
	if f.Inventory != want {
		t.Errorf("Inventory = %+v, want %+v", f.Inventory, want)
	}
}

// Sem .claude/ não há ambiente. Não é erro: é o caso normal de quem acabou de
// instalar o ray e está numa pasta qualquer.
func TestReadFactsWithoutAnEnvironment(t *testing.T) {
	f, err := ReadFacts(t.TempDir())
	if err != nil {
		t.Fatalf("ReadFacts() error = %v", err)
	}
	if f.HasEnvironment {
		t.Error("HasEnvironment = true, want false without a .claude/")
	}
	if f.Inventory != (Inventory{}) {
		t.Errorf("Inventory = %+v, want the zero value", f.Inventory)
	}
	if f.Profile != "" {
		t.Errorf("Profile = %q, want empty", f.Profile)
	}
}

// Ambiente copiado à mão: existe, tem conteúdo, e não tem registro de perfil.
func TestReadFactsWithoutARecordedProfile(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)

	f, err := ReadFacts(target)
	if err != nil {
		t.Fatalf("ReadFacts() error = %v", err)
	}
	if !f.HasEnvironment {
		t.Error("HasEnvironment = false, want true — .claude/ is there")
	}
	if f.Profile != "" {
		t.Errorf("Profile = %q, want empty without a .ray-profile", f.Profile)
	}
	if f.Inventory.Skills != 1 {
		t.Errorf("Inventory.Skills = %d, want 1", f.Inventory.Skills)
	}
}

// O nome vem do registro, cru. Não carrega a receita: isso exigiria ProfilesDir
// e uma leitura a mais, e a tela não precisa dos componentes.
func TestReadFactsReadsTheRecordedProfileName(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	if err := os.WriteFile(profile.ProfileRecordPath(target), []byte("go-backend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := ReadFacts(target)
	if err != nil {
		t.Fatalf("ReadFacts() error = %v", err)
	}
	if f.Profile != "go-backend" {
		t.Errorf("Profile = %q, want %q", f.Profile, "go-backend")
	}
}

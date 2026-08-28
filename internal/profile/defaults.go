package profile

// Defaults devolve os perfis embutidos do ray (go, web, flutter), escritos em
// ~/.ray/profiles na primeira execução. Todos editáveis depois.
//
// Nenhum declara `components:`. O ray não baixa nem embute conteúdo de
// terceiros — quem quiser um skill/agent cria a pasta em
// internal/raypaths.ComponentsDir() e acrescenta a entrada `name`/`dest` na
// receita à mão; o default não tem como adivinhar o que já existe lá.
func Defaults() []Profile {
	return []Profile{goProfile(), webProfile(), flutterProfile()}
}

// allIntegrations liga as capacidades (postura default).
func allIntegrations() Integrations {
	return Integrations{Headroom: true, CodeGraph: true}
}

// defaultSettings é o bloco de settings compartilhado do .claude.
func defaultSettings() map[string]any {
	return map[string]any{"model": "opus", "effortLevel": "high"}
}

// baseScaffoldFiles é o conjunto de orientação (docs/features.md) que todo
// perfil default escreve: CLAUDE.md, SECURITY.md, docs/, regras e o
// comando /document. Os templates que renderizam cada path vivem em
// internal/scaffold.
func baseScaffoldFiles() []ScaffoldFile {
	paths := []string{
		"CLAUDE.md",
		"SECURITY.md",
		"docs/README.md",
		"docs/architecture.md",
		"docs/conventions.md",
		".claude/commands/document.md",
		".claude/commands/handoff.md",
		".claude/commands/revisar.md",
		".claude/commands/destilar.md",
		".claude/handoff.md",
	}
	files := make([]ScaffoldFile, len(paths))
	for i, p := range paths {
		files[i] = ScaffoldFile{Path: p}
	}
	return files
}

// build monta um perfil default a partir de suas partes.
func build(name, desc string, create []string, gitignoreStack []string) Profile {
	return Profile{
		Name:         name,
		Description:  desc,
		Integrations: allIntegrations(),
		Scaffold: Scaffold{
			Files:          baseScaffoldFiles(),
			Settings:       defaultSettings(),
			GitignoreStack: gitignoreStack,
		},
		Create: create,
	}
}

func goProfile() Profile {
	return build("go", "Go backend stack",
		[]string{"go mod init {{.ProjectName}}"},
		[]string{"/{{.ProjectName}}"})
}

func webProfile() Profile {
	return build("web", "Next.js web stack", []string{"npx create-next-app@latest . --yes"},
		[]string{"node_modules/", ".next/"})
}

func flutterProfile() Profile {
	return build("flutter", "Flutter mobile stack", []string{"flutter create ."},
		[]string{".dart_tool/", "build/"})
}

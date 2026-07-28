package profile

// Defaults devolve os perfis embutidos do ray (go, web, flutter), escritos em
// ~/.ray/profiles na primeira execução. Todos editáveis depois.
func Defaults() []Profile {
	return []Profile{goProfile(), webProfile(), flutterProfile()}
}

// skill constrói um componente `via: skills`.
func skill(source, name string) Component {
	return Component{Via: ViaSkills, Source: source, Skill: name}
}

// skillsFrom constrói vários componentes `via: skills` de uma mesma fonte.
func skillsFrom(source string, names ...string) []Component {
	out := make([]Component, 0, len(names))
	for _, n := range names {
		out = append(out, skill(source, n))
	}
	return out
}

// agent constrói um componente `via: aitmpl, type: agent`.
func agent(ref string) Component {
	return Component{Via: ViaAitmpl, Type: TypeAgent, Ref: ref}
}

// baseComponents são compartilhados por todo perfil default.
func baseComponents() []Component {
	return []Component{
		skill("jeffallan/claude-skills", "prompt-engineer"),
		agent("development-tools/code-reviewer"),
		agent("development-tools/debugger"),
		skill("github/awesome-copilot", "documentation-writer"),
	}
}

// allIntegrations liga as três capacidades (postura default).
func allIntegrations() Integrations {
	return Integrations{Headroom: true, Brain: true, CodeGraph: true}
}

// defaultSettings é o bloco de settings compartilhado do .claude.
func defaultSettings() map[string]any {
	return map[string]any{"model": "opus", "effortLevel": "high"}
}

// baseScaffoldFiles é o conjunto de orientação (§7 do build guide) que todo
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
		"docs/specs/TEMPLATE.md",
		".claude/commands/document.md",
		".claude/commands/handoff.md",
		".claude/commands/revisar.md",
		".claude/handoff.md",
	}
	files := make([]ScaffoldFile, len(paths))
	for i, p := range paths {
		files[i] = ScaffoldFile{Path: p}
	}
	return files
}

// build monta um perfil default a partir de suas partes.
func build(name, desc string, create []string, extra []Component, gitignoreStack []string) Profile {
	return Profile{
		Name:         name,
		Description:  desc,
		Integrations: allIntegrations(),
		Components:   append(baseComponents(), extra...),
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
		[]string{"go mod init {{.Name}}"},
		skillsFrom("samber/cc-skills-golang",
			"golang-code-style", "golang-error-handling", "golang-design-patterns",
			"golang-performance", "golang-testing", "golang-security", "golang-documentation"),
		[]string{"/{{.ProjectName}}"})
}

func webProfile() Profile {
	var extra []Component
	extra = append(extra, skillsFrom("vercel-labs/agent-skills",
		"react-best-practices", "composition-patterns", "writing-guidelines", "web-design-guidelines")...)
	extra = append(extra, skillsFrom("anthropics/skills", "frontend-design", "webapp-testing")...)
	extra = append(extra, skill("hoodini/ai-agents-skills", "owasp-security"))
	return build("web", "Next.js web stack", []string{"npx create-next-app@latest . --yes"}, extra,
		[]string{"node_modules/", ".next/"})
}

func flutterProfile() Profile {
	var extra []Component
	extra = append(extra, skillsFrom("flutter/skills",
		"flutter-apply-architecture-best-practices", "flutter-fix-layout-issues",
		"flutter-setup-declarative-routing", "flutter-add-widget-test",
		"flutter-add-integration-test", "flutter-build-responsive-layout")...)
	extra = append(extra, skill("firebase/agent-skills", "firebase-security-rules-auditor"))
	extra = append(extra, skill("leonxlnx/taste-skill", "imagegen-frontend-mobile"))
	return build("flutter", "Flutter mobile stack", []string{"flutter create ."}, extra,
		[]string{".dart_tool/", "build/"})
}

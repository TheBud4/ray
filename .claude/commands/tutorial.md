---
description: Gera um tutorial 0→100% deste projeto, calibrado para nível iniciante
---

Gere um documento-tutorial completo deste projeto (o `ray`), salvo em
`docs/tutorial.md` (crie `docs/` se não existir). Este é um comando de
**geração dentro da sessão** — você (a IA) escreve o conteúdo agora; não existe
currículo pré-pronto embutido na ferramenta.

Calibragem: **nível iniciante**. Assuma pouca familiaridade com Go e com os
conceitos específicos deste design (structs, interfaces, `go:embed`, testes de
tabela, Cobra, hooks de `PreToolUse`/`SessionStart`). Explique fundamentos
antes de avançar.

Estrutura sugerida:

1. **Fundamentos** — o que é o `ray`, por que existe (ver `ray-build-guide.md`
   §1 e `ray-reproducible-environments-design.md` §1-3), conceitos de Go que
   vão aparecer.
2. **Fases** — quebre a reconstrução da CLI em fases pequenas e sequenciais,
   cada uma terminando num estado que compila e (quando fizer sentido) passa
   em teste. Baseie-se na ordem de incrementos de
   `ray-reproducible-environments-plan.md` §0 quando for além do que já existe.
3. **"Pronto quando…"** — cada fase termina com um critério objetivo e
   verificável (ex.: `go build ./...`, `go test ./...`), no mesmo espírito dos
   `milestones` de `ray-reproducible-environments-design.md` §9.3. Se o
   projeto tiver `milestones` declarados em algum lugar, referencie-os como
   checkpoints em vez de inventar novos.
4. Feche com um "próximo passo" claro para retomar depois.

Não implemente nada — este comando só produz o documento. A implementação
acontece nas sessões seguintes, seguindo `.claude/rules/learn-teaching.md`
(escada de dicas, nível iniciante).

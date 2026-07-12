---
description: Resume o estado do projeto ray em relação ao plano de fases e sugere o próximo passo
---

Você foi invocado para orientar sobre onde o projeto `ray` está no plano de
construção. Siga estes passos, na ordem:

1. Leia `docs/ray-build-guide.md`, seção "## 14. Plano de fases (ordem de
   construção)" — a lista de Fases 0 a 12, cada uma com o que entrega.
2. Leia `docs/ray-reproducible-environments-plan.md` — os incrementos I1 a
   I9, que só passam a fazer sentido depois que o build guide (Fases 0–12)
   estiver implementado.
3. Rode `git log --oneline` e liste os arquivos existentes em `internal/`
   (`find internal -type f`). Cruze as mensagens de commit e os pacotes
   existentes com as fases do build guide para inferir o que já está pronto.
4. Rode `go build ./...` e `go test ./...` para confirmar que o estado atual
   está verde antes de sugerir o próximo passo.
5. Devolva um resumo curto e objetivo, nesta ordem:
   - **Fase atual concluída** (número + nome, ex. "Fase 1 — runner").
   - **O que já está pronto** (lista curta de pacotes/arquivos existentes e
     por que contam como aquela fase).
   - **Próximo passo concreto** (a próxima fase/incremento, o pacote a criar,
     e o que ele precisa fazer, citando a seção correspondente do documento
     fonte).
6. **Não implemente nada.** Se o próximo passo for não-trivial (3+ passos ou
   decisão arquitetural — ver `CLAUDE.md`, "Plan Node Default"), termine
   sugerindo entrar em plan mode para ele. Este comando é só diagnóstico.

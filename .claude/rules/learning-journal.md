# Diário de aprendizado — estrutura

O diário é **pessoal**, não vendorizado: vive em
`.claude/.local/learning-journal.md` (fora do git, ver `.gitignore`). Só o
**head** é injetado em cada nova sessão (por `.claude/hooks/session-start.sh`)
— o log completo nunca entra no contexto, para não inchar custo de token
sessão após sessão.

## O que capturar — *deltas de entendimento*

- Conceitos que o usuário sacou (o "aha" em uma linha)
- O que ainda está nebuloso
- Equívocos corrigidos (vindos da checagem de compreensão pós-marco)
- O próximo passo

## O que **não** capturar

Comandos rodados, arquivos tocados, o blow-by-blow da sessão — isso é
transcript, não aprendizado. Se parece um log de atividade, não pertence
aqui.

## Formato do arquivo local

```markdown
## Head (sempre reescrito — é isso que entra no contexto)
Onde você está: <dominado> · <em aberto> · <próximo passo>

---

## Log (append-only — não injetado, só para você reler)

### 2026-07-02
- Entendi: ...
- Ainda nebuloso: ...
- Equívoco corrigido: ...
```

Ao final de cada sessão relevante (ou ao cruzar um marco), **reescreva o
Head** e **acrescente uma entrada no Log** — nunca edite entradas antigas do
Log.

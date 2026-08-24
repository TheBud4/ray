#!/usr/bin/env bash
# PostToolUse(Write|Edit|MultiEdit): avisa — nunca bloqueia — quando
# .claude/handoff.md passa do dobro do orçamento (~40 linhas, ver
# claude/commands/handoff.md.tmpl).
#
# É PostToolUse, ao contrário dos outros guards: o que importa aqui é o
# tamanho final do arquivo no disco, e Edit/MultiEdit não carregam o
# conteúdo inteiro no payload — só o trecho alterado. Rodar antes da escrita
# não daria para contar linha nenhuma.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

payload="$(cat || true)"
file="$(jq -r '.tool_input.file_path // empty' <<<"$payload" 2>/dev/null || true)"

if [[ -z "$file" || "$file" != *.claude/handoff.md ]]; then
  exit 0
fi

if [[ ! -f "$file" ]]; then
  exit 0
fi

# O dobro do alvo, não o alvo em si: ~40 é orientação, não teto rígido, e
# avisar em toda sessão por uma variação normal de +-10 linhas é o tipo de
# ruído que faz um hook de aviso deixar de ser lido.
budget=80
lines="$(wc -l < "$file" | tr -d ' ')"

if (( lines > budget )); then
  # `|| true` é obrigatório: sob `set -e`, um jq que falhe aqui faria o hook
  # sair não-zero e bloquear — exatamente o que ele não tem direito de fazer.
  jq -n --arg lines "$lines" --arg budget "$budget" \
    '{systemMessage: ("guard-handoff: .claude/handoff.md tem " + $lines + " linhas (orçamento ~40, aviso a partir de " + $budget + "). Regenere com /handoff em vez de acrescentar — ele deriva do repo, não narra de memória.")}' || true
fi

exit 0

#!/usr/bin/env bash
# PreToolUse(Edit|Write|MultiEdit): avisa — nunca bloqueia — quando um plano ou
# design está sendo escrito dentro do repositório. Eles descrevem trabalho a
# fazer, que é processo, e o repositório carrega estado atual.
#
# É PreToolUse, ao contrário do guard-vocab: o aviso chega antes da escrita, e
# aí ainda dá para redirecionar. Avisar depois não redireciona nada.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

payload="$(cat || true)"
file="$(jq -r '.tool_input.file_path // empty' <<<"$payload" 2>/dev/null || true)"

if [[ -n "$file" && "$file" == *superpowers/* ]]; then
  # `|| true` é obrigatório: sob `set -e`, um jq que falhe aqui faria o hook sair
  # não-zero e bloquear — exatamente o que ele não tem direito de fazer.
  jq -n '{systemMessage: "guard-plans: plano e design são processo e moram no cérebro, não no repositório. Escreva na pasta do projeto na vault — ver <documentation_sources> no CLAUDE.md."}' || true
fi

exit 0

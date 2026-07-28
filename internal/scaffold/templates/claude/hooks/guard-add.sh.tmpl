#!/usr/bin/env bash
# PreToolUse(Bash): avisa — nunca bloqueia — quando o comando faz `git add`
# cego (-A, --all ou `.`). O add seletivo é o que impede mudança de outra
# pessoa de entrar no commit sem ninguém ver.
set -euo pipefail

# Sem jq não há como ler o payload. No-op silencioso: um hook de aviso nunca
# vira erro.
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

payload="$(cat || true)"
cmd="$(jq -r '.tool_input.command // empty' <<<"$payload" 2>/dev/null || true)"

if [[ -n "$cmd" ]] && grep -qE 'git[[:space:]]+add[[:space:]]+(-A|--all|\.)([[:space:]]|$)' <<<"$cmd"; then
  jq -n '{systemMessage: "guard-add: use `git add` seletivo. `-A`, `--all` e `.` arrastam mudança que pode não ser sua — adicione os arquivos por nome e confira com `git status` antes."}'
fi

exit 0

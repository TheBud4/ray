#!/usr/bin/env bash
# PostToolUse(Edit|Write): avisa — nunca bloqueia — quando vocabulário de
# processo vaza para artefato entregue. Decisão de arquitetura entra como fato
# ("a navegação usa X"), nunca como "por causa da spec Y": quem clona o repo
# não tem como resolver a referência.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

payload="$(cat || true)"
file="$(jq -r '.tool_input.file_path // empty' <<<"$payload" 2>/dev/null || true)"

if [[ -z "$file" || ! -f "$file" ]]; then
  exit 0
fi

rel="${file#"$PWD"/}"

# Isentos: teste (onde CA-NN é convenção obrigatória) e processo interno.
case "$rel" in
  *_test.*|test/*|tests/*|*/test/*|*/tests/*|CLAUDE.md|*/CLAUDE.md|.claude/*|*/.claude/*)
    exit 0
    ;;
esac

# `spec` exige número na sequência — "OpenAPI spec" e "spec-driven" passam.
hits="$(grep -nE 'spec [0-9]|CA-[0-9]|RF-[0-9]|critério de aceite' "$file" 2>/dev/null || true)"

if [[ -n "$hits" ]]; then
  {
    echo "guard-vocab: vocabulário de processo em artefato entregue ($rel):"
    echo "$hits"
    echo "Reescreva como fato. Se o arquivo não é entregue, ele pertence ao cérebro."
  } >&2
  exit 2
fi

exit 0

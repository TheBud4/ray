#!/usr/bin/env bash
# PostToolUse(Edit|Write|MultiEdit): avisa — nunca bloqueia — quando
# vocabulário de processo vaza para artefato entregue. Decisão de arquitetura
# entra como fato ("a navegação usa X"), nunca como "por causa da spec Y":
# quem clona o repo não tem como resolver a referência.
#
# O aviso sai por `systemMessage` e o hook sai 0, como o guard-add e o
# guard-plans. Sair 2 devolvia o achado como erro e interrompia o turno — e
# como a varredura é do arquivo inteiro, não do que a edição acrescentou,
# qualquer edição num arquivo com vocabulário antigo era interrompida por
# linha que o autor da edição não escreveu. O guard-code é o único da família
# com direito de bloquear, e ele é explícito sobre isso.
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

# Isentos: teste (onde CA-NN é convenção obrigatória), processo interno e
# .superpowers/ — workspace de sessão local, gitignorado, onde a nota de
# processo é o conteúdo legítimo, não um vazamento.
#
# Os próprios guards entram na lista: um hook que caça vocabulário precisa
# escrever o vocabulário que caça, e sem esta linha ele se acusa a cada edição
# — inclusive no template de onde é gerado, cujo caminho não tem o ponto de
# `.claude/`. Ruído é o que faz um hook de aviso deixar de ser lido.
case "$rel" in
  *_test.*|test/*|tests/*|*/test/*|*/tests/*|CLAUDE.md|*/CLAUDE.md|.claude/*|*/.claude/*|.superpowers/*|*/.superpowers/*|hooks/guard-*|*/hooks/guard-*)
    exit 0
    ;;
esac

# `spec` exige número na sequência — "OpenAPI spec" e "spec-driven" passam.
hits="$(grep -nE 'spec [0-9]|CA-[0-9]|RF-[0-9]|critério de aceite' "$file" 2>/dev/null || true)"

if [[ -n "$hits" ]]; then
  # `|| true` é obrigatório: sob `set -e`, um jq que falhe aqui faria o hook
  # sair não-zero e bloquear — exatamente o que ele não tem direito de fazer.
  jq -n --arg rel "$rel" --arg hits "$hits" \
    '{systemMessage: ("guard-vocab: vocabulário de processo em artefato entregue (" + $rel + "):\n" + $hits + "\nReescreva como fato. Se o arquivo não é entregue, ele pertence ao cérebro.")}' || true
fi

exit 0

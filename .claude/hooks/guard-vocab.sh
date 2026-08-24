#!/usr/bin/env bash
# PreToolUse(Edit|Write|MultiEdit): avisa — nunca bloqueia — quando vocabulário
# de processo vaza para artefato entregue. Decisão de arquitetura entra como
# fato ("a navegação usa X"), nunca como "por causa da spec Y": quem clona o
# repo não tem como resolver a referência.
#
# Varre o texto que a chamada carrega — `content` no Write, `new_string` no
# Edit, `edits[].new_string` no MultiEdit — e nunca o arquivo no disco. Varrer
# o arquivo inteiro acusava o autor de uma edição por linha que ele não
# escreveu, a cada edição, e ruído é o que faz um hook de aviso deixar de ser
# lido. O preço, aceito: vocabulário já presente num arquivo fica fora do
# alcance deste hook.
#
# É PreToolUse pelo mesmo motivo que o guard-plans — o aviso chega antes da
# escrita, e aí ainda dá para redirecionar. Ler o payload é o que torna isso
# possível: antes da escrita não há arquivo para abrir.
#
# O aviso sai por `systemMessage` e o hook sai 0, como todos os outros guards
# — nenhum hook deste conjunto bloqueia.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

payload="$(cat || true)"
file="$(jq -r '.tool_input.file_path // empty' <<<"$payload" 2>/dev/null || true)"

if [[ -z "$file" ]]; then
  exit 0
fi

rel="${file#"$PWD"/}"

# Isentos: teste (onde CA-NN é convenção obrigatória), processo interno e
# .superpowers/ — workspace de sessão local, gitignorado, onde a nota de
# processo é o conteúdo legítimo, não um vazamento. A isenção é decidida pelo
# caminho; do payload vem só o texto varrido.
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

# Os três campos que carregam texto escrito, concatenados. `select(type ==
# "string")` descarta os ausentes; o `?` em `edits[]` evita erro quando a
# chamada não é MultiEdit.
added="$(jq -r '[.tool_input.content, .tool_input.new_string, (.tool_input.edits[]?.new_string)] | map(select(type == "string")) | join("\n")' <<<"$payload" 2>/dev/null || true)"

if [[ -z "$added" ]]; then
  exit 0
fi

# `spec` exige número na sequência — "OpenAPI spec" e "spec-driven" passam.
# Sem `-n`: a linha seria relativa ao trecho varrido e não ao arquivo, e número
# que não se pode abrir é pior que número nenhum.
hits="$(grep -E 'spec [0-9]|CA-[0-9]|RF-[0-9]|critério de aceite' <<<"$added" || true)"

if [[ -n "$hits" ]]; then
  # `|| true` é obrigatório: sob `set -e`, um jq que falhe aqui faria o hook
  # sair não-zero e bloquear — exatamente o que ele não tem direito de fazer.
  jq -n --arg rel "$rel" --arg hits "$hits" \
    '{systemMessage: ("guard-vocab: vocabulário de processo em artefato entregue (" + $rel + "):\n" + $hits + "\nReescreva como fato. Se o arquivo não é entregue, ele pertence ao cérebro.")}' || true
fi

exit 0

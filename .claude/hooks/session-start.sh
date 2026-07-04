#!/usr/bin/env bash
# SessionStart hook: injects the live handoff + the learning-journal head
# (never the full log) so continuity costs a bounded, small amount of
# context regardless of session number. See .claude/rules/learning-journal.md.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

if [[ -f .claude/handoff.md ]]; then
  echo "## Handoff"
  cat .claude/handoff.md
  echo
fi

if [[ -f .claude/.local/learning-journal.md ]]; then
  echo "## Diário de aprendizado (head)"
  awk '/^---$/{exit} {print}' .claude/.local/learning-journal.md
fi

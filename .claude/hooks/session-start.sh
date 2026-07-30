#!/usr/bin/env bash
# SessionStart hook: injects the live handoff so continuity costs a bounded,
# small amount of context regardless of session number.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

if [[ -f .claude/handoff.md ]]; then
  echo "## Handoff"
  cat .claude/handoff.md
  echo

  # I5: proxy metric for `ray stats` — counts handoffs actually injected,
  # not sessions started (no handoff.md yet doesn't count as activity).
  mkdir -p .claude/.ray-metrics
  count_file=.claude/.ray-metrics/handoffs.count
  count=$(cat "$count_file" 2>/dev/null || echo 0)
  echo $((count + 1)) > "$count_file"
fi

# I6a: injects the learning journal, written only by the assistant. No-ops
# outside --mode learn or before the assistant has written anything yet.
if [[ -f .claude/.local/learning-journal.md ]]; then
  echo "## Learning journal"
  cat .claude/.local/learning-journal.md
  echo
fi

# O progresso de marcos é escrito pelo ray (nunca pela IA) e injetado ao lado
# do diário. Só existe depois do primeiro `ray learn check` bem-sucedido.
if [[ -f .claude/.local/milestones-progress.md ]]; then
  echo "## Milestones"
  cat .claude/.local/milestones-progress.md
  echo
fi

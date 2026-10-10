#!/usr/bin/env bash
# Smoke test do binário: o roteiro que a varredura de 2026-10 seguiu à mão, para
# que o que ela pegou (init ai repetido sobrescrevendo edição, dry-run que deixa
# rastro, escrita por symlink) quebre o gate em vez de esperar a próxima
# varredura. Roda o `ray` de verdade num RAY_HOME e num HOME descartáveis.
#
# Não instala nada: a receita do teste não liga integração (nada vira
# dependência obrigatória) e `--no-global` pula os passos de máquina inteira.
# Não é um teste end-to-end da instalação, e não pretende ser.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# O binário é compilado antes de trocar o HOME: o Go guarda o cache de módulos
# nele, e um HOME novo faria baixar tudo de novo.
ray="$work/ray"
(cd "$root" && go build -o "$ray" .)

export HOME="$work/home" RAY_HOME="$work/ray-home"
mkdir -p "$HOME" "$RAY_HOME/profiles" "$RAY_HOME/components/s"

fail() { echo "smoke: FAIL — $*" >&2; exit 1; }
step() { echo "smoke: $*"; }

# hash do conteúdo de uma árvore (arquivos e nomes), para comparar antes/depois.
tree_sum() { (cd "$1" && find . -type f | LC_ALL=C sort | while read -r f; do cksum "$f"; done); }

cat > "$RAY_HOME/profiles/smoke.yaml" <<'YAML'
name: smoke
description: smoke test recipe, no integrations
components:
  - name: s
    dest: .claude/skills
scaffold:
  files:
    - path: CLAUDE.md
YAML
printf '# s\n' > "$RAY_HOME/components/s/SKILL.md"

init() { "$ray" init ai --profile smoke --no-global "$@"; }

step "init ai in an empty folder"
proj="$work/proj"
mkdir "$proj"
init "$proj" >/dev/null || fail "init ai exited non-zero on an empty folder"
for f in CLAUDE.md .claude/settings.json .claude/.ray-profile .claude/skills/s/SKILL.md .claude/hooks/session-start.sh; do
  [ -e "$proj/$f" ] || fail "init ai did not create $f"
done
[ "$(cat "$proj/.claude/.ray-profile")" = "smoke" ] || fail ".ray-profile does not record the recipe"

step "init ai twice changes nothing"
before="$(tree_sum "$proj")"
init "$proj" >/dev/null || fail "second init ai exited non-zero"
[ "$before" = "$(tree_sum "$proj")" ] || fail "a second init ai changed the tree"

step "an edited component survives init ai and update"
echo "edited by the user" >> "$proj/.claude/skills/s/SKILL.md"
init "$proj" >/dev/null || fail "init ai after an edit exited non-zero"
grep -q "edited by the user" "$proj/.claude/skills/s/SKILL.md" || fail "init ai overwrote an edited component"
"$ray" update --no-global "$proj" >/dev/null || fail "update exited non-zero"
grep -q "edited by the user" "$proj/.claude/skills/s/SKILL.md" || fail "update overwrote an edited component"

step "dry-run leaves no trace"
new="$work/does-not-exist-yet"
init --dry-run "$new" >/dev/null || fail "dry-run exited non-zero"
[ ! -e "$new" ] || fail "dry-run created the target folder"
empty="$work/empty"
mkdir "$empty"
init --dry-run "$empty" >/dev/null || fail "dry-run on an existing folder exited non-zero"
[ -z "$(ls -A "$empty")" ] || fail "dry-run wrote into an existing folder: $(ls -A "$empty")"

step "a symlink out of the project is refused"
victim="$work/victim"
echo "KEEP" > "$victim"
evil="$work/evil"
mkdir -p "$evil/.claude"
ln -s "$victim" "$evil/.claude/.ray-profile"
if init "$evil" >/dev/null 2>&1; then
  fail "init ai followed a symlink out of the project"
fi
[ "$(cat "$victim")" = "KEEP" ] || fail "init ai wrote through a symlink out of the project"

step "a CRLF checkout (the Windows default) leaves hooks and components intact"
crlf="$work/crlf"
mkdir "$crlf"
init "$crlf" >/dev/null || fail "init ai exited non-zero for the CRLF project"
git -C "$crlf" init -q
git -C "$crlf" add .claude .gitignore CLAUDE.md
git -C "$crlf" -c user.email=smoke@ray -c user.name=smoke commit -q -m vendor
git clone -q -c core.autocrlf=true "$crlf" "$work/crlf-clone"
if grep -lq $'\r' "$work"/crlf-clone/.claude/hooks/*.sh "$work/crlf-clone/.claude/skills/s/SKILL.md"; then
  fail "a core.autocrlf checkout put CRLF in the vendored hooks or components"
fi
if (cd "$work/crlf-clone" && "$ray" status) 2>&1 | grep -q "edited locally"; then
  fail "a core.autocrlf checkout made untouched components look edited"
fi

step "ok"

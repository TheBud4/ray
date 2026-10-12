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

step "a component symlink that leaves the component is refused"
mkdir -p "$RAY_HOME/components/leaky"
printf '# leaky\n' > "$RAY_HOME/components/leaky/SKILL.md"
ln -s "$victim" "$RAY_HOME/components/leaky/leak.txt"
cat > "$RAY_HOME/profiles/leaky.yaml" <<'YAML'
name: leaky
description: smoke recipe with a component that links outside itself
components:
  - name: leaky
    dest: .claude/skills
YAML
leakproj="$work/leakproj"
mkdir "$leakproj"
if "$ray" init ai --profile leaky --no-global "$leakproj" >/dev/null 2>&1; then
  fail "init ai copied a component whose symlink leaves the component"
fi
[ -z "$(ls -A "$leakproj")" ] || fail "init ai left traces after refusing a leaking component: $(ls -A "$leakproj")"

step "templates can be listed, edited, diffed and reset from the binary"
tpl="CLAUDE.md.tmpl"
tpl_state() { "$ray" template list | awk -v n="$tpl" '$1 == n { print $2 }'; }
[ "$(tpl_state)" = "current" ] || fail "template list: $tpl should be current after init ai, got '$(tpl_state)'"
editor="$work/editor.sh"
printf '#!/usr/bin/env bash\necho "# edited by the smoke test" >> "$1"\n' > "$editor"
chmod +x "$editor"
EDITOR="$editor" "$ray" template edit "$tpl" >/dev/null || fail "template edit exited non-zero"
[ "$(tpl_state)" = "edited" ] || fail "template list: $tpl should be edited after the edit, got '$(tpl_state)'"
"$ray" template diff "$tpl" | grep -q '^+# edited by the smoke test' || fail "template diff does not show the edit"
if "$ray" template reset "$tpl" >/dev/null 2>&1; then
  fail "template reset discarded an edit without --force"
fi
"$ray" template reset "$tpl" --force >/dev/null || fail "template reset --force exited non-zero"
[ "$(tpl_state)" = "current" ] || fail "template list: $tpl should be current after reset, got '$(tpl_state)'"
[ "$("$ray" profile show smoke | tail -n 1)" = "✓ ready" ] || fail "profile show: the smoke recipe should be ready"
"$ray" profile show leaky | tail -n 1 | grep -q "1 problem" || fail "profile show does not flag the leaking component"

step "serve answers only with the token, our Host and our Origin"
serve_out="$work/serve.out"
"$ray" serve --no-open --port 0 >"$serve_out" 2>"$work/serve.err" &
serve_pid=$!
trap 'kill "$serve_pid" 2>/dev/null || true; rm -rf "$work"' EXIT
url=""
tries=0
while [ "$tries" -lt 100 ]; do
  tries=$((tries + 1))
  url="$(grep -oE 'http://127\.0\.0\.1:[0-9]+/#token=[0-9a-f]{64}' "$serve_out" || true)"
  [ -n "$url" ] && break
  kill -0 "$serve_pid" 2>/dev/null || fail "serve exited early: $(cat "$work/serve.err")"
  sleep 0.1
done
[ -n "$url" ] || fail "serve did not print the URL with the token within 10 s"
base="${url%%/#*}"
tok="${url##*#token=}"
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
[ "$(code "$base/api/templates")" = "401" ] || fail "GET /api/templates without the token is not 401"
[ "$(code -H "X-Ray-Token: $tok" "$base/api/templates")" = "200" ] || fail "GET /api/templates with the token is not 200"
[ "$(code -H "X-Ray-Token: $tok" -H 'Host: evil.com' "$base/api/templates")" = "403" ] || fail "a foreign Host header is not 403"
hash="$(curl -s -H "X-Ray-Token: $tok" "$base/api/template?name=$tpl" | grep -oE '"hash":"[^"]*"' | sed 's/^"hash":"//; s/"$//')"
[ -n "$hash" ] || fail "GET /api/template did not return a hash"
body="{\"name\":\"$tpl\",\"content\":\"# smoke\\n\",\"baseHash\":\"$hash\"}"
[ "$(code -X PUT -H "X-Ray-Token: $tok" -H 'Content-Type: application/json' "$base/api/template" -d "$body")" = "403" ] \
  || fail "a PUT without Origin is not 403"
[ "$(code -X PUT -H "X-Ray-Token: $tok" -H 'Content-Type: application/json' -H "Origin: $base" "$base/api/template" -d "$body")" = "200" ] \
  || fail "a PUT with the token and our Origin is not 200"
curl -s -H "X-Ray-Token: $tok" "$base/api/template?name=$tpl" | grep -q '"content":"# smoke' || fail "the saved template does not read back"
kill -INT "$serve_pid"
wait "$serve_pid" || fail "serve did not exit cleanly on Ctrl-C"
trap 'rm -rf "$work"' EXIT

step "ok"

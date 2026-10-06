// Package claudecfg faz o merge idempotente de .claude/settings.json.
package claudecfg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// RayHookPrefix é o começo do comando de todo hook que o ray instala. É por
// ele que MergeSettings distingue um hook do ray (substituído a cada rodada)
// de um hook do usuário (preservado). Aponta para $CLAUDE_PROJECT_DIR, que o
// Claude Code exporta: um caminho relativo dependeria do diretório em que a
// sessão abriu, e numa subpasta o script não seria achado.
const RayHookPrefix = `bash "$CLAUDE_PROJECT_DIR"/.claude/hooks/`

// legacyRayHookPrefix é o comando em caminho relativo que versões anteriores
// escreviam. Continua valendo como "do ray" só para ser trocado pelo atual: o
// projeto já montado migra no próximo `ray init ai`, sem ficar com os dois.
const legacyRayHookPrefix = "bash .claude/hooks/"

// MergeSettings aplica settings em <target>/.claude/settings.json. Sem force,
// o que o usuário já tem vence: uma chave de topo só é gravada se o arquivo não
// a tem, e `hooks` é mesclado por evento — os hooks do ray (comando começando
// por RayHookPrefix) são trocados pelos atuais e os do usuário ficam, antes
// dos novos. Com force, cada chave de settings substitui a do arquivo, inclusive
// `hooks`. Em ambos os casos as chaves que o ray não gerencia são preservadas.
// dryRun imprime o resultado em out em vez de gravar.
func MergeSettings(target string, settings map[string]any, force, dryRun bool, out io.Writer) error {
	claudeDir := filepath.Join(target, ".claude")
	path := filepath.Join(claudeDir, "settings.json")

	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if doc == nil { // arquivo "null": mapa nil, nada a preservar
			doc = map[string]any{}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Ida e volta por JSON: os tipos de settings (int vindo do YAML, por
	// exemplo) passam a coincidir com os do arquivo lido, e a comparação de
	// entradas duplicadas deixa de falhar por tipo.
	norm, err := normalize(settings)
	if err != nil {
		return err
	}
	for k, v := range norm {
		switch {
		case force:
			doc[k] = v
		case k == "hooks":
			doc[k] = mergeHooks(doc[k], v)
		default:
			if _, ok := doc[k]; !ok {
				doc[k] = v
			}
		}
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if dryRun {
		_, err := out.Write(data)
		return err
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// normalize devolve m com os tipos que o encoding/json produziria ao ler o
// arquivo.
func normalize(m map[string]any) (map[string]any, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// mergeHooks une os hooks novos aos do arquivo, evento por evento: tira dos
// existentes os comandos do ray e acrescenta ao fim cada entrada nova que
// ainda não esteja lá. Se existing ou novo não forem o mapa esperado, vale o
// novo.
func mergeHooks(existing, novo any) any {
	cur, ok := existing.(map[string]any)
	nv, ok2 := novo.(map[string]any)
	if !ok || !ok2 {
		return novo
	}
	for event, v := range cur {
		list, isList := v.([]any)
		if !isList {
			continue
		}
		if kept := dropRayEntries(list); len(kept) > 0 {
			cur[event] = kept
		} else {
			delete(cur, event)
		}
	}
	for event, v := range nv {
		add, isList := v.([]any)
		if !isList {
			continue
		}
		list, _ := cur[event].([]any)
		for _, e := range add {
			if !containsEntry(list, e) {
				list = append(list, e)
			}
		}
		cur[event] = list
	}
	return cur
}

// dropRayEntries remove de cada entrada os hooks do ray e descarta a entrada
// que ficou sem hooks. Entrada sem lista `hooks` é do usuário e fica como está.
func dropRayEntries(entries []any) []any {
	kept := []any{}
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			kept = append(kept, e)
			continue
		}
		hooks, ok := entry["hooks"].([]any)
		if !ok {
			kept = append(kept, e)
			continue
		}
		left := []any{}
		for _, h := range hooks {
			if hm, ok := h.(map[string]any); ok {
				if cmd, _ := hm["command"].(string); strings.HasPrefix(cmd, RayHookPrefix) || strings.HasPrefix(cmd, legacyRayHookPrefix) {
					continue
				}
			}
			left = append(left, h)
		}
		if len(left) == 0 {
			continue
		}
		entry["hooks"] = left
		kept = append(kept, entry)
	}
	return kept
}

func containsEntry(list []any, e any) bool {
	for _, x := range list {
		if reflect.DeepEqual(x, e) {
			return true
		}
	}
	return false
}

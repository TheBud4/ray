// Package claudecfg faz o merge idempotente de .claude/settings.json.
package claudecfg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MergeSettings aplica settings em <target>/.claude/settings.json: cada chave
// de topo de settings (ex. model, effortLevel, hooks) substitui a do arquivo,
// preservando as chaves que o ray não gerencia. dryRun imprime o resultado em
// out em vez de gravar.
func MergeSettings(target string, settings map[string]any, dryRun bool, out io.Writer) error {
	claudeDir := filepath.Join(target, ".claude")
	path := filepath.Join(claudeDir, "settings.json")

	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	for k, v := range settings {
		doc[k] = v
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

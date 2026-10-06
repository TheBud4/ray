// Package fsutil reúne operações de disco que mais de um pacote precisa e que
// a biblioteca padrão não traz prontas. Só filesystem, sem rede.
package fsutil

import (
	"os"
	"path/filepath"
	"time"
)

// Ganchos substituíveis em teste: o rename de verdade, a classificação de erro
// transitório (específica de cada SO) e a espera entre tentativas.
var (
	renameFile          = os.Rename
	isTransientRename   = isTransientRenameError
	sleepBetweenRetries = time.Sleep
)

// Tentativas e espera inicial do rename no Windows. A espera dobra a cada
// tentativa (1ms, 2ms, ... 64ms), somando cerca de meio segundo no pior caso.
const (
	renameAttempts   = 10
	renameFirstDelay = time.Millisecond
)

// renameWithRetry renomeia from para to. No Windows o rename falha com "acesso
// negado" enquanto um leitor concorrente (outro `ray` lendo o mesmo estado, um
// antivírus) tem o destino aberto; essa janela é curta, então uma espera
// limitada resolve. Fora do Windows nenhum erro é transitório e há uma única
// tentativa, como antes.
func renameWithRetry(from, to string) error {
	delay := renameFirstDelay
	var err error
	for attempt := 1; attempt <= renameAttempts; attempt++ {
		if err = renameFile(from, to); err == nil || !isTransientRename(err) {
			return err
		}
		if attempt < renameAttempts {
			sleepBetweenRetries(delay)
			delay *= 2
		}
	}
	return err
}

// WriteFileAtomic grava data em path de modo que quem lê o arquivo vê o
// conteúdo antigo inteiro ou o novo inteiro, nunca o meio: escreve num
// temporário no mesmo diretório, sincroniza e só então o renomeia por cima do
// destino. O os.WriteFile trunca antes de escrever, e uma interrupção (ou um
// leitor concorrente) nesse intervalo enxerga o arquivo vazio — para um estado
// compartilhado entre projetos, como o das linhas-base, isso é perda silenciosa.
//
// O diretório pai tem de existir. Em caso de erro o temporário é removido e o
// destino fica como estava.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	// O CreateTemp cria com 0600; o modo pedido é o que o os.WriteFile daria
	// (antes do umask), então é aplicado explicitamente.
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return err
	}
	if err := renameWithRetry(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

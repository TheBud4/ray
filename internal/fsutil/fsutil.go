// Package fsutil reúne operações de disco que mais de um pacote precisa e que
// a biblioteca padrão não traz prontas. Só filesystem, sem rede.
package fsutil

import (
	"os"
	"path/filepath"
)

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
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

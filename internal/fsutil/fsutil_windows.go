//go:build windows

package fsutil

import (
	"errors"
	"syscall"
)

// errSharingViolation e errLockViolation não têm constante no pacote syscall.
const (
	errSharingViolation syscall.Errno = 32
	errLockViolation    syscall.Errno = 33
)

// isTransientRenameError diz se o rename falhou porque outro handle (um leitor
// concorrente, um antivírus, o indexador) segura o destino aberto sem
// FILE_SHARE_DELETE. No Windows isso se resolve sozinho em instantes.
func isTransientRenameError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case syscall.ERROR_ACCESS_DENIED, errSharingViolation, errLockViolation:
		return true
	}
	return false
}

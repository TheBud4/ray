//go:build !windows

package fsutil

// isTransientRenameError é sempre falso fora do Windows: o rename do POSIX
// substitui o destino mesmo com leitores abertos, então não há o que repetir.
func isTransientRenameError(error) bool { return false }

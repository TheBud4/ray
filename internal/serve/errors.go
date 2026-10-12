package serve

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"

	"github.com/TheBud4/ray/internal/catalog"
)

// apiError é o corpo de toda resposta de erro em /api/*: Erro é uma frase
// curta, Codigo o código que a página trata e Detalhe o texto do erro do Go.
type apiError struct {
	Erro    string `json:"erro"`
	Codigo  string `json:"codigo"`
	Detalhe string `json:"detalhe"`
}

// Códigos de erro da API. Cada um tem um status fixo.
const (
	codeUnknownTemplate = "unknown-template"
	codeReadOnly        = "read-only-on-web"
	codeEdited          = "edited"
	codeStale           = "stale"
	codeCustom          = "custom"
	codeNotUTF8         = "not-utf8"
	codeTooLarge        = "too-large"
	codeRefused         = "refused"
	codeInternal        = "internal"
	codeBadRequest      = "bad-request"
	codeBusy            = "busy"
	codeNotFound        = "not-found"
	codeUnauthorized    = "unauthorized"
	codeForbidden       = "forbidden"
	codeMethod          = "method-not-allowed"
	codePayloadTooLarge = "payload-too-large"
)

// errorKinds dá a cada código o status e a frase curta.
var errorKinds = map[string]struct {
	status int
	erro   string
}{
	codeUnknownTemplate: {http.StatusNotFound, "unknown template"},
	codeReadOnly:        {http.StatusForbidden, "hooks are read-only on the web"},
	codeEdited:          {http.StatusConflict, "the template has edits"},
	codeStale:           {http.StatusConflict, "the template changed on disk"},
	codeCustom:          {http.StatusUnprocessableEntity, "a custom template has no default"},
	codeNotUTF8:         {http.StatusUnprocessableEntity, "the content is not valid UTF-8"},
	codeTooLarge:        {http.StatusUnprocessableEntity, "the content is larger than 1 MiB"},
	codeRefused:         {http.StatusUnprocessableEntity, "refused"},
	codeInternal:        {http.StatusInternalServerError, "internal error"},
	codeBadRequest:      {http.StatusBadRequest, "bad request"},
	codeBusy:            {http.StatusServiceUnavailable, "a preview is already running"},
	codeNotFound:        {http.StatusNotFound, "not found"},
	codeUnauthorized:    {http.StatusUnauthorized, "unauthorized"},
	codeForbidden:       {http.StatusForbidden, "forbidden"},
	codeMethod:          {http.StatusMethodNotAllowed, "method not allowed"},
	codePayloadTooLarge: {http.StatusRequestEntityTooLarge, "payload too large"},
}

// writeCode responde o erro de código code com detalhe.
func writeCode(w http.ResponseWriter, code, detalhe string) {
	k := errorKinds[code]
	writeJSON(w, k.status, apiError{Erro: k.erro, Codigo: code, Detalhe: detalhe})
}

// writeError responde err com o código que codeFor escolhe.
func writeError(w http.ResponseWriter, err error) {
	writeCode(w, codeFor(err), err.Error())
}

// codeFor traduz um erro do catalog no código da API. As recusas tipadas têm
// código próprio; uma falha de E/S do disco é internal; o resto (symlink no
// caminho, linha-base ilegível) é uma recusa sem sentinela, refused.
func codeFor(err error) string {
	var edited *catalog.ErrEdited
	switch {
	case errors.Is(err, catalog.ErrUnknown):
		return codeUnknownTemplate
	case errors.Is(err, catalog.ErrReadOnly):
		return codeReadOnly
	case errors.As(err, &edited):
		return codeEdited
	case errors.Is(err, catalog.ErrStale):
		return codeStale
	case errors.Is(err, catalog.ErrCustom):
		return codeCustom
	case errors.Is(err, catalog.ErrNotUTF8):
		return codeNotUTF8
	case isIOError(err):
		return codeInternal
	}
	return codeRefused
}

// isIOError diz se err vem de uma operação do sistema operacional.
func isIOError(err error) bool {
	var pathErr *fs.PathError
	var linkErr *os.LinkError
	var sysErr *os.SyscallError
	return errors.As(err, &pathErr) || errors.As(err, &linkErr) || errors.As(err, &sysErr)
}

// writeJSON responde v como JSON com o status dado.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// refuse é a recusa do guard: JSON em /api/*, texto puro nas rotas estáticas.
func refuse(w http.ResponseWriter, isAPI bool, code, detalhe string) {
	if isAPI {
		writeCode(w, code, detalhe)
		return
	}
	status := errorKinds[code].status
	http.Error(w, http.StatusText(status), status)
}

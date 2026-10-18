package serve

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"

	"github.com/TheBud4/ray/internal/catalog"
)

// apiError é o corpo de toda resposta de erro em /api/*: Message é uma frase
// curta, Code o código que a página trata e Detail o texto do erro do Go. As
// chaves no JSON são erro, codigo e detalhe.
type apiError struct {
	Message string `json:"erro"`
	Code    string `json:"codigo"`
	Detail  string `json:"detalhe"`
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
	codeRenderTimeout   = "render-timeout"
)

// errorKinds dá a cada código o status e a frase curta.
var errorKinds = map[string]struct {
	status  int
	message string
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
	codeRenderTimeout:   {http.StatusUnprocessableEntity, "the template takes too long to render"},
}

// writeCode responde o erro de código code com detail.
func writeCode(w http.ResponseWriter, code, detail string) {
	k := errorKinds[code]
	writeJSON(w, k.status, apiError{Message: k.message, Code: code, Detail: detail})
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
	case errors.Is(err, errRenderTimeout):
		return codeRenderTimeout
	case isIOError(err):
		return codeInternal
	}
	return codeRefused
}

// writeWriteError responde o erro de uma escrita (Save ou Reset). Os dois
// podem falhar depois de gravar o arquivo, então uma falha interna não afirma
// que nada foi gravado.
func writeWriteError(w http.ResponseWriter, err error) {
	code := codeFor(err)
	detail := err.Error()
	if code == codeInternal {
		detail = "the file may already have been written; reload it before saving again: " + detail
	}
	writeCode(w, code, detail)
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
func refuse(w http.ResponseWriter, isAPI bool, code, detail string) {
	if isAPI {
		writeCode(w, code, detail)
		return
	}
	status := errorKinds[code].status
	http.Error(w, http.StatusText(status), status)
}

package serve

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TheBud4/ray/internal/catalog"
)

// maxContent é o teto, em bytes, do conteúdo decodificado de um Save ou de um
// Preview: o corpo cabe em 2 MiB, mas escapes JSON ainda podem inflar o texto.
const maxContent = 1 << 20

// defaultPreviewTimeout é o prazo de um preview quando Options não dá outro.
const defaultPreviewTimeout = 5 * time.Second

// writeMu serializa toda escrita no overlay e na linha-base (Save e Reset). A
// conferência de base defasada do Save só vale se ninguém gravar entre ela e a
// gravação: dois Saves concorrentes passariam os dois por ela. É do pacote, e
// não do handler, porque o overlay e o arquivo da linha-base são do processo.
var writeMu sync.Mutex

// apiHandler atende as rotas de templates. Os handlers são finos: o nome vai
// direto para o catalog, que o confina à lista (Lookup); nenhum caminho é
// montado aqui.
type apiHandler struct {
	templates catalog.Templates
	// renderGate, de capacidade 1, deixa um preview por vez; com ele ocupado,
	// o seguinte é recusado na hora, sem fila.
	renderGate     chan struct{}
	previewHook    func()
	previewTimeout time.Duration
}

func newAPIHandler(o Options) *apiHandler {
	timeout := o.previewTimeout
	if timeout <= 0 {
		timeout = defaultPreviewTimeout
	}
	return &apiHandler{
		templates:      o.Templates,
		renderGate:     make(chan struct{}, 1),
		previewHook:    o.previewHook,
		previewTimeout: timeout,
	}
}

// methods despacha pelo método; outro método é 405 em JSON.
type methods map[string]http.HandlerFunc

func (m methods) serve(w http.ResponseWriter, r *http.Request) {
	if h, ok := m[r.Method]; ok {
		h(w, r)
		return
	}
	allowed := make([]string, 0, len(m))
	for method := range m {
		allowed = append(allowed, method)
	}
	sort.Strings(allowed)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeCode(w, codeMethod, "allowed methods: "+strings.Join(allowed, ", "))
}

// decodeBody decodifica o corpo JSON em v: campo desconhecido, tipo errado ou
// um segundo valor depois do primeiro são 400. Devolve false quando já
// respondeu.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeBodyError(w, err)
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("the body has more than one JSON value")
		}
		writeBodyError(w, err)
		return false
	}
	return true
}

// tooLarge recusa um conteúdo acima de maxContent. Devolve true quando já
// respondeu.
func tooLarge(w http.ResponseWriter, content string) bool {
	if len(content) <= maxContent {
		return false
	}
	writeCode(w, codeTooLarge, "the decoded content has more than 1048576 bytes")
	return true
}

// listEntry é uma linha de GET /api/templates.
type listEntry struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	NoBaseline bool   `json:"noBaseline"`
	ReadOnly   bool   `json:"readOnly"`
}

// verified confere a linha-base, quando há quem a confira, e responde a
// recusa quando ela está ilegível: sem ela, nenhum estado decidido aqui é
// confiável. Devolve false quando já respondeu.
func (a *apiHandler) verified(w http.ResponseWriter) bool {
	if err := a.verify(); err != nil {
		writeError(w, err)
		return false
	}
	return true
}

// verify confere a linha-base; sem Verify configurado, não há o que conferir.
func (a *apiHandler) verify() error {
	if a.templates.Verify == nil {
		return nil
	}
	return a.templates.Verify()
}

// list responde a lista de templates. Só lê: o estado vem da simulação do
// `init ai`, sem gravar nada.
func (a *apiHandler) list(w http.ResponseWriter, _ *http.Request) {
	if !a.verified(w) {
		return
	}
	entries, err := a.templates.List()
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]listEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, listEntry{
			Name:       e.Name,
			State:      string(e.State),
			NoBaseline: e.NoBaseline,
			ReadOnly:   catalog.IsHook(e.Name),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// templateInfo é a resposta de GET /api/template.
type templateInfo struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Content  string `json:"content"`
	EOL      string `json:"eol"`
	UTF8     bool   `json:"utf8"`
	Hash     string `json:"hash"`
	ReadOnly bool   `json:"readOnly"`
}

func (a *apiHandler) info(w http.ResponseWriter, r *http.Request) {
	if !a.verified(w) {
		return
	}
	f, err := a.templates.Info(r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, templateInfo{
		Name:     f.Name,
		State:    string(f.State),
		Content:  f.Content,
		EOL:      f.EOL,
		UTF8:     f.UTF8,
		Hash:     f.Hash,
		ReadOnly: f.ReadOnly,
	})
}

// renderInfo é uma falha de render: posição (0 quando não há) e mensagem.
type renderInfo struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}

func toRenderInfo(e *catalog.RenderError) *renderInfo {
	if e == nil {
		return nil
	}
	return &renderInfo{Line: e.Line, Col: e.Col, Message: e.Message}
}

// saveRequest é o corpo de PUT /api/template.
type saveRequest struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	BaseHash string `json:"baseHash"`
}

// saveResponse é a resposta de um Save bem-sucedido. Um template gravado que
// não renderiza volta com Render preenchido, ainda com status 200.
type saveResponse struct {
	Outcome     string      `json:"outcome"`
	State       string      `json:"state"`
	Render      *renderInfo `json:"render"`
	Warnings    []string    `json:"warnings"`
	BaselineErr string      `json:"baselineErr"`
	Hash        string      `json:"hash"`
}

// errRenderTimeout recusa a gravação de um template que não renderiza dentro
// do prazo do preview.
var errRenderTimeout = errors.New("the template does not render within the preview deadline; fix it before saving")

// save grava o template, serializado com as outras escritas. O JSON só traz
// UTF-8 válido, e um U+FFFD no conteúdo é texto legítimo; a gravação por cima
// de um arquivo que não é UTF-8 o catalog recusa com ErrNotUTF8.
func (a *apiHandler) save(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if !decodeBody(w, r, &req) || tooLarge(w, req.Content) {
		return
	}
	res, err := a.saveSerialized(req)
	if err != nil {
		writeWriteError(w, err)
		return
	}
	out := saveResponse{
		Outcome:  string(res.Outcome),
		State:    string(res.State),
		Render:   toRenderInfo(res.Render),
		Warnings: res.Warnings,
		Hash:     res.Hash,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	if res.BaselineErr != nil {
		out.BaselineErr = res.BaselineErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// previewRequest é o corpo de POST /api/preview.
type previewRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Stack   bool   `json:"stack"`
}

// previewResponse é o render de um rascunho: Output, ou Err quando não
// renderiza (status 200 nos dois casos).
type previewResponse struct {
	Output string      `json:"output"`
	Err    *renderInfo `json:"err"`
}

// saveSerialized grava req sob writeMu. Um nome fora da lista e um hook são
// recusados antes de qualquer render. Depois, o conteúdo passa pelo render com
// prazo nos dois conjuntos de dados: o Save renderiza o que grava sem prazo, e
// um template que não termina seguraria o mutex de todas as escritas. Assim o
// mutex fica preso no máximo dois prazos de preview além do próprio Save. Esse
// render não usa o portão do preview: um preview em andamento não pode virar
// 503 numa gravação.
func (a *apiHandler) saveSerialized(req saveRequest) (catalog.SaveResult, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	e, err := a.templates.Lookup(req.Name)
	if err != nil {
		return catalog.SaveResult{}, err
	}
	if catalog.IsHook(e.Name) {
		return catalog.SaveResult{}, catalog.ErrReadOnly
	}
	for _, stack := range []bool{false, true} {
		_, timedOut, err := a.boundedPreview(e.Name, req.Content, stack)
		if err != nil {
			return catalog.SaveResult{}, err
		}
		if timedOut {
			return catalog.SaveResult{}, errRenderTimeout
		}
	}
	return a.templates.Save(e.Name, []byte(req.Content), req.BaseHash)
}

// previewOutcome é o que a goroutine do render devolve.
type previewOutcome struct {
	res catalog.PreviewResult
	err error
}

// boundedPreview roda o Preview numa goroutine e espera até previewTimeout.
// Vencido o prazo, devolve timedOut e abandona a goroutine: ela escreve num
// canal com buffer e termina sozinha, sem bloquear ninguém. Não há como
// interrompê-la — text/template não aceita cancelamento —, e o teto de saída
// do Preview não limita a CPU: um template que gira sem produzir saída (um
// range enorme de corpo vazio) pode rodar indefinidamente. O prazo só limita a
// taxa: no máximo uma goroutine abandonada nova por prazo vencido.
func (a *apiHandler) boundedPreview(name, content string, stack bool) (res catalog.PreviewResult, timedOut bool, err error) {
	done := make(chan previewOutcome, 1)
	go func() {
		if a.previewHook != nil {
			a.previewHook()
		}
		res, err := a.templates.Preview(name, []byte(content), stack)
		done <- previewOutcome{res: res, err: err}
	}()
	timer := time.NewTimer(a.previewTimeout)
	defer timer.Stop()
	select {
	case o := <-done:
		return o.res, false, o.err
	case <-timer.C:
		return catalog.PreviewResult{}, true, nil
	}
}

// preview renderiza um rascunho sem gravar nada. Só um render roda por vez; o
// seguinte, com o portão ocupado, é 503 na hora. O portão só é solto quando o
// render termina ou o prazo vence — nunca quando o cliente desiste, porque o
// render continuaria rodando e um cliente que aborta cada pedido teria vários
// ao mesmo tempo. Vencido o prazo, a resposta é um erro de render; o limite
// da goroutine abandonada está em boundedPreview.
func (a *apiHandler) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if !decodeBody(w, r, &req) || tooLarge(w, req.Content) {
		return
	}
	select {
	case a.renderGate <- struct{}{}:
	default:
		writeCode(w, codeBusy, "another preview is rendering; try again")
		return
	}
	defer func() { <-a.renderGate }()

	res, timedOut, err := a.boundedPreview(req.Name, req.Content, req.Stack)
	switch {
	case err != nil:
		writeError(w, err)
	case timedOut:
		writeJSON(w, http.StatusOK, previewResponse{Err: &renderInfo{Message: "preview timed out"}})
	default:
		writeJSON(w, http.StatusOK, previewResponse{Output: res.Output, Err: toRenderInfo(res.Err)})
	}
}

// diffResponse é a resposta de GET /api/diff.
type diffResponse struct {
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	Outdated bool   `json:"outdated"`
}

func (a *apiHandler) diff(w http.ResponseWriter, r *http.Request) {
	if !a.verified(w) {
		return
	}
	d, err := a.templates.Diff(r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, diffResponse{Kind: string(d.Kind), Text: d.Text, Outdated: d.Outdated})
}

// resetRequest é o corpo de POST /api/reset.
type resetRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

// resetResponse diz o que o Reset fez: "restored", "created" ou
// "already the default".
type resetResponse struct {
	Action      string `json:"action"`
	BaselineErr string `json:"baselineErr"`
}

// reset devolve o template ao padrão, serializado com as outras escritas. Um
// hook é recusado antes de qualquer escrita: pela web ele é só leitura.
func (a *apiHandler) reset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	action, baselineErr, err := a.resetSerialized(req.Name, req.Force)
	if err != nil {
		writeWriteError(w, err)
		return
	}
	out := resetResponse{Action: action}
	if baselineErr != nil {
		out.BaselineErr = baselineErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// resetSerialized é o Reset do catalog para quem não é hook, sob writeMu. A
// linha-base é conferida antes de tudo: ilegível, nada é escrito.
func (a *apiHandler) resetSerialized(name string, force bool) (action string, baselineErr, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := a.verify(); err != nil {
		return "", nil, err
	}
	e, err := a.templates.Lookup(name)
	if err != nil {
		return "", nil, err
	}
	if catalog.IsHook(e.Name) {
		return "", nil, catalog.ErrReadOnly
	}
	return a.templates.Reset(e.Name, force, catalog.CountChanged)
}

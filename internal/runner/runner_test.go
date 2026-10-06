package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFakeRecordCalls(t *testing.T) {
	f := &FakeRunner{}

	_, _ = f.Run(context.Background(), Command{
		Name: "npx",
		Args: []string{"x"}})

	if len(f.Calls) != 1 || f.Calls[0].Name != "npx" {
		t.Fatalf("Call Not Recorded: %+v", f.Calls)
	}
}

func TestExecRunnerEcho(t *testing.T) {

	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{Name: "echo", Args: []string{"hi"}})

	if err != nil {
		t.Fatal(err)
	}

	if res.ExitCode != 0 || res.Stdout != "hi\n" {
		t.Fatalf("Unexpected: %+v", res)
	}
}

func TestExecRunnerPropagatesEnv(t *testing.T) {
	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "echo $DO_NOT_TRACK"},
		Env:  map[string]string{"DO_NOT_TRACK": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "1\n" {
		t.Fatalf("Stdout = %q, want %q (Env not propagated to the subprocess)", res.Stdout, "1\n")
	}
}

func TestExecRunnerNilEnvInheritsProcessEnv(t *testing.T) {
	t.Setenv("RAY_TEST_INHERIT", "yes")
	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "echo $RAY_TEST_INHERIT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "yes\n" {
		t.Fatalf("Stdout = %q, want %q (nil Env should still inherit the process env)", res.Stdout, "yes\n")
	}
}

// FailureReason é o motivo curto que o resumo mostra: vazio no sucesso, o erro
// quando o processo nem rodou, e o exit com a última linha do stderr quando saiu
// com código diferente de zero.
func TestFailureReason(t *testing.T) {
	long := strings.Repeat("x", 300)
	cases := []struct {
		name string
		res  Result
		err  error
		want string
	}{
		{"success", Result{}, nil, ""},
		{"process could not run", Result{}, errors.New("exec: \"uv\": not found"), `exec: "uv": not found`},
		{"exit without stderr", Result{ExitCode: 2}, nil, "exit 2"},
		{"exit with stderr", Result{ExitCode: 22, Stderr: "curl: (6) Could not resolve host\n"}, nil, "exit 22: curl: (6) Could not resolve host"},
		{"last non-empty stderr line wins", Result{ExitCode: 1, Stderr: "warning: noise\nreal error\n\n"}, nil, "exit 1: real error"},
		{"long line is cut", Result{ExitCode: 1, Stderr: long}, nil, "exit 1: " + long[:200] + "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FailureReason(tc.res, tc.err); got != tc.want {
				t.Errorf("FailureReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

// lineSignal é um io.Writer que avisa quando recebe uma linha, para provar que
// a saída chega ENQUANTO o processo ainda roda.
type lineSignal struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	seen chan struct{}
	want string
	once sync.Once
}

func (w *lineSignal) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if strings.Contains(w.buf.String(), w.want) {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

func (w *lineSignal) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// O processo imprime "first" e fica esperando uma linha na entrada. Se a saída
// fosse acumulada até o fim, "first" nunca chegaria antes de a entrada ser
// dada, e o teste travaria (por isso o prazo). Prova streaming E stdin de uma vez.
func TestExecRunnerStreamsOutputAndForwardsInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell")
	}
	stdin, feed := io.Pipe()
	defer feed.Close() // se o teste falhar, o processo não fica esperando a entrada
	out := &lineSignal{seen: make(chan struct{}), want: "first"}
	var errOut bytes.Buffer

	done := make(chan error, 1)
	var res Result
	go func() {
		var err error
		res, err = ExecRunner{}.Run(context.Background(), Command{
			Name: "sh", Args: []string{"-c", "echo first; read x; echo got-$x; echo warn >&2"},
			Stdin: stdin, Stdout: out, Stderr: &errOut,
		})
		done <- err
	}()

	select {
	case <-out.seen:
	case <-time.After(3 * time.Second):
		t.Fatal(`"first" did not arrive while the process was still running: the output is buffered until the end`)
	}
	if _, err := io.WriteString(feed, "hello\n"); err != nil {
		t.Fatal(err)
	}
	feed.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not finish after receiving its input")
	}
	if got := out.String(); got != "first\ngot-hello\n" {
		t.Errorf("stdout = %q, want %q", got, "first\ngot-hello\n")
	}
	if got := errOut.String(); got != "warn\n" {
		t.Errorf("stderr = %q, want it kept apart from stdout: %q", got, "warn\n")
	}
	if res.Stdout != "" || res.Stderr != "" {
		t.Errorf("Result = %+v, want empty Stdout/Stderr when the streams were provided", res)
	}
}

// Sem fluxos, nada muda: a saída continua acumulada em Result.
func TestExecRunnerStillBuffersWhenNoStreamsAreGiven(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell")
	}
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", "echo out; echo err >&2"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "out\n" || res.Stderr != "err\n" {
		t.Errorf("Result = %+v, want the buffered output", res)
	}
}

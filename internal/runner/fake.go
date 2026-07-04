package runner

import "context"

// FakeRunner registers calls and returns programmed responses.
// Receiver: Pointer
// because each run mutates calls
type FakeRunner struct {
	Calls   []Command
	Results map[string]Result
	Err     error
}

func (f *FakeRunner) Run(_ context.Context, c Command) (Result, error) {

	f.Calls = append(f.Calls, c)

	if f.Err != nil {
		return Result{}, f.Err
	}

	if r, ok := f.Results[c.String()]; ok {
		return r, nil
	}
	return Result{ExitCode: 0}, nil
}

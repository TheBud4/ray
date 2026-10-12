package catalog

import "testing"

func TestClassifyTellsWhatAnEditDid(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "# mine\n")
	before := []byte("# mine\n")

	if got, _ := f.cat.Classify(sample, before, before); got != OutcomeNoChanges {
		t.Errorf("same bytes = %q, want %q", got, OutcomeNoChanges)
	}
	f.overlay(sample, "# edited\n")
	if got, _ := f.cat.Classify(sample, before, []byte("# edited\n")); got != Outcome(Edited) {
		t.Errorf("changed = %q, want edited", got)
	}
	emb := f.embedded(sample)
	f.overlay(sample, string(emb))
	f.baseline(sample, emb)
	if got, _ := f.cat.Classify(sample, before, emb); got != OutcomeCurrent {
		t.Errorf("back to default = %q, want current", got)
	}
}

func TestClassifyKeepsCustomAsCustom(t *testing.T) {
	f := newFixture(t)
	f.overlay("mine.tmpl", "b\n")
	if got, _ := f.cat.Classify("mine.tmpl", []byte("a\n"), []byte("b\n")); got != Outcome(Custom) {
		t.Errorf("custom edit = %q, want custom (not edited)", got)
	}
}

func TestClassifyRejectsAnUnknownName(t *testing.T) {
	f := newFixture(t)
	if _, err := f.cat.Classify("nope.tmpl", []byte("a"), []byte("b")); err == nil {
		t.Error("Classify of an unknown name succeeded; want an error")
	}
}

func TestCountChangedCountsPlusAndMinusLines(t *testing.T) {
	if got := CountChanged([]byte("a\nb\n"), []byte("a\nc\n")); got != 2 {
		t.Errorf("CountChanged = %d, want 2 (one removed, one added)", got)
	}
	if got := CountChanged([]byte("a\n"), []byte("a\n")); got != 0 {
		t.Errorf("CountChanged of equal texts = %d, want 0", got)
	}
}

package report

import "testing"

func TestTallyCountsEntriesByOutcome(t *testing.T) {
	entries := []Entry{
		{Kind: Component, Name: "a", Outcome: Installed},
		{Kind: Component, Name: "b", Outcome: Installed},
		{Kind: Global, Name: "headroom", Outcome: Failed, Detail: "exit 1"},
		{Kind: File, Name: "CLAUDE.md", Outcome: Created},
		{Kind: File, Name: ".gitignore", Outcome: Kept, Detail: "already there"},
		{Kind: Command, Name: "uv tool upgrade x", Outcome: Updated, Detail: "upgraded"},
		{Kind: Component, Name: "c", Outcome: Unchanged},
	}
	got := Tally(entries)
	want := map[Outcome]int{Installed: 2, Failed: 1, Created: 1, Kept: 1, Updated: 1, Unchanged: 1}
	for o, n := range want {
		if got[o] != n {
			t.Errorf("Tally()[%s] = %d, want %d", o, got[o], n)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Tally() = %v, want only the outcomes present", got)
	}
}

func TestTallyOfNothingIsEmpty(t *testing.T) {
	if got := Tally(nil); len(got) != 0 {
		t.Errorf("Tally(nil) = %v, want empty", got)
	}
}

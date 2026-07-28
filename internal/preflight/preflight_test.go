package preflight

import "testing"

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

func TestRunSetsFoundFromLooker(t *testing.T) {
	l := stubLooker{"npx": true, "node": false, "python3.10+": true, "uv": true, "headroom": false, "graphify": false}
	checks := Run(l, true)

	got := map[string]bool{}
	for _, c := range checks {
		got[c.Name] = c.Found
	}
	for name, want := range l {
		if got[name] != want {
			t.Errorf("check %q Found = %v, want %v", name, got[name], want)
		}
	}
}

func TestRunRequiredWithNeedPython(t *testing.T) {
	l := stubLooker{} // nothing found
	checks := Run(l, true)

	required := map[string]bool{}
	for _, c := range checks {
		required[c.Name] = c.Required
	}
	want := map[string]bool{
		"npx": true, "node": false, "python3.10+": true, "uv": true,
		"headroom": false, "graphify": false,
	}
	for name, w := range want {
		if required[name] != w {
			t.Errorf("check %q Required = %v, want %v (needPython=true)", name, required[name], w)
		}
	}
}

func TestRunRequiredWithoutNeedPython(t *testing.T) {
	l := stubLooker{}
	checks := Run(l, false)

	required := map[string]bool{}
	for _, c := range checks {
		required[c.Name] = c.Required
	}
	if required["python3.10+"] {
		t.Error("python3.10+ Required = true, want false when needPython=false")
	}
	if required["uv"] {
		t.Error("uv Required = true, want false when needPython=false")
	}
	if !required["npx"] {
		t.Error("npx Required = false, want true regardless of needPython")
	}
}

func TestRunFixCommandsPresentWhereExpected(t *testing.T) {
	checks := Run(stubLooker{}, true)

	wantFix := map[string]bool{
		"npx": false, "node": false, "python3.10+": false,
		"uv": true, "headroom": true, "graphify": true,
	}
	for _, c := range checks {
		want, ok := wantFix[c.Name]
		if !ok {
			t.Fatalf("unexpected check %q", c.Name)
		}
		hasFix := len(c.Fix) > 0
		if hasFix != want {
			t.Errorf("check %q has Fix = %v, want %v", c.Name, hasFix, want)
		}
	}
}

func TestMissingRequiredFiltersByRequiredAndFound(t *testing.T) {
	l := stubLooker{"npx": true, "python3.10+": false, "uv": true, "headroom": false, "graphify": false}
	checks := Run(l, true)

	missing := MissingRequired(checks)
	names := map[string]bool{}
	for _, c := range missing {
		names[c.Name] = true
	}
	if !names["python3.10+"] {
		t.Error("MissingRequired should include python3.10+ (required, missing)")
	}
	if names["headroom"] || names["graphify"] {
		t.Error("MissingRequired should not include non-required checks even if missing")
	}
	if names["npx"] {
		t.Error("MissingRequired should not include npx (present)")
	}
}

func TestRunIncludesOptionalJQ(t *testing.T) {
	l := stubLooker{"npx": true, "jq": false}
	checks := Run(l, false)

	var jq *Check
	for i := range checks {
		if checks[i].Name == "jq" {
			jq = &checks[i]
		}
	}
	if jq == nil {
		t.Fatal("Run() não inclui check para jq; os hooks scaffoldados dependem dele")
	}
	if jq.Required {
		t.Error("jq Required = true, want false — hook sem jq faz no-op, não quebra")
	}
	if jq.Hint == "" {
		t.Error("jq Hint vazio; sem hint o ray doctor não diz o que fazer")
	}
}

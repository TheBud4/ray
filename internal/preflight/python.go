package preflight

// pythonCandidates lista, em ordem de tentativa, os nomes de binário que
// satisfazem "python3.10+" no GOOS dado. No Windows o instalador oficial
// normalmente expõe "python" (não "python3") — daí o fallback; nos demais
// SOs o binário continua sendo só "python3".
func pythonCandidates(goos string) []string {
	if goos == "windows" {
		return []string{"python3", "python"}
	}
	return []string{"python3"}
}

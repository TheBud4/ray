package preflight

import "os/exec"

// PathLooker responde a mesma pergunta do RunnerLooker — "esta dependência
// está disponível?" — sem executar nada: exec.LookPath resolve o nome contra o
// $PATH e checa o bit de execução, e para isso não cria processo. É o que
// permite ao `ray status` afirmar que só lê enquanto ainda avisa sobre um
// servidor do .mcp.json cujo Command sumiu.
//
// Não fura a fronteira do §3 (o internal/runner é a única saída para processos
// externos) porque não há processo: LookPath é stat sobre diretórios do PATH.
//
// O preço é não ver versão. Onde a versão importa — a tabela do `ray doctor` e
// o gate do `ray init ai`, que exigem python3.10+ — o RunnerLooker continua
// sendo o certo.
type PathLooker struct{}

// Look reporta se name está no PATH. "python3.10+" resolve para python3:
// presença é tudo que este Looker consegue afirmar, e responder false por não
// achar um executável chamado "python3.10+" seria pior que responder pela
// presença.
func (PathLooker) Look(name string) bool {
	bin := name
	if name == "python3.10+" {
		bin = "python3"
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

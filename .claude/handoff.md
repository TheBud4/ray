Nenhuma sessão de implementação começou ainda. Estado atual: só a
documentação de design/plano existe neste repositório; o código Go do `ray`
ainda não foi escrito aqui. Usuário pausou de propósito antes do primeiro
comando — retomar do zero, sem pressa.

**Próximo passo, exato, quando o usuário voltar:**

1. Rodar no terminal, dentro de `/home/thebud4/www/Projetos/ray`:
   ```
   go mod init github.com/murilopmr/ray
   ```
   (module path já confirmado com o usuário como o mesmo documentado em
   `ray-build-guide.md` §Stack — Go 1.25 · Cobra · `gopkg.in/yaml.v3`.)
2. Depois de rodar, olhar o arquivo `go.mod` gerado e responder, em voz alta
   pra mentora: (a) o que tem dentro dele, (b) por que o Go precisa desse
   arquivo.
3. Isso é o degrau 2 da escada de dicas (ponteiro conceitual) — o usuário
   ainda não tinha usado `go.mod` nem uma lib de CLI (Cobra) antes; só tinha
   Go instalado. Calibrar explicações partindo do zero nesses dois pontos.
4. Marco seguinte (ainda não iniciado): esqueleto do comando raiz com Cobra
   compilando + `ray --help` rodando (ver `ray-build-guide.md`).

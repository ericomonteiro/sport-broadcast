# Transmissão — Placar de Beach Tênis

Placar ao vivo para transmissões de partidas de beach tênis. Um operador atualiza o placar em um painel no navegador, e o overlay aparece na transmissão do OBS, atualizando em tempo real.

## Requisitos

- [Go](https://go.dev/dl/) 1.24 ou superior
- OBS Studio (para usar o overlay na transmissão)

Não há dependências externas: o projeto usa apenas a biblioteca padrão do Go.

## Como rodar

```bash
go run ./cmd/broadcast -addr :8080 -best-of 3
```

Opções:

| Flag | Padrão | Descrição |
|---|---|---|
| `-addr` | `:8080` | Endereço HTTP do servidor |
| `-best-of` | `3` | Quantidade de sets da partida (`1`, `3` ou `5`) |

Para gerar um binário:

```bash
go build -o broadcast ./cmd/broadcast
./broadcast -best-of 3
```

## Configurando o OBS

1. Adicione uma fonte **Navegador** (*Browser*) na cena.
2. Use a URL `http://localhost:8080/scoreboard/overlay`.
3. Use a resolução de 1920 × 1080. O fundo da página é transparente, então o placar aparece por cima da imagem.

## Usando o painel de controle

Abra `http://localhost:8080/control` em outra aba ou em outro dispositivo na mesma rede.

- **+ Ponto:** registra um ponto para o time.
- **Trocar saque:** passa o saque para o outro time. A troca é sempre manual.
- **Desfazer último ponto:** volta um ponto. Os nomes dos atletas não são afetados.
- **Novo jogo:** zera o placar e mantém os nomes.
- **Atletas:** digite os nomes de cada time separados por vírgula e clique em *Salvar nomes*.

Várias abas do painel podem ficar abertas ao mesmo tempo. Quando uma muda o placar, as outras são atualizadas.

## Regras de pontuação

- **Game:** pontos 0, 15, 30, 40 e vantagem. Vence quem chegar a 4 pontos com 2 de diferença.
- **Set:** vence quem chegar a 6 games com 2 de diferença. Em 6–6, joga-se tie-break até 7 pontos (também com 2 de diferença).
- **Partida:** melhor de 1, 3 ou 5 sets.

Se o campeonato usar outro formato, ajuste o pacote `internal/scoreboard`.

## Estrutura do projeto

```
cmd/broadcast/       ponto de entrada do servidor
internal/scoreboard/ regras de pontuação, sem I/O (com testes)
internal/server/     API HTTP e stream de atualizações (SSE)
web/                 páginas do overlay e do painel, embutidas no binário
```

## Testes

```bash
go test ./...
```

## Rotas

| Método | Rota | Descrição |
|---|---|---|
| GET | `/scoreboard/overlay` | Overlay para o OBS |
| GET | `/control` | Painel do operador |
| GET | `/state` | Estado atual da partida (JSON) |
| GET | `/events` | Stream SSE com as atualizações |
| POST | `/api/point` | Registra ponto. Corpo: `{"team": 0}` ou `{"team": 1}` |
| POST | `/api/serve` | Troca o saque |
| POST | `/api/undo` | Desfaz o último ponto |
| POST | `/api/teams` | Altera os nomes. Corpo: `{"teams": [{"players": ["..."]}, {"players": ["..."]}]}` |
| POST | `/api/reset` | Zera o placar. Corpo: `{"bestOf": 3}` |

As respostas de sucesso são o estado da partida em JSON. Erros de JSON inválido retornam `400`; violações das regras (por exemplo, ponto com a partida encerrada) retornam `422`.

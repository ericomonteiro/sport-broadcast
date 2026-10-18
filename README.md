# Transmissão — Placar de Beach Tênis

Placar ao vivo para transmissões de partidas de beach tênis. Cada jogo tem o seu placar salvo no Postgres. Um operador atualiza o placar em um painel no navegador, e o overlay do jogo aparece na transmissão do OBS, atualizando em tempo real.

## Requisitos

- [Go](https://go.dev/dl/) 1.25 ou superior
- [Docker](https://www.docker.com/) (para o Postgres local)
- OBS Studio (para usar o overlay na transmissão)

## Como rodar

1. Suba o banco:

   ```bash
   docker compose up -d
   ```

2. Copie a configuração local (só na primeira vez) e ajuste se necessário:

   ```bash
   cp application-local.properties.example application-local.properties
   ```

3. Rode o servidor:

   ```bash
   ENVIRONMENT=local go run ./cmd/broadcast
   ```

As tabelas são criadas automaticamente na primeira execução.

Para gerar um binário:

```bash
go build -o broadcast ./cmd/broadcast
ENVIRONMENT=local ./broadcast
```

## Configuração

As configurações vêm de um arquivo `application-<ENVIRONMENT>.properties` e de variáveis de ambiente. Com `ENVIRONMENT=local`, o arquivo lido é `application-local.properties`. Variáveis de ambiente têm prioridade sobre o arquivo, com o nome em maiúsculas e `_` no lugar de `.` (por exemplo, `DB_HOST`).

| Chave | Padrão | Descrição |
|---|---|---|
| `server.addr` | `:8080` | Endereço HTTP do servidor |
| `db.host` | — | Host do Postgres (obrigatório) |
| `db.port` | — | Porta do Postgres (obrigatório) |
| `db.user` | — | Usuário do Postgres (obrigatório) |
| `db.password` | — | Senha do Postgres |
| `db.name` | — | Nome do banco (obrigatório) |
| `db.sslmode` | `disable` | Modo SSL do Postgres |

Se `ENVIRONMENT` não estiver definida, só as variáveis de ambiente são lidas.

## Fluxo pelo navegador

Abra `http://localhost:8080/` para:

1. **Criar partida:** preencha o nome, o melhor de e os jogadores de cada time. Ao criar, a partida já fica selecionada.
2. **Ver as rotas:** a partida selecionada mostra a URL do overlay (com botão para copiar), a URL do painel e uma prévia do overlay.
3. **Selecionar uma partida em aberto:** a lista mostra só as partidas que ainda não foram finalizadas. A seleção fica na URL (`#<id>`), então recarregar a página mantém a partida escolhida.
4. **Finalizar partida:** pelo início da página ou pelo painel de controle. Depois de finalizada, a partida sai da lista de abertas e o placar não aceita mais alterações. O overlay continua acessível.

## Criando um jogo pela API

Cada jogo recebe um ID. Ele aparece nas URLs do overlay e do painel.

```bash
curl -X POST http://localhost:8080/api/games \
  -H 'Content-Type: application/json' \
  -d '{"name":"Final masculina","bestOf":3,"teams":[{"players":["Erico","João"]},{"players":["Pedro","Lucas"]}]}'
```

A resposta traz o `id` do jogo. Use `GET /api/games` para listar os jogos (ou `GET /api/games?status=open` para só os abertos).

## Configurando o OBS

1. Adicione uma fonte **Navegador** (*Browser*) na cena.
2. Use a URL `http://localhost:8080/scoreboard/<id do jogo>/overlay`.
3. Use a resolução de 1920 × 1080. O fundo da página é transparente, então o placar aparece por cima da imagem.

## Usando o painel de controle

Abra `http://localhost:8080/scoreboard/<id do jogo>/control` em outra aba ou em outro dispositivo na mesma rede.

- **+ Ponto:** registra um ponto para o time.
- **+ Game / − Game:** adiciona ou remove um game do time. Adicionar um game segue as mesmas regras de um game vencido na quadra, então pode fechar o set.
- **+ Set / − Set:** dá o set ao time, com placar final (6-x, 7-5 ou 7-6), ou remove o último set vencido por ele. Use para corrigir o placar.
- **Trocar saque:** passa o saque para o outro time. A troca é sempre manual.
- **Finalizar partida:** fecha a partida. O painel fica bloqueado, mas o overlay continua acessível.
- **Desfazer último ponto:** volta a última ação. Os nomes dos atletas não são afetados.
- **Novo jogo:** zera o placar e mantém os nomes.
- **Atletas:** digite os nomes de cada time separados por vírgula e clique em *Salvar nomes*.

Várias abas do painel podem ficar abertas ao mesmo tempo. Quando uma muda o placar, as outras são atualizadas.

O placar é salvo a cada ação. Ao reiniciar o servidor, o placar do jogo volta como estava. O histórico de desfazer não é salvo, então depois de reiniciar só é possível desfazer ações feitas desde a inicialização.

## Regras de pontuação

- **Game:** pontos 0, 15, 30, 40 e vantagem. Vence quem chegar a 4 pontos com 2 de diferença.
- **Set:** vence quem chegar a 6 games com 2 de diferença. Em 6–6, joga-se tie-break até 7 pontos (também com 2 de diferença).
- **Partida:** melhor de 1, 3 ou 5 sets.

Se o campeonato usar outro formato, ajuste o pacote `internal/scoreboard`.

## Estrutura do projeto

```
cmd/broadcast/          ponto de entrada do servidor
internal/config/        configurações (arquivo .properties e variáveis de ambiente)
internal/infra/database/ conexão com o Postgres e criação das tabelas
internal/model/         entidades: Game e Scoreboard
internal/repository/    consultas ao banco
internal/scoreboard/    regras de pontuação, sem I/O (com testes)
internal/service/       regras: partidas, placares ao vivo e assinantes
internal/handler/       camada HTTP: rotas, páginas, JSON e stream de atualizações (SSE)
resource/               ícones usados no overlay (ex.: bolinha de tênis), embutidos no binário
web/                    páginas do overlay e do painel, embutidas no binário
docker-compose.yml      Postgres para desenvolvimento local
```

## Testes

```bash
go test ./...
```

Os testes não acessam o banco: a camada de serviço usa um armazenamento em memória no lugar do Postgres. Por isso não é preciso subir o Docker para rodar os testes.

## Rotas

| Método | Rota | Descrição |
|---|---|---|
| GET | `/` | Página de partidas (criar, selecionar e finalizar) |
| GET | `/scoreboard/{id}/overlay` | Overlay do jogo para o OBS |
| GET | `/scoreboard/{id}/control` | Painel do operador do jogo |
| GET | `/api/games` | Lista os jogos. Com `?status=open`, só os abertos |
| POST | `/api/games` | Cria um jogo. Corpo: `{"name", "bestOf", "teams"}` |
| GET | `/api/games/{id}` | Dados de um jogo, incluindo `finishedAt` |
| POST | `/api/games/{id}/finish` | Finaliza o jogo. Repetir a chamada não altera a data |
| GET | `/api/scoreboard/{id}/state` | Estado atual do placar (JSON) |
| GET | `/api/scoreboard/{id}/events` | Stream SSE com as atualizações do jogo |
| POST | `/api/scoreboard/{id}/point` | Registra ponto. Corpo: `{"team": 0}` ou `{"team": 1}` |
| POST | `/api/scoreboard/{id}/games/add` | Adiciona um game ao time. Corpo: `{"team": 0}` ou `{"team": 1}` |
| POST | `/api/scoreboard/{id}/games/remove` | Remove um game do time (correção) |
| POST | `/api/scoreboard/{id}/sets/add` | Dá o set ao time, com placar final (6-x, 7-5 ou 7-6) |
| POST | `/api/scoreboard/{id}/sets/remove` | Remove o último set vencido pelo time (correção) |
| POST | `/api/scoreboard/{id}/serve` | Troca o saque |
| POST | `/api/scoreboard/{id}/undo` | Desfaz a última ação |
| POST | `/api/scoreboard/{id}/teams` | Altera os nomes. Corpo: `{"teams": [{"players": ["..."]}, {"players": ["..."]}]}` |
| POST | `/api/scoreboard/{id}/reset` | Zera o placar. Corpo: `{"bestOf": 3}` |

Erros: `400` para ID ou JSON inválido, `404` para jogo inexistente, `422` para violação das regras, dados inválidos ou partida finalizada, e `500` para erros de banco (registrados no log, sem detalhes na resposta).

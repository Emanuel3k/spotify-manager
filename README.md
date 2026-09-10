# spotify-manager

CLI pessoal para gerenciar sua conta Spotify, construída em Go com arquitetura hexagonal (ports & adapters). Cresce por features — cada uma nova reaproveita a mesma autenticação e a mesma estrutura de portas/adapters.

## Requisitos

- Go 1.26+
- Uma conta Spotify Premium e um app criado em [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard) (Development Mode). Em Development Mode a Spotify limita o app a até 5 usuários autorizados — adicione sua própria conta em **Settings → User Management** do app, mesmo sendo você o dono.

## Setup

1. Copie `.env.example` para `.env` e preencha com as credenciais do seu app:

   ```
   SPOTIFY_CLIENT_ID=...
   SPOTIFY_CLIENT_SECRET=...
   SPOTIFY_REDIRECT_URI=http://127.0.0.1:8080/callback
   ```

   O redirect URI precisa estar cadastrado *exatamente igual* nas configurações do app no dashboard da Spotify. A Spotify exige um IP de loopback explícito (`127.0.0.1`), não aceita `localhost`.

2. `.env` é ignorado pelo git — as credenciais nunca saem da sua máquina.

## Uso

### Modo interativo

Rodar sem nenhum argumento abre um menu navegável com as setas — não precisa lembrar comando nenhum:

```powershell
go run ./cmd
```

### Modo direto (scriptável)

Todo comando também existe como subcomando, pra automação ou uso rápido:

```powershell
# Login (abre o navegador para autorizar; guarda o token localmente)
go run ./cmd auth login

# Ver quem está logado / status da sessão
go run ./cmd auth whoami
go run ./cmd auth status

# Logout
go run ./cmd auth logout

# Dividir uma playlist em playlists por ano de lançamento (privadas, upsert)
go run ./cmd playlist split-by-year "https://open.spotify.com/playlist/<id>"
```

Ou compile um binário: `go build -o spotify-manager.exe ./cmd` e chame `.\spotify-manager.exe` (menu) ou `.\spotify-manager.exe <comando>` (direto).

### Logs

Por padrão só aparecem logs de nível INFO no stderr. Para mais detalhe (toda chamada HTTP feita à Spotify, com status e duração):

```powershell
$env:LOG_LEVEL="debug"
go run ./cmd playlist split-by-year "..."
```

## Features

### Autenticação (`auth`)

Fluxo OAuth 2.0 Authorization Code completo: abre o navegador, sobe um servidor local para capturar o `redirect_uri`, troca o código por token, guarda o token em `~/.config/spotify-manager/credentials.json` (ou equivalente no Windows), e renova automaticamente quando expira.

### Split por ano (`playlist split-by-year`)

Recebe o link de uma playlist, agrupa as faixas pelo ano de lançamento e cria (ou reaproveita) uma playlist privada por ano encontrado, adicionando as faixas correspondentes.

- **Upsert de playlist**: se já existe uma playlist sua com o nome do ano (ex. `"2020"`), ela é reaproveitada em vez de criar outra.
- **Upsert de faixas**: faixas que já estão na playlist do ano não são adicionadas de novo — rodar o comando várias vezes na mesma playlist de origem é seguro (idempotente).
- Faixas sem ano de lançamento conhecido são ignoradas (fica um aviso no log).
- A Spotify só retorna o conteúdo de playlists que você é dono ou colabora — um link de playlist de terceiros/editorial roda sem erro, mas com zero faixas.

## Limitações conhecidas da API da Spotify

A Spotify mudou a Web API para apps em Development Mode em fevereiro de 2026 (ver [migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide)). Duas coisas relevantes aqui:

- Só é possível ler o conteúdo de playlists que você é dono ou colabora.
- **Bug conhecido da própria Spotify**: `POST /me/playlists` às vezes ignora o campo `public: false` e cria a playlist como pública mesmo assim. O código tenta corrigir com uma chamada de PUT logo em seguida, mas isso nem sempre funciona do lado da Spotify — se notar uma playlist de ano marcada como pública, ajuste manualmente pelo app do Spotify.

## Desenvolvimento

```powershell
go build ./...
go vet ./...
gofmt -l .          # deve ficar vazio
go test ./...        # roda tudo
go test ./... -v -run TestNomeDoTeste   # um teste específico
```

Veja [CLAUDE.md](CLAUDE.md) para a arquitetura e convenções do projeto.

## Roadmap

Sem próximas features definidas ainda — evoluir conforme surgirem ideias.

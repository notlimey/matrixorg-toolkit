# callbot

A Matrix bot that monitors Element Call sessions and displays a live status widget in your room — showing who is in the call, how long it has been running, and participant avatars with initials.

The widget is a self-hosted web page pinned in the room's sidebar. It reads call state directly from the Matrix room via the [Matrix Widget API](https://github.com/matrix-org/matrix-widget-api), so it updates in real time without the bot sending any messages.

---

## Features

- Detects Element Call sessions using `org.matrix.msc3401.call.member` state events
- Pins a live dark-theme status widget in the room sidebar
- Shows participant initials avatars (up to 5 + overflow), elapsed time, room name
- Updates automatically every minute and on every join/leave
- Bot commands to configure everything at runtime without restarting
- E2EE support via [mautrix-go](https://github.com/mautrix/go)
- Config persisted to `config.json` — survives restarts

---

## Requirements

- A Matrix account for the bot (`@callbot:matrix.org` or similar)
- [libolm](https://gitlab.matrix.org/matrix-org/olm) for E2EE
- Docker + Docker Compose (recommended), or Go 1.21+ to build locally
- A public HTTPS URL to serve the widget from (see [Widget hosting](#widget-hosting))

---

## Setup

### 1. Clone the repo

```sh
git clone <repo-url>
cd callbot
```

### 2. Create a Matrix account for the bot

Register a new account on any Matrix homeserver (e.g. matrix.org). Make note of:
- Homeserver URL (e.g. `https://matrix.org`)
- Full user ID (e.g. `@callbot:matrix.org`)
- Password

### 3. Configure environment

Copy the example env file and fill in your values:

```sh
cp .env.example .env
```

```env
# Required
BOT_HOMESERVER=https://matrix.org
BOT_USERNAME=@callbot:matrix.org
BOT_PASSWORD=your_bot_password

# Required — long random secret used to encrypt the E2EE key store.
# Generate with: openssl rand -hex 32
# Never change this after first run or you will lose your E2EE keys.
CALLBOT_PICKLE_KEY=change_me_to_a_long_random_secret

# Required for the widget feature — public HTTPS URL where the widget is served.
# See "Widget hosting" below.
WIDGET_URL=https://callbot.example.com

# Enable the built-in static file server (serves widget/index.html).
# Set to a port number to enable, leave unset to disable.
WIDGET_PORT=8080

# Optional: seed the watched room on first run.
# After that, use !callbot commands to change it — config is saved to config.json.
# WATCHED_ROOM_ID=!roomid:matrix.org

# Optional: override log level (debug / info / warn / error). Default: info
# LOG_LEVEL=info
```

### 4. Invite the bot and configure

Start the bot:

```sh
docker compose up -d
```

Invite `@callbot:matrix.org` to your call room, then send commands:

```
!callbot watch          — watch this room for call events
!callbot widget         — pin the status widget in the watched room
!callbot status         — show current config
!callbot help           — list all commands
```

---

## Widget hosting

The widget is a single static file (`widget/index.html`) that must be served over **HTTPS** — browsers block mixed content, and Element won't load an `http://` widget URL.

Choose one of the options below:

---

### Option A — Cloudflare Tunnel (recommended, no open ports)

Cloudflare Tunnel connects outbound to Cloudflare's network, so you don't need to open any firewall ports. Cloudflare handles HTTPS automatically.

**1.** Go to [Cloudflare Zero Trust](https://one.dash.cloudflare.com) → Networks → Tunnels → **Create a tunnel** → Docker.
Copy the tunnel token shown.

**2.** In the tunnel's **Public Hostnames** tab, add:
- Subdomain / Domain: `callbot.example.com`
- Service type: `HTTP`
- URL: `callbot:8080`

**3.** Add to `.env`:
```env
CLOUDFLARE_TOKEN=your_tunnel_token_here
WIDGET_URL=https://callbot.example.com
WIDGET_PORT=8080
```

**4.** The `compose.yml` already includes a `cloudflared` service — just deploy:
```sh
docker compose up -d
```

---

### Option B — nginx reverse proxy (no Cloudflare)

If you already run nginx on your server with an SSL certificate, you can proxy the widget port without Cloudflare.

**1.** Remove (or comment out) the `cloudflared` service from `compose.yml`, and bind the port to localhost only:

```yaml
services:
  callbot:
    ports:
      - "127.0.0.1:8080:8080"
```

**2.** Add an nginx server block:

```nginx
server {
    listen 443 ssl;
    server_name callbot.example.com;

    ssl_certificate     /etc/letsencrypt/live/callbot.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/callbot.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
    }
}
```

**3.** Add to `.env`:
```env
WIDGET_URL=https://callbot.example.com
WIDGET_PORT=8080
```

---

### Option C — External hosting (no built-in server)

You can host `widget/index.html` anywhere that serves static files over HTTPS — GitHub Pages, Cloudflare Pages, Vercel, S3, etc.

Just leave `WIDGET_PORT` unset (the bot won't start an HTTP server) and set `WIDGET_URL` to wherever you uploaded the file:

```env
WIDGET_URL=https://your-github-username.github.io/callbot
# WIDGET_PORT=   ← leave unset
```

---

## Bot commands

| Command | Description |
|---|---|
| `!callbot watch` | Watch the current room for call events |
| `!callbot watch <roomID>` | Watch a specific room |
| `!callbot widget` | Pin the status widget in the watched room |
| `!callbot widget remove` | Unpin the widget |
| `!callbot status` | Show current config and call state |
| `!callbot help` | List all commands |

Room config is saved to `config.json` (inside the `/data` Docker volume) and restored on restart.

---

## Building locally (without Docker)

Requires Go 1.21+ and `libolm` installed.

**macOS (Homebrew):**
```sh
brew install libolm
CGO_CFLAGS="-I/opt/homebrew/include" CGO_LDFLAGS="-L/opt/homebrew/lib" go run .
```

**Debian/Ubuntu:**
```sh
apt install libolm-dev
go run .
```

Or use the Makefile:
```sh
make run
make build
```

---

## Project structure

```
callbot/
├── main.go          # Entry point, sync loop, HTTP server
├── call.go          # Call member event handling, state tracking
├── commands.go      # !callbot command handler
├── widget.go        # Pin/unpin widget via Matrix state events
├── config.go        # Persist watched room to config.json
├── state.go         # Shared global state
├── widget/
│   └── index.html   # The widget web app (dark theme, self-contained)
├── Dockerfile
├── compose.yml
└── .env.example
```

---

## Notes

- The bot needs **power level 50+** in the watched room to pin widgets (send state events). Promote it in room settings.
- On first run the bot logs in with your password and stores credentials in `callbot.db`. Keep this file safe and back it up.
- `CALLBOT_PICKLE_KEY` encrypts the E2EE key store inside `callbot.db`. If you lose it or change it you will need to delete the database and re-login.
- The widget communicates directly with the Matrix homeserver from the user's browser — the bot is not involved after the widget is pinned.

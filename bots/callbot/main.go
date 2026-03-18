package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	godotenv.Load()

	homeserver  := os.Getenv("BOT_HOMESERVER")
	username    := os.Getenv("BOT_USERNAME")
	password    := os.Getenv("BOT_PASSWORD")
	pickleKey   := os.Getenv("CALLBOT_PICKLE_KEY")
	dbPath      := os.Getenv("CALLBOT_DB_PATH")
	cfgPath     := os.Getenv("CONFIG_PATH")
	logLevel    := os.Getenv("LOG_LEVEL")
	widgetPort  := os.Getenv("WIDGET_PORT")
	widgetDir   := os.Getenv("WIDGET_DIR")

	switch {
	case homeserver == "" || username == "" || password == "":
		log.Fatal("Missing required env vars: BOT_HOMESERVER, BOT_USERNAME, BOT_PASSWORD")
	case pickleKey == "":
		log.Fatal("Missing required env var: CALLBOT_PICKLE_KEY (set a long random secret, never change it)")
	}
	if dbPath == "" {
		dbPath = "callbot.db"
	}
	if cfgPath != "" {
		configPath = cfgPath
	}
	if widgetDir == "" {
		widgetDir = "widget"
	}

	// widgetURL is the public HTTPS URL users' browsers load the widget from.
	// Must be set if you intend to use !callbot widget.
	widgetURL = os.Getenv("WIDGET_URL")

	// Load persisted room config. Env vars act as a one-time bootstrap:
	// if config.json exists its value takes precedence.
	cfg := loadConfig()
	if cfg.WatchedRoom != "" {
		watchedRoom = cfg.WatchedRoom
	} else {
		watchedRoom = id.RoomID(os.Getenv("WATCHED_ROOM_ID"))
	}

	botUserID = id.UserID(username)

	var level zerolog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = zerolog.DebugLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	default:
		level = zerolog.InfoLevel
	}
	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Stamp}).
		With().Timestamp().Logger().Level(level)

	var err error
	client, err = mautrix.NewClient(homeserver, botUserID, "")
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}
	client.Log = zlog

	cryptoHelper, err = cryptohelper.NewCryptoHelper(client, []byte(pickleKey), dbPath)
	if err != nil {
		log.Fatal("Failed to create crypto helper:", err)
	}
	// LoginAs lets cryptohelper manage the device lifecycle — creates/reuses a
	// device, uploads E2EE keys, and persists credentials in the db.
	cryptoHelper.LoginAs = &mautrix.ReqLogin{
		Type:                     mautrix.AuthTypePassword,
		Identifier:               mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: botUserID.Localpart()},
		Password:                 password,
		InitialDeviceDisplayName: "callbot",
	}

	mainCtx, cancelMain := context.WithCancel(context.Background())

	if err = cryptoHelper.Init(mainCtx); err != nil {
		log.Fatal("Failed to init crypto:", err)
	}

	// Serve widget files over HTTP if WIDGET_PORT is set.
	// Put a reverse proxy (nginx/traefik) in front for HTTPS.
	if widgetPort != "" {
		go func() {
			fs := http.FileServer(http.Dir(widgetDir))
			zlog.Info().Str("port", widgetPort).Str("dir", widgetDir).Msg("Serving widget")
			if err := http.ListenAndServe(":"+widgetPort, fs); err != nil {
				zlog.Error().Err(err).Msg("Widget HTTP server error")
			}
		}()
	}

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// Auto-accept invites.
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if evt.GetStateKey() == username && evt.Content.AsMember().Membership == event.MembershipInvite {
			if _, err := client.JoinRoomByID(ctx, evt.RoomID); err == nil {
				zlog.Info().Str("room", evt.RoomID.String()).Msg("Joined room")
			}
		}
	})

	// Watch call member events (MSC3401 used by Element Call, MSC4143 newer variant).
	syncer.OnEventType(event.Type{Type: "org.matrix.msc3401.call.member", Class: event.StateEventType}, handleCallMember)
	syncer.OnEventType(event.Type{Type: "org.matrix.msc4143.rtc.member", Class: event.StateEventType}, handleCallMember)

	// Register no-op handlers for encrypted call-notify event types so the
	// crypto layer doesn't log "unsupported event type" warnings for them.
	noop := func(context.Context, *event.Event) {}
	syncer.OnEventType(event.Type{Type: "org.matrix.msc4075.call.notify", Class: event.MessageEventType}, noop)
	syncer.OnEventType(event.Type{Type: "org.matrix.msc4075.rtc.notification", Class: event.MessageEventType}, noop)

	// Commands: !callbot <status|watch|widget|help>
	syncer.OnEventType(event.EventMessage, handleCommand(zlog))

	startTime = time.Now().UnixMilli()

	if watchedRoom != "" {
		if _, err := client.JoinRoomByID(mainCtx, watchedRoom); err != nil {
			zlog.Error().Err(err).Str("room", watchedRoom.String()).Msg("Could not join watched room")
		} else {
			zlog.Info().Str("room", watchedRoom.String()).Msg("Ensured membership")
		}
		bootstrapCallState(mainCtx, zlog)
	}

	// Graceful shutdown on SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		zlog.Info().Msg("Shutting down...")
		cancelMain()
	}()

	zlog.Info().Msg("Bot started!")

	// Sync with retry so transient network errors don't kill the bot.
	for {
		if err := client.SyncWithContext(mainCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
			zlog.Error().Err(err).Msg("Sync error, retrying in 5s")
			select {
			case <-time.After(5 * time.Second):
			case <-mainCtx.Done():
				return
			}
		} else {
			break
		}
	}
	zlog.Info().Msg("Stopped.")
}

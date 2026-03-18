package main

import (
	"context"
	"errors"
	"log"
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
	dbPath   := os.Getenv("CALLBOT_DB_PATH")
	cfgPath  := os.Getenv("CONFIG_PATH")
	logLevel := os.Getenv("LOG_LEVEL")

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

	// Load persisted room config. Env vars act as a one-time bootstrap:
	// if config.json exists its values take precedence.
	cfg := loadConfig()
	if cfg.WatchedRoom != "" {
		watchedRoom = cfg.WatchedRoom
	} else {
		watchedRoom = id.RoomID(os.Getenv("WATCHED_ROOM_ID"))
	}
	if cfg.AnnounceRoom != "" {
		announceRoom = cfg.AnnounceRoom
	} else {
		announceRoom = id.RoomID(os.Getenv("ANNOUNCE_ROOM_ID"))
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
	// LoginAs lets cryptohelper manage the device lifecycle — creates/reuses a device,
	// uploads E2EE keys, and persists the access token in the db so re-login only
	// happens when credentials are missing.
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

	// Commands: !callbot <status|watch|announce|help>
	syncer.OnEventType(event.EventMessage, handleCommand(zlog))

	startTime = time.Now().UnixMilli()

	for _, roomID := range []id.RoomID{watchedRoom, announceRoom} {
		if roomID == "" {
			continue
		}
		if _, err := client.JoinRoomByID(mainCtx, roomID); err != nil {
			zlog.Error().Err(err).Str("room", roomID.String()).Msg("Could not join room")
		} else {
			zlog.Info().Str("room", roomID.String()).Msg("Ensured membership")
		}
	}

	// Bootstrap call state from the current room state so restarts during
	// an active call don't leave the card stale.
	bootstrapCallState(mainCtx, zlog)
	if participants := uniqueParticipants(); len(participants) > 0 {
		mu.Lock()
		callActive = true
		callStartedAt = time.Now()
		lastParticipants = participants
		callCtx, cancel := context.WithCancel(context.Background())
		callCtxCancel = cancel
		mu.Unlock()
		go tickMinutely(callCtx)
		zlog.Info().Int("participants", len(participants)).Msg("Active call detected on startup")
		plain, html := buildCardHTML(mainCtx, participants, 0, false)
		sendOrEditCard(mainCtx, plain, html)
	}

	// Graceful shutdown: on SIGTERM/SIGINT mark the call ended (if active)
	// so the card is left in a clean state, then stop syncing.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		zlog.Info().Msg("Shutting down...")

		mu.Lock()
		active := callActive
		started := callStartedAt
		saved := lastParticipants
		callActive = false
		if callCtxCancel != nil {
			callCtxCancel()
			callCtxCancel = nil
		}
		mu.Unlock()

		if active && len(saved) > 0 {
			duration := time.Since(started).Round(time.Second)
			plain, html := buildCardHTML(context.Background(), saved, duration, true)
			sendOrEditCard(context.Background(), plain, html)
		}

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

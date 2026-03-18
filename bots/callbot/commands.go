package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const prefix = "!callbot"

func handleCommand(zlog zerolog.Logger) func(ctx context.Context, evt *event.Event) {
	return func(ctx context.Context, evt *event.Event) {
		// Ignore own messages and anything older than startup.
		if evt.Sender == botUserID || evt.Timestamp < startTime {
			return
		}
		content := evt.Content.AsMessage()
		if content == nil {
			return
		}
		body := strings.TrimSpace(content.Body)
		if !strings.HasPrefix(body, prefix) {
			return
		}

		args := strings.Fields(strings.TrimPrefix(body, prefix))
		cmd := ""
		if len(args) > 0 {
			cmd = strings.ToLower(args[0])
		}

		switch cmd {
		case "status":
			reply(ctx, evt.RoomID, buildStatus())

		case "watch":
			newRoom := evt.RoomID // default: current room
			if len(args) >= 2 {
				newRoom = id.RoomID(args[1])
			}
			if _, err := client.JoinRoomByID(ctx, newRoom); err != nil {
				reply(ctx, evt.RoomID, fmt.Sprintf("❌ Could not join room: %v", err))
				return
			}
			mu.Lock()
			watchedRoom = newRoom
			mu.Unlock()
			saveConfig()
			bootstrapCallState(ctx, zlog)
			reply(ctx, evt.RoomID, fmt.Sprintf("✅ Now watching %s", newRoom))

		case "widget":
			if watchedRoom == "" {
				reply(ctx, evt.RoomID, "❌ No watched room set — run !callbot watch first")
				return
			}
			if widgetURL == "" {
				reply(ctx, evt.RoomID, "❌ WIDGET_URL is not configured")
				return
			}
			sub := ""
			if len(args) >= 2 {
				sub = strings.ToLower(args[1])
			}
			switch sub {
			case "remove", "unpin", "off":
				if err := unpinWidget(ctx, watchedRoom); err != nil {
					reply(ctx, evt.RoomID, fmt.Sprintf("❌ Could not remove widget: %v", err))
				} else {
					reply(ctx, evt.RoomID, "✅ Widget removed")
				}
			default:
				if err := pinWidget(ctx, watchedRoom, widgetURL); err != nil {
					reply(ctx, evt.RoomID, fmt.Sprintf("❌ Could not pin widget: %v", err))
				} else {
					reply(ctx, evt.RoomID, fmt.Sprintf("✅ Widget pinned in %s", watchedRoom))
				}
			}

		case "help", "h", "":
			reply(ctx, evt.RoomID,
				"!callbot status — current state\n"+
					"!callbot watch [roomID] — watch a room (default: this room)\n"+
					"!callbot widget — pin call status widget in watched room\n"+
					"!callbot widget remove — unpin widget\n"+
					"!callbot help — this message",
			)

		default:
			reply(ctx, evt.RoomID, fmt.Sprintf("Unknown command %q — try !callbot help", cmd))
		}
	}
}

func buildStatus() string {
	mu.Lock()
	wr     := watchedRoom
	active := callActive
	started := callStartedAt
	n := 0
	seen := map[id.UserID]bool{}
	for _, uid := range activeDevices {
		if !seen[uid] { seen[uid] = true; n++ }
	}
	mu.Unlock()

	wURL := widgetURL
	lines := []string{
		fmt.Sprintf("👁 Watching:  %s", wr),
		fmt.Sprintf("🌐 Widget URL: %s", func() string {
			if wURL == "" { return "(not set)" }
			return wURL
		}()),
	}
	if active {
		elapsed := time.Since(started).Round(time.Second)
		lines = append(lines, fmt.Sprintf("🟢 Call active — %d participant(s) — %s", n, fmtDuration(elapsed)))
	} else {
		lines = append(lines, "⚫ No active call")
	}
	return strings.Join(lines, "\n")
}

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func reply(ctx context.Context, roomID id.RoomID, text string) {
	content := event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text,
		Mentions: &event.Mentions{},
	}
	client.SendMessageEvent(ctx, roomID, event.EventMessage, &content)
}

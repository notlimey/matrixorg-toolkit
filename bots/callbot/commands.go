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
			resetCallState()
			mu.Lock()
			watchedRoom = newRoom
			watchedRoomName = ""
			mu.Unlock()
			saveConfig()
			bootstrapCallState(ctx, zlog)
			reply(ctx, evt.RoomID, fmt.Sprintf("✅ Now watching %s", newRoom))
			if participants := uniqueParticipants(); len(participants) > 0 {
				mu.Lock()
				callActive = true
				callStartedAt = time.Now()
				lastParticipants = participants
				callCtx, cancel := context.WithCancel(context.Background())
				callCtxCancel = cancel
				mu.Unlock()
				go tickMinutely(callCtx)
				plain, html := buildCardHTML(ctx, participants, 0, false)
				sendOrEditCard(ctx, plain, html)
			}

		case "announce":
			if len(args) < 2 {
				reply(ctx, evt.RoomID, "Usage: !callbot announce <roomID>")
				return
			}
			newRoom := id.RoomID(args[1])
			if _, err := client.JoinRoomByID(ctx, newRoom); err != nil {
				reply(ctx, evt.RoomID, fmt.Sprintf("❌ Could not join room: %v", err))
				return
			}
			mu.Lock()
			announceRoom = newRoom
			announceMsgID = ""
			mu.Unlock()
			saveConfig()
			reply(ctx, evt.RoomID, fmt.Sprintf("✅ Announce room set to %s", newRoom))

		case "help", "h", "":
			reply(ctx, evt.RoomID,
				"!callbot status — current state\n"+
					"!callbot watch <roomID> — change watched room\n"+
					"!callbot announce <roomID> — change announce room\n"+
					"!callbot help — this message",
			)

		default:
			reply(ctx, evt.RoomID, fmt.Sprintf("Unknown command %q — try !callbot help", cmd))
		}
	}
}

// resetCallState tears down any active call tracking, ready for a room switch.
func resetCallState() {
	mu.Lock()
	defer mu.Unlock()
	if callCtxCancel != nil {
		callCtxCancel()
		callCtxCancel = nil
	}
	callActive = false
	callStartedAt = time.Time{}
	lastParticipants = nil
	announceMsgID = ""
	activeDevices = map[string]id.UserID{}
	profileCache = map[id.UserID]*cachedProfile{}
}

func buildStatus() string {
	mu.Lock()
	wr := watchedRoom
	ar := announceRoom
	active := callActive
	started := callStartedAt
	participants := make([]id.UserID, 0, len(activeDevices))
	seen := map[id.UserID]bool{}
	for _, uid := range activeDevices {
		if !seen[uid] {
			seen[uid] = true
			participants = append(participants, uid)
		}
	}
	mu.Unlock()

	lines := []string{
		fmt.Sprintf("👁 Watching:  %s", wr),
		fmt.Sprintf("📣 Announce:  %s", ar),
	}
	if active {
		elapsed := time.Since(started).Round(time.Second)
		lines = append(lines, fmt.Sprintf("🟢 Call active — %d participant(s) — %s", len(participants), formatElapsed(elapsed)))
	} else {
		lines = append(lines, "⚫ No active call")
	}
	return strings.Join(lines, "\n")
}

func reply(ctx context.Context, roomID id.RoomID, text string) {
	content := event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text,
		Mentions: &event.Mentions{},
	}
	client.SendMessageEvent(ctx, roomID, event.EventMessage, &content)
}

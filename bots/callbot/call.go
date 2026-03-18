package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// CallMemberContent is the per-device call membership content (Element Call / MSC3401).
// Active member: Application is set. Left: empty content {}.
type CallMemberContent struct {
	Application string `json:"application"`
	DeviceID    string `json:"device_id"`
}

// parseUserIDFromStateKey extracts @user:server from a call member state key.
// New per-device format: _@user:server_DEVICEID_m.call → @user:server
// Legacy format: state key is the bare user ID.
func parseUserIDFromStateKey(stateKey string) id.UserID {
	if strings.HasPrefix(stateKey, "_@") {
		rest := stateKey[1:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			return ""
		}
		underscoreIdx := strings.Index(rest[colonIdx:], "_")
		if underscoreIdx < 0 {
			return id.UserID(rest)
		}
		return id.UserID(rest[:colonIdx+underscoreIdx])
	}
	return id.UserID(stateKey)
}

// bootstrapCallState queries the current room state to populate activeDevices
// before the sync loop starts, so restarts during an active call stay accurate.
func bootstrapCallState(ctx context.Context, zlog zerolog.Logger) {
	stateMap, err := client.State(ctx, watchedRoom)
	if err != nil {
		log.Printf("Could not fetch initial room state for bootstrap: %v", err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for evtType, keyMap := range stateMap {
		if evtType.Type != "org.matrix.msc3401.call.member" && evtType.Type != "org.matrix.msc4143.rtc.member" {
			continue
		}
		for stateKey, evt := range keyMap {
			userID := parseUserIDFromStateKey(stateKey)
			if userID == "" || userID == botUserID {
				continue
			}
			raw, _ := json.Marshal(evt.Content.Raw)
			var content CallMemberContent
			json.Unmarshal(raw, &content)
			if content.Application != "" {
				activeDevices[stateKey] = userID
				if !callActive {
					callActive = true
					callStartedAt = time.Now()
				}
				zlog.Info().Str("user", string(userID)).Str("device", content.DeviceID).Msg("bootstrap: active in call")
			}
		}
	}
}

func handleCallMember(ctx context.Context, evt *event.Event) {
	if evt.RoomID != watchedRoom {
		return
	}
	// Ignore historical timeline events replayed during initial sync.
	if evt.Timestamp < startTime {
		return
	}

	stateKey := evt.GetStateKey()
	userID := parseUserIDFromStateKey(stateKey)
	if userID == "" || userID == botUserID {
		return
	}

	raw, _ := json.Marshal(evt.Content.Raw)
	var content CallMemberContent
	json.Unmarshal(raw, &content)

	isActive := content.Application != ""

	mu.Lock()
	_, wasActive := activeDevices[stateKey]
	if isActive == wasActive {
		mu.Unlock()
		return
	}
	if isActive {
		activeDevices[stateKey] = userID
		if !callActive {
			callActive = true
			callStartedAt = time.Now()
		}
		log.Printf("joined: %s (device %s)", userID, content.DeviceID)
	} else {
		delete(activeDevices, stateKey)
		log.Printf("left: %s", userID)
		if len(activeDevices) == 0 {
			callActive = false
		}
	}
	mu.Unlock()
}

func uniqueParticipants() []id.UserID {
	mu.Lock()
	defer mu.Unlock()
	seen := map[id.UserID]bool{}
	for _, uid := range activeDevices {
		seen[uid] = true
	}
	users := make([]id.UserID, 0, len(seen))
	for uid := range seen {
		users = append(users, uid)
	}
	return users
}

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func getProfile(ctx context.Context, userID id.UserID) *cachedProfile {
	mu.Lock()
	p, ok := profileCache[userID]
	mu.Unlock()
	if ok {
		return p
	}
	p = &cachedProfile{}
	if profile, err := client.GetProfile(ctx, userID); err == nil {
		p.displayName = profile.DisplayName
		if !profile.AvatarURL.IsEmpty() {
			p.avatarMXC = profile.AvatarURL.String()
		}
	}
	mu.Lock()
	profileCache[userID] = p
	mu.Unlock()
	return p
}

func getRoomName(ctx context.Context) string {
	mu.Lock()
	name := watchedRoomName
	mu.Unlock()
	if name != "" {
		return name
	}
	var nameContent event.RoomNameEventContent
	if err := client.StateEvent(ctx, watchedRoom, event.StateRoomName, "", &nameContent); err == nil && nameContent.Name != "" {
		mu.Lock()
		watchedRoomName = nameContent.Name
		mu.Unlock()
		return nameContent.Name
	}
	return ""
}

func displayName(p *cachedProfile, uid id.UserID) string {
	if p.displayName != "" {
		return p.displayName
	}
	local := strings.TrimPrefix(strings.Split(string(uid), ":")[0], "@")
	return local
}

func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatDuration(d time.Duration) string {
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

// buildParticipantLine returns an HTML line of participant names like:
// 👤 Alex · 👤 Maya · 👤 Jordan · <grey>+1 more</grey>
func buildParticipantLine(participants []id.UserID, profiles []*cachedProfile) string {
	const maxShown = 5
	shown := participants
	shownProfiles := profiles
	extra := 0
	if len(participants) > maxShown {
		shown = participants[:maxShown]
		shownProfiles = profiles[:maxShown]
		extra = len(participants) - maxShown
	}

	var parts []string
	for i, uid := range shown {
		parts = append(parts, "👤 <b>"+displayName(shownProfiles[i], uid)+"</b>")
	}
	if extra > 0 {
		parts = append(parts, fmt.Sprintf(`<font data-mx-color="#6b7280">+%d more</font>`, extra))
	}
	return strings.Join(parts, " · ")
}

func buildCardHTML(ctx context.Context, participants []id.UserID, elapsed time.Duration, ended bool) (plain, html string) {
	roomName := getRoomName(ctx)

	profiles := make([]*cachedProfile, len(participants))
	for i, uid := range participants {
		profiles[i] = getProfile(ctx, uid)
	}

	countText := fmt.Sprintf("%d participant", len(participants))
	if len(participants) != 1 {
		countText += "s"
	}
	participantLine := buildParticipantLine(participants, profiles)

	if ended {
		durStr := formatDuration(elapsed)
		plain = fmt.Sprintf("📵 Call ended — lasted %s", durStr)
		html = "⚫ <b><font data-mx-color=\"#6b7280\">Call ended</font></b>" +
			" · <font data-mx-color=\"#6b7280\">lasted " + durStr + "</font><br>" +
			"<b>" + roomName + "</b>"
	} else {
		elapsedStr := formatElapsed(elapsed)
		plain = fmt.Sprintf("📞 %s — %s — %s", roomName, countText, elapsedStr)
		html = "🟢 <b><font data-mx-color=\"#16a34a\">Live call</font></b>" +
			" · <font data-mx-color=\"#6b7280\">" + elapsedStr + "</font><br>" +
			"<b>" + roomName + "</b><br>" +
			"<font data-mx-color=\"#6b7280\">" + countText + "</font><br><br>" +
			participantLine
	}
	return plain, html
}

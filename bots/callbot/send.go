package main

import (
	"context"
	"log"

	"maunium.net/go/mautrix/event"
)

// buildContent creates a Matrix HTML message content with no mentions (silent — won't ping anyone).
func buildContent(plain, html string) event.MessageEventContent {
	return event.MessageEventContent{
		MsgType:       event.MsgText,
		Format:        event.FormatHTML,
		Body:          plain,
		FormattedBody: html,
		Mentions:      &event.Mentions{}, // m.mentions:{} = intentionally no mentions, suppresses pings
	}
}

// sendOrEditCard sends a new card or silently edits the existing one.
func sendOrEditCard(ctx context.Context, plain, html string) {
	mu.Lock()
	msgID := announceMsgID
	room := announceRoom
	mu.Unlock()

	if room == "" {
		log.Printf("announce room not set — use '!callbot announce' to configure it")
		return
	}

	content := buildContent(plain, html)

	if msgID == "" {
		// New message
		encrypted, err := cryptoHelper.Encrypt(ctx, announceRoom, event.EventMessage, &content)
		if err == nil {
			resp, err := client.SendMessageEvent(ctx, announceRoom, event.EventEncrypted, encrypted)
			if err != nil {
				log.Printf("failed to send encrypted card: %v", err)
				return
			}
			mu.Lock()
			announceMsgID = resp.EventID
			mu.Unlock()
		} else {
			resp, err := client.SendMessageEvent(ctx, announceRoom, event.EventMessage, &content)
			if err != nil {
				log.Printf("failed to send card: %v", err)
				return
			}
			mu.Lock()
			announceMsgID = resp.EventID
			mu.Unlock()
		}
	} else {
		// Edit existing message
		newContent := buildContent(plain, html)
		content.NewContent = &newContent
		content.RelatesTo = &event.RelatesTo{
			Type:    event.RelReplace,
			EventID: msgID,
		}
		encrypted, err := cryptoHelper.Encrypt(ctx, announceRoom, event.EventMessage, &content)
		if err == nil {
			client.SendMessageEvent(ctx, announceRoom, event.EventEncrypted, encrypted)
		} else {
			client.SendMessageEvent(ctx, announceRoom, event.EventMessage, &content)
		}
	}
}

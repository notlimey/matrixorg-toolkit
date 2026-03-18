package main

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const widgetStateKey = "callbot-status-widget"

// pinWidget sends an im.vector.modular.widgets state event that pins the call
// status widget in the given room. Element substitutes the $matrix_* template
// vars before loading the iframe.
func pinWidget(ctx context.Context, roomID id.RoomID, baseURL string) error {
	url := fmt.Sprintf(
		"%s?widgetId=$widgetId&userId=$matrix_user_id&roomId=$matrix_room_id",
		baseURL,
	)
	content := map[string]any{
		"type":              "m.custom",
		"url":               url,
		"name":              "📞 Call Status",
		"waitForIframeLoad": true,
		"data":              map[string]any{},
	}
	_, err := client.SendStateEvent(ctx, roomID,
		event.Type{Type: "im.vector.modular.widgets", Class: event.StateEventType},
		widgetStateKey, content)
	return err
}

// unpinWidget removes the widget by sending an empty-content state event.
func unpinWidget(ctx context.Context, roomID id.RoomID) error {
	_, err := client.SendStateEvent(ctx, roomID,
		event.Type{Type: "im.vector.modular.widgets", Class: event.StateEventType},
		widgetStateKey, map[string]any{})
	return err
}

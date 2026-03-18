package main

import (
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

var (
	mu            sync.Mutex
	client        *mautrix.Client
	cryptoHelper  *cryptohelper.CryptoHelper
	botUserID     id.UserID
	startTime     int64 // unix ms — ignore events older than this
	activeDevices = map[string]id.UserID{} // stateKey -> userID
	callActive    = false
	callStartedAt time.Time
	watchedRoom   id.RoomID
	widgetURL     string // public HTTPS URL of the widget web app
)

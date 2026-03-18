package main

import (
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

type cachedProfile struct {
	displayName string
	avatarMXC   string
}

var (
	mu               sync.Mutex
	client           *mautrix.Client
	cryptoHelper     *cryptohelper.CryptoHelper
	botUserID        id.UserID
	startTime        int64 // unix ms — ignore events older than this
	activeDevices    = map[string]id.UserID{} // stateKey -> userID
	callActive       = false
	callStartedAt    time.Time
	callCtxCancel    func()
	announceMsgID    id.EventID
	lastParticipants []id.UserID
	watchedRoom      id.RoomID
	announceRoom     id.RoomID
	profileCache     = map[id.UserID]*cachedProfile{}
	watchedRoomName  string
)

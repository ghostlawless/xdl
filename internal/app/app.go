package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	ModeVerbose RunMode = iota
	ModeQuiet
	ModeDebug
)

func generateRunID() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

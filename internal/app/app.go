package app

import (
	"crypto/rand"
	"encoding/hex"
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
		n := uint64(time.Now().UnixNano())
		b[0] = byte(n)
		b[1] = byte(n >> 8)
		b[2] = byte(n >> 16)
	}
	return hex.EncodeToString(b[:])
}

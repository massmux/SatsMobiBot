package lnurl

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Tag []string

type Tags []Tag

type NostrEvent struct {
	ID        string    `json:"id"`
	PubKey    string    `json:"pubkey"`
	CreatedAt time.Time `json:"created_at"`
	Kind      int       `json:"kind"`
	Tags      Tags      `json:"tags"`
	Content   string    `json:"content"`
	Sig       string    `json:"sig"`
}

// DescriptionHash is the SHA256 hash of the metadata
func (w Lnurl) Nip57DescriptionHash(zapEventSerialized string) string {
	hash := sha256.Sum256([]byte(zapEventSerialized))
	hashString := hex.EncodeToString(hash[:])
	return hashString
}

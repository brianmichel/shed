package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

const Version = "1"

type Message struct {
	Version    string         `json:"version"`
	Type       string         `json:"type"`
	MessageID  string         `json:"message_id"`
	AckID      string         `json:"ack_id,omitempty"`
	RequestID  string         `json:"request_id"`
	SessionID  string         `json:"session_id"`
	SandboxID  string         `json:"sandbox_id"`
	Seq        int64          `json:"seq"`
	Timestamp  time.Time      `json:"timestamp"`
	ExpectsAck bool           `json:"expects_ack"`
	ReplyTo    string         `json:"reply_to,omitempty"`
	Payload    map[string]any `json:"payload"`
}

func New(msgType, sessionID, sandboxID string, seq int64, payload map[string]any) Message {
	return Message{Version: Version, Type: msgType, MessageID: NewID("msg"), RequestID: NewID("req"), SessionID: sessionID, SandboxID: sandboxID, Seq: seq, Timestamp: time.Now().UTC(), Payload: payload, ExpectsAck: false}
}

func Ack(replyTo Message, seq int64) Message {
	m := New("ack", replyTo.SessionID, replyTo.SandboxID, seq, map[string]any{"status": "accepted"})
	m.AckID = replyTo.MessageID
	m.RequestID = replyTo.RequestID
	m.ReplyTo = replyTo.MessageID
	return m
}

func Validate(m Message) error {
	if m.Version == "" {
		m.Version = Version
	}
	if m.Version != Version {
		return errors.New("unsupported_protocol_version")
	}
	if m.Type == "" {
		return errors.New("missing_type")
	}
	if m.MessageID == "" {
		return errors.New("missing_message_id")
	}
	if m.RequestID == "" {
		return errors.New("missing_request_id")
	}
	if m.SessionID == "" {
		return errors.New("missing_session_id")
	}
	if m.SandboxID == "" {
		return errors.New("missing_sandbox_id")
	}
	if m.Seq <= 0 {
		return errors.New("invalid_seq")
	}
	return nil
}

func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// Package transport handles WebSocket IO for the Garden connection.
// It dials the endpoint, sends JSON messages, and delivers received messages
// to a channel. It knows nothing about the Garden protocol itself.
package transport

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
)

// Message is a raw Garden message as received from the server.
type Message struct {
	Type    string
	Seq     int64
	Payload map[string]any
}

// Transport is a WebSocket connection to Garden.
type Transport struct {
	ws       *websocket.Conn
	mu       sync.Mutex
	incoming chan Message
}

// New dials Garden and returns a ready Transport.
func New(wsURL, sessionKey, sandboxID string) (*Transport, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	q := u.Query()
	q.Set("session_key", sessionKey)
	q.Set("sandbox_id", sandboxID)
	u.RawQuery = q.Encode()

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return &Transport{ws: ws, incoming: make(chan Message, 64)}, nil
}

// Send encodes msg as JSON and writes it to the WebSocket.
func (t *Transport) Send(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ws.WriteMessage(websocket.TextMessage, data)
}

// Messages returns the channel on which decoded messages arrive.
func (t *Transport) Messages() <-chan Message { return t.incoming }

// ReadLoop reads frames from the WebSocket, decodes them, and delivers them to
// Messages(). It returns when the connection is closed or an error occurs.
func (t *Transport) ReadLoop() error {
	defer close(t.incoming)
	for {
		_, data, err := t.ws.ReadMessage()
		if err != nil {
			return err
		}
		var env struct {
			Type    string         `json:"type"`
			Seq     int64          `json:"seq"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Type == "" {
			continue
		}
		t.incoming <- Message{Type: env.Type, Seq: env.Seq, Payload: env.Payload}
	}
}

package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nhooyr.io/websocket"
)

const (
	TypeHello    = "hello"
	TypeReady    = "ready"
	TypeRequest  = "request"
	TypeResponse = "response"
	TypeError    = "error"
	MaxBodyBytes = 1 << 20
)

// Envelope is the framing structure exchanged between server and client.
type Envelope struct {
	Type     string            `json:"type"`
	Hello    *HelloPayload     `json:"hello,omitempty"`
	Ready    *ReadyPayload     `json:"ready,omitempty"`
	Request  *RequestPayload   `json:"request,omitempty"`
	Response *ResponsePayload  `json:"response,omitempty"`
	Error    string            `json:"error,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

type HelloPayload struct {
	Subdomain string `json:"subdomain"`
	PublicKey string `json:"public_key"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

type ReadyPayload struct {
	Message   string `json:"message"`
	Subdomain string `json:"subdomain,omitempty"`
}

type RequestPayload struct {
	ID      string              `json:"id"`
	Method  string              `json:"method"`
	Target  string              `json:"target"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type ResponsePayload struct {
	ID      string              `json:"id"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

// Write sends the envelope using JSON text frames.
func Write(ctx context.Context, conn *websocket.Conn, env *Envelope) error {
	bytes, err := json.Marshal(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, bytes)
}

// Read retrieves an envelope from the peer.
func Read(ctx context.Context, conn *websocket.Conn) (*Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageText {
		return nil, errors.New("unexpected frame type")
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

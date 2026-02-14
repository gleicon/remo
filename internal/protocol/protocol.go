package protocol

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	TypeHello    = "hello"
	TypeReady    = "ready"
	TypeRequest  = "request"
	TypeResponse = "response"
	TypeError    = "error"
	MaxBodyBytes = 1 << 20
)

const ProxyChannelType = "remo-proxy"

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

type ReadWriter interface {
	io.ReadWriteCloser
}

func Write(ctx context.Context, rw ReadWriter, env *Envelope) error {
	bytes, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(bytes) > MaxBodyBytes {
		return errors.New("message too large")
	}
	sizeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBuf, uint32(len(bytes)))

	done := make(chan error, 1)
	go func() {
		_, err := rw.Write(sizeBuf)
		if err != nil {
			done <- err
			return
		}
		_, err = rw.Write(bytes)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func Read(ctx context.Context, rw ReadWriter) (*Envelope, error) {
	header := make([]byte, 4)
	done := make(chan error, 1)
	var n int
	go func() {
		var err error
		n, err = rw.Read(header)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			return nil, err
		}
	}
	if n < 4 {
		return nil, errors.New("incomplete header")
	}
	size := int(binary.BigEndian.Uint32(header))
	if size > MaxBodyBytes {
		return nil, errors.New("message too large")
	}
	data := make([]byte, size)
	readDone := make(chan error, 1)
	go func() {
		var err error
		_, err = io.ReadFull(rw, data)
		readDone <- err
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-readDone:
		if err != nil {
			return nil, err
		}
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

func ReadRequest(ctx context.Context, rw ReadWriter) (*Envelope, error) {
	return Read(ctx, rw)
}

func WriteResponse(ctx context.Context, rw ReadWriter, env *Envelope) error {
	return Write(ctx, rw, env)
}

func SetReadDeadline(rw io.Reader, t time.Time) error {
	if rd, ok := rw.(interface{ SetReadDeadline(time.Time) error }); ok {
		return rd.SetReadDeadline(t)
	}
	return nil
}

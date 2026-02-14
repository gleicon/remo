package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/gleicon/remo/internal/protocol"
)

type Tunnel struct {
	subdomain string
	pubKey    string
	conn      protocol.ReadWriter
	log       zerolog.Logger
	closing   chan struct{}
	inflight  map[string]chan *protocol.ResponsePayload
	mu        sync.Mutex
	counter   uint64
}

func newTunnel(subdomain, pubKey string, conn protocol.ReadWriter, log zerolog.Logger) *Tunnel {
	return &Tunnel{
		subdomain: subdomain,
		pubKey:    pubKey,
		conn:      conn,
		log:       log,
		closing:   make(chan struct{}),
		inflight:  make(map[string]chan *protocol.ResponsePayload),
	}
}

func (t *Tunnel) close(reason error) {
	t.mu.Lock()
	select {
	case <-t.closing:
	default:
		close(t.closing)
	}
	for id, ch := range t.inflight {
		close(ch)
		delete(t.inflight, id)
	}
	t.mu.Unlock()
	if reason != nil {
		t.log.Warn().Err(reason).Str("subdomain", t.subdomain).Msg("tunnel closed")
	} else {
		t.log.Info().Str("subdomain", t.subdomain).Msg("tunnel closed")
	}
}

func (t *Tunnel) nextRequestID() string {
	id := atomic.AddUint64(&t.counter, 1)
	return fmt.Sprintf("%s-%d", t.subdomain, id)
}

func (t *Tunnel) sendRequest(ctx context.Context, req *protocol.RequestPayload) (*protocol.ResponsePayload, error) {
	select {
	case <-t.closing:
		return nil, errors.New("tunnel closed")
	default:
	}
	req.ID = t.nextRequestID()
	respCh := make(chan *protocol.ResponsePayload, 1)
	t.mu.Lock()
	t.inflight[req.ID] = respCh
	t.mu.Unlock()
	if err := protocol.Write(ctx, t.conn, &protocol.Envelope{Type: protocol.TypeRequest, Request: req}); err != nil {
		t.removeInflight(req.ID)
		return nil, err
	}
	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, errors.New("tunnel response missing")
		}
		return resp, nil
	case <-ctx.Done():
		t.removeInflight(req.ID)
		return nil, ctx.Err()
	case <-t.closing:
		t.removeInflight(req.ID)
		return nil, errors.New("tunnel closed")
	}
}

func (t *Tunnel) removeInflight(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, id)
}

func (t *Tunnel) runReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			t.close(ctx.Err())
			return
		case <-t.closing:
			return
		default:
		}
		env, err := protocol.Read(ctx, t.conn)
		if err != nil {
			t.close(err)
			return
		}
		t.handleEnvelope(env)
	}
}

func (t *Tunnel) keepalive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.closing:
			return
		case <-ticker.C:
			_ = t.conn.Close()
		}
	}
}

func (t *Tunnel) handleEnvelope(env *protocol.Envelope) {
	switch env.Type {
	case protocol.TypeResponse:
		if env.Response == nil {
			return
		}
		t.mu.Lock()
		ch := t.inflight[env.Response.ID]
		if ch != nil {
			delete(t.inflight, env.Response.ID)
		}
		t.mu.Unlock()
		if ch != nil {
			ch <- env.Response
		}
	default:
	}
}

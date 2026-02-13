package protocol

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeTypes(t *testing.T) {
	if TypeHello != "hello" {
		t.Fatalf("unexpected TypeHello: %s", TypeHello)
	}
	if TypeReady != "ready" {
		t.Fatalf("unexpected TypeReady: %s", TypeReady)
	}
	if TypeRequest != "request" {
		t.Fatalf("unexpected TypeRequest: %s", TypeRequest)
	}
	if TypeResponse != "response" {
		t.Fatalf("unexpected TypeResponse: %s", TypeResponse)
	}
	if TypeError != "error" {
		t.Fatalf("unexpected TypeError: %s", TypeError)
	}
}

func TestMaxBodyBytes(t *testing.T) {
	if MaxBodyBytes != 1<<20 {
		t.Fatalf("expected 1MB, got %d", MaxBodyBytes)
	}
}

func TestEnvelopeMarshalHello(t *testing.T) {
	env := &Envelope{
		Type: TypeHello,
		Hello: &HelloPayload{
			Subdomain: "foo",
			PublicKey: "abc123",
			Timestamp: 1234567890,
			Signature: "sig",
		},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != TypeHello {
		t.Fatalf("type mismatch: %s", decoded.Type)
	}
	if decoded.Hello == nil {
		t.Fatal("hello payload is nil")
	}
	if decoded.Hello.Subdomain != "foo" {
		t.Fatalf("subdomain mismatch: %s", decoded.Hello.Subdomain)
	}
	if decoded.Hello.Timestamp != 1234567890 {
		t.Fatalf("timestamp mismatch: %d", decoded.Hello.Timestamp)
	}
}

func TestEnvelopeMarshalReady(t *testing.T) {
	env := &Envelope{
		Type:  TypeReady,
		Ready: &ReadyPayload{Message: "ready"},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Ready == nil || decoded.Ready.Message != "ready" {
		t.Fatal("ready payload mismatch")
	}
}

func TestEnvelopeMarshalRequest(t *testing.T) {
	env := &Envelope{
		Type: TypeRequest,
		Request: &RequestPayload{
			ID:      "req-1",
			Method:  "GET",
			Target:  "/api/test",
			Headers: map[string][]string{"Accept": {"application/json"}},
			Body:    []byte("request body"),
		},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Request == nil {
		t.Fatal("request payload is nil")
	}
	if decoded.Request.ID != "req-1" {
		t.Fatalf("id mismatch: %s", decoded.Request.ID)
	}
	if decoded.Request.Method != "GET" {
		t.Fatalf("method mismatch: %s", decoded.Request.Method)
	}
	if decoded.Request.Target != "/api/test" {
		t.Fatalf("target mismatch: %s", decoded.Request.Target)
	}
	if string(decoded.Request.Body) != "request body" {
		t.Fatalf("body mismatch: %s", decoded.Request.Body)
	}
}

func TestEnvelopeMarshalResponse(t *testing.T) {
	env := &Envelope{
		Type: TypeResponse,
		Response: &ResponsePayload{
			ID:      "req-1",
			Status:  200,
			Headers: map[string][]string{"Content-Type": {"text/plain"}},
			Body:    []byte("ok"),
		},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Response == nil {
		t.Fatal("response payload is nil")
	}
	if decoded.Response.Status != 200 {
		t.Fatalf("status mismatch: %d", decoded.Response.Status)
	}
}

func TestEnvelopeMarshalError(t *testing.T) {
	env := &Envelope{
		Type:  TypeError,
		Error: "something went wrong",
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Error != "something went wrong" {
		t.Fatalf("error mismatch: %s", decoded.Error)
	}
}

func TestEnvelopeOmitsNilFields(t *testing.T) {
	env := &Envelope{Type: TypeReady, Ready: &ReadyPayload{Message: "ok"}}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["hello"]; ok {
		t.Fatal("hello should be omitted")
	}
	if _, ok := raw["request"]; ok {
		t.Fatal("request should be omitted")
	}
	if _, ok := raw["response"]; ok {
		t.Fatal("response should be omitted")
	}
}

func TestEnvelopeMeta(t *testing.T) {
	env := &Envelope{
		Type: TypeReady,
		Meta: map[string]string{"version": "1.0"},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Meta["version"] != "1.0" {
		t.Fatalf("meta mismatch: %v", decoded.Meta)
	}
}

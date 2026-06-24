package helpers

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, map[string]any{"hello": "world", "n": 42})

	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	// Default recorder status is 200 — WriteJSON must not have written an error code.
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("body[hello] = %v, want world", got["hello"])
	}
}

// TestWriteJSON_MarshalFailureSends500 covers the comment in write.go: an
// un-marshalable value must produce a clean 500, not a half-written 200 body.
func TestWriteJSON_MarshalFailureSends500(t *testing.T) {
	rec := httptest.NewRecorder()
	// A channel cannot be JSON-encoded, so json.Marshal fails before any write.
	WriteJSON(rec, make(chan int))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on marshal failure", rec.Code)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusNotFound)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got struct {
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got.Status != http.StatusNotFound {
		t.Errorf("body.status = %d, want 404", got.Status)
	}
	if got.Error != http.StatusText(http.StatusNotFound) {
		t.Errorf("body.error = %q, want %q", got.Error, http.StatusText(http.StatusNotFound))
	}
}

func TestWriteXML(t *testing.T) {
	type payload struct {
		XMLName xml.Name `xml:"payload"`
		Value   string   `xml:"value"`
	}
	rec := httptest.NewRecorder()
	WriteXML(rec, payload{Value: "ok"})

	if ct := rec.Header().Get("Content-Type"); ct != "text/xml; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/xml; charset=utf-8", ct)
	}

	var got payload
	if err := xml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid XML: %v", err)
	}
	if got.Value != "ok" {
		t.Errorf("body.value = %q, want ok", got.Value)
	}
}

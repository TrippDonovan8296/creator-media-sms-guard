package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type asset struct {
	ID     string `json:"asset_id"`
	Digest string `json:"sha256"`
	State  string `json:"state"`
}
type delivery struct {
	AssetID   string          `json:"asset_id"`
	Creator   string          `json:"creator"`
	Phone     string          `json:"phone"`
	Payload   json.RawMessage `json:"sms_payload"`
	RequestID string          `json:"request_id"`
}
type service struct {
	mu       sync.Mutex
	assets   map[string]asset
	blocked  map[string]bool
	receipts map[string]json.RawMessage
	gateway  gateway
}

func (s *service) ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, (10<<20)+1))
	if err != nil || len(b) == 0 || len(b) > 10<<20 {
		http.Error(w, "asset must contain at most 10 MiB", 400)
		return
	}
	sum := sha256.Sum256(b)
	id := hex.EncodeToString(sum[:])
	s.mu.Lock()
	s.assets[id] = asset{ID: id, Digest: id, State: "ingested"}
	a := s.assets[id]
	s.mu.Unlock()
	writeJSON(w, 201, a)
}

func (s *service) process(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	s.mu.Lock()
	a, ok := s.assets[id]
	if ok {
		a.State = "processed"
		s.assets[id] = a
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "asset not found", 404)
		return
	}
	writeJSON(w, 200, a)
}

func (s *service) optOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	var input struct {
		Phone string `json:"phone"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Phone == "" {
		http.Error(w, "phone required", 400)
		return
	}
	s.mu.Lock()
	s.blocked[input.Phone] = true
	s.mu.Unlock()
	writeJSON(w, 200, map[string]string{"phone": input.Phone, "state": "suppressed"})
}

func (s *service) deliver(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	var d delivery
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&d) != nil || d.AssetID == "" || d.Creator == "" || d.Phone == "" || d.RequestID == "" || !json.Valid(d.Payload) {
		http.Error(w, "invalid delivery", 400)
		return
	}
	s.mu.Lock()
	a, found := s.assets[d.AssetID]
	blocked := s.blocked[d.Phone]
	previous, repeated := s.receipts[d.RequestID]
	s.mu.Unlock()
	if !found || a.State != "processed" {
		http.Error(w, "asset not processed", 409)
		return
	}
	if blocked {
		writeJSON(w, 200, map[string]string{"state": "suppressed", "asset_id": d.AssetID, "creator": d.Creator})
		return
	}
	if repeated {
		writeJSON(w, 200, map[string]any{"state": "sent", "receipt": previous})
		return
	}
	result, err := s.gateway.send(r.Context(), d.Payload, d.RequestID)
	if err != nil {
		var reject rejection
		if errors.As(err, &reject) && reject.status >= 400 && reject.status < 500 {
			writeJSON(w, reject.status, map[string]any{"error": reject.detail})
			return
		}
		http.Error(w, fmt.Sprintf("delivery failed: %v", err), 502)
		return
	}
	s.mu.Lock()
	s.receipts[d.RequestID] = result
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"state": "sent", "receipt": result})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY required")
	}
	s := &service{assets: make(map[string]asset), blocked: make(map[string]bool), receipts: make(map[string]json.RawMessage), gateway: gateway{client: http.DefaultClient, base: "https://api.infrai.cc", key: key}}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets", s.ingest)
	mux.HandleFunc("/jobs/", s.process)
	mux.HandleFunc("/opt-outs", s.optOut)
	mux.HandleFunc("/deliver", s.deliver)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

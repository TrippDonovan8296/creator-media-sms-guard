package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliveryDecision(t *testing.T) {
	for _, tc := range []struct {
		name   string
		optOut bool
		calls  int
		state  string
	}{
		{"allowed", false, 1, "sent"},
		{"suppressed", true, 0, "suppressed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/v1/sms/batch/send" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected gateway request: %v", r)
				}
				var params map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
					t.Fatal(err)
				}
				if string(params["messages"]) != "[]" || string(params["idempotency_key"]) != `"request-1"` || len(params) != 2 {
					t.Errorf("unexpected gateway params: %s", params)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"data":{"accepted":true},"metadata":{}}`))
			}))
			defer upstream.Close()
			s := &service{assets: map[string]asset{"a": {ID: "a", State: "processed"}}, blocked: map[string]bool{}, receipts: map[string]json.RawMessage{}, gateway: gateway{client: upstream.Client(), base: upstream.URL, key: "test-key"}}
			if tc.optOut {
				s.blocked["+15550123"] = true
			}
			r := httptest.NewRequest(http.MethodPost, "/deliver", strings.NewReader(`{"asset_id":"a","creator":"ada","phone":"+15550123","request_id":"request-1","sms_payload":{"messages":[]}}`))
			w := httptest.NewRecorder()
			s.deliver(w, r)
			if w.Code != 200 || calls != tc.calls || !strings.Contains(w.Body.String(), `"state":"`+tc.state+`"`) {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

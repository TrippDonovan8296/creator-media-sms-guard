package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type gateway struct {
	client *http.Client
	base   string
	key    string
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (g gateway) send(ctx context.Context, payload json.RawMessage, id string) (json.RawMessage, error) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, fmt.Errorf("decode SMS batch params: %w", err)
	}
	idempotencyKey, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	params["idempotency_key"] = idempotencyKey
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode SMS batch params: %w", err)
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/v1/sms/batch/send", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+g.key)
		req.Header.Set("Content-Type", "application/json")
		res, err := g.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		var reply envelope
		if err := json.Unmarshal(body, &reply); err != nil {
			return nil, fmt.Errorf("decode gateway reply: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if !reply.OK {
			return nil, rejection{status: res.StatusCode, detail: reply.Error}
		}
		if res.StatusCode >= 500 {
			return nil, errors.New("gateway request failed")
		}
		return reply.Data, nil
	}
	return nil, errors.New("retry budget exhausted")
}

type rejection struct {
	status int
	detail json.RawMessage
}

func (r rejection) Error() string { return string(r.detail) }

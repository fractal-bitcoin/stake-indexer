package stateapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	auth    string
	http    *http.Client
}

type HeightState struct {
	BlockHash string `json:"blockHash"`
	StateHash string `json:"stateHash"`
}

type queryFip101Envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Detail []HeightState `json:"detail"`
	} `json:"data"`
}

var defaultClient *Client

func Init(baseURL, auth string, timeout time.Duration) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		defaultClient = nil
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	defaultClient = &Client{
		baseURL: baseURL,
		auth:    strings.TrimSpace(auth),
		http:    &http.Client{Timeout: timeout},
	}
	return nil
}

func GetHeightState(ctx context.Context, height uint64) (HeightState, error) {
	if defaultClient == nil {
		return HeightState{}, fmt.Errorf("state api client is not configured")
	}
	return defaultClient.GetHeightState(ctx, height)
}

func (c *Client) GetHeightState(ctx context.Context, height uint64) (HeightState, error) {
	url := fmt.Sprintf("%s/brc20/statehash?start=%d&end=%d", c.baseURL, height, height)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HeightState{}, fmt.Errorf("new request: %w", err)
	}
	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return HeightState{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return HeightState{}, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	var envelope queryFip101Envelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return HeightState{}, fmt.Errorf("decode state api response: %w", err)
	}
	if envelope.Code != 0 {
		msg := strings.TrimSpace(envelope.Msg)
		if msg == "" {
			msg = "state api returned non-zero code"
		}
		return HeightState{}, fmt.Errorf("%s", msg)
	}
	if len(envelope.Data.Detail) == 0 {
		return HeightState{}, fmt.Errorf("state api returned empty detail")
	}
	return envelope.Data.Detail[0], nil
}

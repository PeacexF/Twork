package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// one public channel returned by a channel search
type Channel struct {
	Username    string
	Title       string
	Subscribers int
}

// searches for public channels by keyword, or a clear error if discovery is unavailable
type Searcher interface {
	Search(ctx context.Context, query string) ([]Channel, error)
}

// backs Searcher with the TGStat channels/search API
type TGStat struct {
	token  string
	client *http.Client
}

// builds a TGStat-backed searcher, or nil if no token is configured
func NewTGStat(token string) *TGStat {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return &TGStat{token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

type tgstatResponse struct {
	Status   string `json:"status"`
	Error    string `json:"error"`
	Response struct {
		Items []struct {
			Username          string `json:"username"`
			Title             string `json:"title"`
			PeerType          string `json:"peer_type"`
			ParticipantsCount int    `json:"participants_count"`
		} `json:"items"`
	} `json:"response"`
}

// queries TGStat channels/search and maps the results to Channels
func (t *TGStat) Search(ctx context.Context, query string) ([]Channel, error) {
	q := url.Values{}
	q.Set("token", t.token)
	q.Set("q", query)
	q.Set("limit", "20")

	reqURL := "https://api.tgstat.ru/channels/search?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TGStat returned HTTP %d", resp.StatusCode)
	}

	var parsed tgstatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding TGStat response: %w", err)
	}
	if parsed.Status != "ok" {
		if parsed.Error != "" {
			return nil, fmt.Errorf("TGStat: %s", parsed.Error)
		}
		return nil, fmt.Errorf("TGStat returned a non-ok status")
	}

	var out []Channel
	for _, it := range parsed.Response.Items {
		username := strings.TrimPrefix(it.Username, "@")
		if username == "" {
			continue // can't monitor a channel with no public username
		}
		out = append(out, Channel{
			Username:    username,
			Title:       it.Title,
			Subscribers: it.ParticipantsCount,
		})
	}
	return out, nil
}

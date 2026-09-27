package practice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client posts completed practice instances to the cortisol server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

type IngestPayload struct {
	Practice           string  `json:"practice"`
	OrganizationID     string  `json:"organization_id"`
	UserID             string  `json:"user_id"`
	SessionID          string  `json:"session_id"`
	ProjectID          string  `json:"project_id"`
	StartedAt          string  `json:"started_at"`
	EndedAt            string  `json:"ended_at"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	ActiveAnswerTimeMs int64   `json:"active_answer_time_ms"`
	Attempts           int     `json:"attempts"`
	Outcome            string  `json:"outcome"`
	PointsDelta        *int    `json:"points_delta"`
	RepoName           string  `json:"repo_name"`
	RepoOrg            string  `json:"repo_org"`
	FilePath           string  `json:"file_path"`
	Module             string  `json:"module"`
	StartLine          int     `json:"start_line"`
	EndLine            int     `json:"end_line"`
}

func (c *Client) PostEvent(ctx context.Context, payload IngestPayload) (Event, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/practice-events", bytes.NewReader(body))
	if err != nil {
		return Event{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return Event{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Event{}, err
	}
	if resp.StatusCode != http.StatusCreated {
		return Event{}, fmt.Errorf("practice ingest failed: %s", bytes.TrimSpace(data))
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, err
	}
	return event, nil
}

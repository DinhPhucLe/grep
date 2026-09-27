package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var errSessionInvalid = errors.New("session expired; sign in on the dashboard")

var authHTTPClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func validateSession(ctx context.Context, creds sessionCredentials) (sessionCredentials, error) {
	if strings.TrimRight(creds.ServerURL, "/") != cortisolServerURL() {
		return sessionCredentials{}, errors.New("saved session belongs to another server; sign in again")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cortisolServerURL()+"/api/v1/auth/me", nil)
	if err != nil {
		return sessionCredentials{}, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	res, err := authHTTPClient.Do(req)
	if err != nil {
		return sessionCredentials{}, fmt.Errorf("could not verify sign-in: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return sessionCredentials{}, errSessionInvalid
	}
	if res.StatusCode != 200 {
		return sessionCredentials{}, fmt.Errorf("could not verify sign-in (HTTP %d)", res.StatusCode)
	}
	var session sessionAPIResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&session); err != nil {
		return sessionCredentials{}, err
	}
	if session.User.ID == "" || session.Organization.ID == "" {
		return sessionCredentials{}, errors.New("incomplete session response")
	}
	session.Token = creds.Token
	return credentialsFromSession(session), nil
}

func credentialsFromSession(s sessionAPIResponse) sessionCredentials {
	return sessionCredentials{Token: s.Token, ExpiresAt: parseSessionExpiry(s.ExpiresAt),
		UserID: s.User.ID, Name: s.User.Name, GitHubLogin: s.User.GitHubLogin,
		OrgID: s.Organization.ID, OrgName: s.Organization.Name, ServerURL: cortisolServerURL()}
}

func cortisolServerURL() string {
	if u := strings.TrimSpace(os.Getenv("CORTISOL_SERVER_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:8080"
}

type sessionAPIResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
	User      struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		GitHubLogin string `json:"githubLogin"`
	} `json:"user"`
	Organization struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"organization"`
}

type apiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func logoutSession(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cortisolServerURL()+"/api/v1/auth/logout", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := authHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("logout failed (HTTP %d); please try again", res.StatusCode)
	}
	return nil
}

func postJSON(ctx context.Context, url string, body any, token string, dest any) error {
	_, err := postJSONStatus(ctx, url, body, token, dest)
	return err
}

func postJSONStatus(ctx context.Context, url string, body any, token string, dest any) (string, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := authHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode == http.StatusAccepted || res.StatusCode >= 300 {
		var envelope apiErrorBody
		_ = json.Unmarshal(raw, &envelope)
		code := envelope.Error.Code
		if code == "" {
			code = fmt.Sprintf("http_%d", res.StatusCode)
		}
		if res.StatusCode == http.StatusAccepted {
			return code, nil
		}
		msg := envelope.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return code, fmt.Errorf("%s: %s", code, msg)
	}
	if dest != nil {
		if err := json.Unmarshal(raw, dest); err != nil {
			return "", err
		}
	}
	return "", nil
}

type knowledgeCreatedEvent struct {
	Type string `json:"type"`
	Item struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		Authors []struct {
			UserID string `json:"userId"`
			Name   string `json:"name"`
		} `json:"authors"`
	} `json:"item"`
}

func listenKnowledgeEvents(ctx context.Context, token string, onEvent func(knowledgeCreatedEvent)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cortisolServerURL()+"/api/v1/knowledge/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
		return fmt.Errorf("sse %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line == "" && len(dataLines) > 0 {
			payload := strings.Join(dataLines, "\n")
			dataLines = nil
			var evt knowledgeCreatedEvent
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				continue
			}
			if evt.Type == "knowledge.created" {
				onEvent(evt)
			}
		}
	}
	return scanner.Err()
}

func parseSessionExpiry(raw string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, raw)
	return t
}

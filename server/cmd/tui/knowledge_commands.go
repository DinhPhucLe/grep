package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	knowledgeSearchK = 5
)

type knowledgeSearchDoneMsg struct {
	hits   []knowledgeHit
	failed bool
	err    string
}

type knowledgePostDoneMsg struct {
	err string
}

func parseSearchLearning(line string) (query string, ok bool) {
	trimmed := strings.TrimSpace(line)
	rest, kind := splitLearningCommand(trimmed)
	if kind != "search" {
		return "", false
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}

func parseSendLearning(line string) (topics []string, content string, ok bool) {
	trimmed := strings.TrimSpace(line)
	rest, kind := splitLearningCommand(trimmed)
	if kind != "send" {
		return nil, "", false
	}
	if rest == "" {
		return nil, "", false
	}
	left, right, found := strings.Cut(rest, "|")
	if !found {
		return nil, "", false
	}
	content = strings.TrimSpace(right)
	if content == "" {
		return nil, "", false
	}
	for _, part := range strings.Split(left, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		return nil, "", false
	}
	return topics, content, true
}

// splitLearningCommand recognizes /search-learning and /send-learning with
// optional underscores and any casing. Returns the remainder after the command.
func splitLearningCommand(trimmed string) (rest, kind string) {
	lower := strings.ToLower(strings.ReplaceAll(trimmed, "_", "-"))
	for _, spec := range []struct {
		prefix, kind string
	}{
		{"/search-learning", "search"},
		{"/send-learning", "send"},
	} {
		if lower == spec.prefix {
			return "", spec.kind
		}
		if !strings.HasPrefix(lower, spec.prefix+" ") {
			continue
		}
		idx := strings.IndexByte(trimmed, ' ')
		if idx < 0 {
			return "", spec.kind
		}
		cmdPart := strings.ToLower(strings.ReplaceAll(trimmed[:idx], "_", "-"))
		if cmdPart != spec.prefix {
			continue
		}
		return strings.TrimSpace(trimmed[idx+1:]), spec.kind
	}
	switch {
	case strings.HasPrefix(lower, "/search-learn"):
		return "", "search-hint"
	case strings.HasPrefix(lower, "/send-learn"):
		return "", "send-hint"
	default:
		return "", ""
	}
}

// ensureAuth returns true when a session token is available. It hydrates m.auth
// from ~/.cortisol/credentials when in-memory auth is missing (e.g. learning
// commands typed before loadAuthOnStart finishes).
func (m *model) ensureAuth() bool {
	if m.auth != nil && strings.TrimSpace(m.auth.Token) != "" {
		return true
	}
	creds, err := loadCredentials()
	if err != nil || strings.TrimSpace(creds.Token) == "" {
		return false
	}
	m.auth = &authIdentity{
		Token:       creds.Token,
		Name:        creds.Name,
		GitHubLogin: creds.GitHubLogin,
		OrgName:     creds.OrgName,
		OrgID:       creds.OrgID,
	}
	return true
}

func (m *model) requireAuthNotice() bool {
	if m.ensureAuth() {
		return false
	}
	m.clipboardNotice = "Run /login first"
	return true
}

func (m *model) handleLearningCommand(trimmed string) (bool, tea.Cmd) {
	_, kind := splitLearningCommand(trimmed)
	switch kind {
	case "search":
		if m.requireAuthNotice() {
			return true, nil
		}
		m.clearLoginPrompt()
		query, ok := parseSearchLearning(trimmed)
		if !ok {
			m.clipboardNotice = "Usage: /search-learning <query>"
			return true, nil
		}
		m.clipboardNotice = "Searching knowledge…"
		m.status = "Searching knowledge…"
		m.beginKnowledgeSearch()
		return true, m.runKnowledgeSearch(query)
	case "send":
		if m.requireAuthNotice() {
			return true, nil
		}
		m.clearLoginPrompt()
		topics, content, ok := parseSendLearning(trimmed)
		if !ok {
			m.clipboardNotice = "Usage: /send-learning topic1,topic2 | content"
			return true, nil
		}
		m.clipboardNotice = "Sending learning…"
		return true, m.runKnowledgePost(topics, content)
	case "search-hint":
		m.clipboardNotice = "Usage: /search-learning <query>"
		return true, nil
	case "send-hint":
		m.clipboardNotice = "Usage: /send-learning topic1,topic2 | content"
		return true, nil
	default:
		return false, nil
	}
}

func (m *model) runKnowledgeSearch(query string) tea.Cmd {
	token := m.auth.Token
	orgID := ""
	if m.auth != nil {
		orgID = m.auth.OrgID
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		u, err := url.Parse(cortisolServerURL() + "/api/v1/knowledge")
		if err != nil {
			return knowledgeSearchDoneMsg{failed: true, err: err.Error()}
		}
		q := u.Query()
		q.Set("query", query)
		q.Set("k", fmt.Sprintf("%d", knowledgeSearchK))
		if orgID != "" {
			q.Set("organizationId", orgID)
		}
		u.RawQuery = q.Encode()
		var raw json.RawMessage
		if err := getJSON(ctx, u.String(), token, &raw); err != nil {
			return knowledgeSearchDoneMsg{failed: true, err: err.Error()}
		}
		hits, ok := parseKnowledgeSearchPayload(raw)
		if !ok {
			return knowledgeSearchDoneMsg{failed: true, err: "invalid search response"}
		}
		return knowledgeSearchDoneMsg{hits: hits}
	}
}

func (m *model) runKnowledgePost(topics []string, content string) tea.Cmd {
	token := m.auth.Token
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := postJSON(ctx, cortisolServerURL()+"/api/v1/knowledge", map[string]any{
			"content": content,
			"topics":  topics,
		}, token, nil)
		if err != nil {
			return knowledgePostDoneMsg{err: err.Error()}
		}
		return knowledgePostDoneMsg{}
	}
}

func (m *model) handleKnowledgeCommandMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch v := msg.(type) {
	case knowledgeSearchDoneMsg:
		if v.err != "" {
			m.clipboardNotice = "Knowledge search failed: " + v.err
			m.status = "Knowledge search failed"
		} else if v.failed {
			m.clipboardNotice = "Knowledge search failed"
			m.status = "Knowledge search failed"
		} else if len(v.hits) == 0 {
			m.clipboardNotice = "No knowledge matches"
			m.status = "No knowledge matches"
		} else {
			m.clipboardNotice = fmt.Sprintf("Knowledge: %d match(es) — Enter to expand", len(v.hits))
			m.status = "Ready"
		}
		m.finishKnowledgeSearch(v.hits, v.failed || v.err != "")
		return nil, true
	case knowledgePostDoneMsg:
		if v.err != "" {
			m.clipboardNotice = "Send learning failed: " + v.err
		} else {
			m.clipboardNotice = "Posted learning"
		}
		return nil, true
	}
	return nil, false
}

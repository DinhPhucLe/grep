package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

// Handoffs are short-lived and process-local. Only the terminal receives the
// polling secret; the browser gets an independent approval ID in its URL fragment.
// Restarting the API safely invalidates unfinished handoffs.
type cliHandoff struct {
	secretHash string
	expires    time.Time
	token      string
}

func randomHandoffSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) handleCLIStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := randomHandoffSecret()
	if err != nil {
		writeError(w, 500, "start_failed", "Could not start terminal sign-in")
		return
	}
	secret, err := randomHandoffSecret()
	if err != nil {
		writeError(w, 500, "start_failed", "Could not start terminal sign-in")
		return
	}
	s.handoffMu.Lock()
	defer s.handoffMu.Unlock()
	if s.handoffs == nil {
		s.handoffs = make(map[string]*cliHandoff)
	}
	for key, h := range s.handoffs {
		if time.Now().After(h.expires) {
			delete(s.handoffs, key)
		}
	}
	if len(s.handoffs) >= 1000 {
		writeError(w, 429, "busy", "Please try signing in again later")
		return
	}
	s.handoffs[id] = &cliHandoff{secretHash: hashToken(secret), expires: time.Now().Add(10 * time.Minute)}
	writeJSON(w, 200, map[string]any{"id": id, "secret": secret, "expiresIn": 600})
}

func (s *Service) handleCLIApprove(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID string `json:"id"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, 400, "invalid_request", "Expected a terminal sign-in request")
		return
	}
	s.handoffMu.Lock()
	defer s.handoffMu.Unlock()
	h := s.handoffs[input.ID]
	if h == nil || time.Now().After(h.expires) {
		delete(s.handoffs, input.ID)
		writeError(w, 410, "expired", "This terminal sign-in expired. Open the dashboard again from your terminal.")
		return
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if h.token != "" && h.token != token {
		writeError(w, 409, "already_approved", "This terminal is already connected to another session")
		return
	}
	h.token = token
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleCLIPoll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, 400, "invalid_request", "Expected a terminal sign-in request")
		return
	}
	token, status := s.claimCLI(input.ID, input.Secret)
	switch status {
	case http.StatusGone:
		writeError(w, status, "expired", "Terminal sign-in expired. Open the dashboard again.")
		return
	case http.StatusUnauthorized:
		writeError(w, status, "unauthorized", "Invalid terminal sign-in secret")
		return
	case http.StatusAccepted:
		writeError(w, status, "authorization_pending", "Waiting for dashboard sign-in")
		return
	}
	p, err := s.Authenticate(r.Context(), token)
	if err != nil {
		writeError(w, 401, "unauthorized", "Dashboard session expired. Please sign in again.")
		return
	}
	writeJSON(w, 200, sessionResponse{
		Token: token, ExpiresAt: p.ExpiresAt,
		User:         User{ID: p.UserID, Name: p.Name, Mail: p.Mail, GitHubLogin: p.GitHubLogin, AvatarURL: p.AvatarURL},
		Organization: Organization{ID: p.OrganizationID, Name: p.OrgName},
	})
}

func (s *Service) claimCLI(id, secret string) (string, int) {
	s.handoffMu.Lock()
	defer s.handoffMu.Unlock()
	h := s.handoffs[id]
	if h == nil || time.Now().After(h.expires) {
		delete(s.handoffs, id)
		return "", http.StatusGone
	}
	if subtle.ConstantTimeCompare([]byte(h.secretHash), []byte(hashToken(secret))) != 1 {
		return "", http.StatusUnauthorized
	}
	token := h.token
	if token == "" {
		return "", http.StatusAccepted
	}
	// Consume before returning the bearer token; a handoff can only be claimed once.
	delete(s.handoffs, id)
	return token, http.StatusOK
}

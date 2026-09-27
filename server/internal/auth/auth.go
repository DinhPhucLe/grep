// Package auth implements GitHub device-flow login and bearer session tokens.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cortisol-server/internal/identity"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	UsersCollection    = "users"
	SessionsCollection = "auth_sessions"
	OrgsCollection     = "organizations"
	MembersCollection  = "organization_members"

	defaultSessionTTL = 30 * 24 * time.Hour
	tokenBytes        = 32
)

var (
	ErrUnauthorized     = errors.New("unauthorized")
	ErrInvalidConfig    = errors.New("invalid auth config")
	ErrDevicePending    = errors.New("authorization_pending")
	ErrDeviceSlowDown   = errors.New("slow_down")
	ErrDeviceExpired    = errors.New("expired_token")
	ErrDeviceDenied     = errors.New("access_denied")
	ErrDeviceBadCode    = errors.New("incorrect_device_code")
)

// Config is loaded from the process environment.
type Config struct {
	GitHubClientID     string
	GitHubClientSecret string
	DefaultOrgID       bson.ObjectID
	SessionTTL         time.Duration
	HTTPClient         *http.Client
}

// ConfigFromEnv reads GitHub OAuth and default-org settings.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		GitHubClientID:     strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret: strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		SessionTTL:         defaultSessionTTL,
		HTTPClient:         &http.Client{Timeout: 15 * time.Second},
	}
	if raw := strings.TrimSpace(os.Getenv("AUTH_SESSION_TTL")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Minute {
			return Config{}, fmt.Errorf("%w: AUTH_SESSION_TTL", ErrInvalidConfig)
		}
		cfg.SessionTTL = d
	}
	orgHex := strings.TrimSpace(os.Getenv("CORTISOL_DEFAULT_ORG_ID"))
	if orgHex == "" {
		// Seeded NovaPay org id from seed.DemoCastIDs().NovaPayOrgID (kindOrg=4, org=1).
		orgHex = "660100000000000004000000"
	}
	orgID, err := bson.ObjectIDFromHex(orgHex)
	if err != nil {
		return Config{}, fmt.Errorf("%w: CORTISOL_DEFAULT_ORG_ID", ErrInvalidConfig)
	}
	cfg.DefaultOrgID = orgID
	if cfg.GitHubClientID == "" {
		return Config{}, fmt.Errorf("%w: GITHUB_CLIENT_ID is required", ErrInvalidConfig)
	}
	return cfg, nil
}

// User is the authenticated account projection.
type User struct {
	ID          bson.ObjectID `json:"id" bson:"_id"`
	Name        string        `json:"name" bson:"name"`
	Mail        string        `json:"mail" bson:"mail"`
	GitHubID    string        `json:"githubId,omitempty" bson:"github_id,omitempty"`
	GitHubLogin string        `json:"githubLogin,omitempty" bson:"github_login,omitempty"`
	AvatarURL   string        `json:"avatarUrl,omitempty" bson:"avatar_url,omitempty"`
	CreatedAt   time.Time     `json:"createdAt" bson:"created_at"`
}

// Organization is a minimal org projection for auth responses.
type Organization struct {
	ID   bson.ObjectID `json:"id"`
	Name string        `json:"name"`
}

// Principal is attached to authenticated request contexts.
type Principal = identity.Principal

// WithPrincipal stores p on ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return identity.WithPrincipal(ctx, p)
}

// PrincipalFromContext returns the authenticated principal when present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	return identity.PrincipalFromContext(ctx)
}

// Service owns GitHub device flow and session persistence.
type Service struct {
	db     *mongo.Database
	cfg    Config
	client GitHubAPI
}

// GitHubAPI is the GitHub OAuth/device seam (mockable in tests).
type GitHubAPI interface {
	StartDevice(ctx context.Context, clientID string) (DeviceStart, error)
	PollToken(ctx context.Context, clientID, clientSecret, deviceCode string) (string, error)
	FetchUser(ctx context.Context, accessToken string) (GitHubUser, error)
}

// DeviceStart is returned by POST /api/v1/auth/github/device.
type DeviceStart struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

// GitHubUser is the subset of the GitHub user API we persist.
type GitHubUser struct {
	ID        int64
	Login     string
	Name      string
	Email     string
	AvatarURL string
}

// NewService wires Mongo + GitHub for auth handlers.
func NewService(database *mongo.Database, cfg Config) *Service {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Service{
		db:     database,
		cfg:    cfg,
		client: &httpGitHub{http: client},
	}
}

// WithGitHub replaces the GitHub client (tests).
func (s *Service) WithGitHub(api GitHubAPI) *Service {
	s.client = api
	return s
}

// Register mounts auth HTTP routes onto mux.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/github/device", s.handleDeviceStart)
	mux.HandleFunc("POST /api/v1/auth/github/poll", s.handleDevicePoll)
	mux.HandleFunc("GET /api/v1/auth/me", s.handleMe)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
}

// Require loads a bearer session into context or returns 401.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "bearer token required")
			return
		}
		principal, err := s.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

// Authenticate resolves a raw bearer token to a Principal.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	hash := hashToken(rawToken)
	var sess struct {
		ID        bson.ObjectID `bson:"_id"`
		UserID    bson.ObjectID `bson:"user_id"`
		ExpiresAt time.Time     `bson:"expires_at"`
	}
	err := s.db.Collection(SessionsCollection).FindOne(ctx, bson.M{
		"token_hash": hash,
		"expires_at": bson.M{"$gt": time.Now().UTC()},
	}).Decode(&sess)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	var user User
	if err := s.db.Collection(UsersCollection).FindOne(ctx, bson.M{"_id": sess.UserID}).Decode(&user); err != nil {
		return Principal{}, ErrUnauthorized
	}
	orgID, orgName, err := s.ensureDefaultMembership(ctx, user.ID)
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		UserID:         user.ID,
		Name:           user.Name,
		Mail:           user.Mail,
		GitHubLogin:    user.GitHubLogin,
		AvatarURL:      user.AvatarURL,
		OrganizationID: orgID,
		OrgName:        orgName,
		SessionID:      sess.ID,
	}, nil
}

func (s *Service) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	start, err := s.client.StartDevice(r.Context(), s.cfg.GitHubClientID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "github_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, start)
}

type pollRequest struct {
	DeviceCode string `json:"deviceCode"`
}

type sessionResponse struct {
	Token        string       `json:"token"`
	ExpiresAt    time.Time    `json:"expiresAt"`
	User         User         `json:"user"`
	Organization Organization `json:"organization"`
}

func (s *Service) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	var input pollRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected {deviceCode}")
		return
	}
	deviceCode := strings.TrimSpace(input.DeviceCode)
	if deviceCode == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "deviceCode is required")
		return
	}
	accessToken, err := s.client.PollToken(r.Context(), s.cfg.GitHubClientID, s.cfg.GitHubClientSecret, deviceCode)
	if err != nil {
		switch {
		case errors.Is(err, ErrDevicePending):
			writeError(w, http.StatusAccepted, "authorization_pending", "waiting for user authorization")
		case errors.Is(err, ErrDeviceSlowDown):
			writeError(w, http.StatusAccepted, "slow_down", "poll slower")
		case errors.Is(err, ErrDeviceExpired):
			writeError(w, http.StatusGone, "expired_token", "device code expired; restart login")
		case errors.Is(err, ErrDeviceDenied):
			writeError(w, http.StatusForbidden, "access_denied", "user denied authorization")
		default:
			writeError(w, http.StatusBadGateway, "github_error", "GitHub token exchange failed")
		}
		return
	}
	ghUser, err := s.client.FetchUser(r.Context(), accessToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "github_error", "failed to load GitHub user")
		return
	}
	user, err := s.upsertGitHubUser(r.Context(), ghUser)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "persist_failed", "failed to upsert user")
		return
	}
	orgID, orgName, err := s.ensureDefaultMembership(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "persist_failed", "failed to join default organization")
		return
	}
	rawToken, expiresAt, err := s.createSession(r.Context(), user.ID, r.Header.Get("User-Agent"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "persist_failed", "failed to create session")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		Token:     rawToken,
		ExpiresAt: expiresAt,
		User:      user,
		Organization: Organization{
			ID:   orgID,
			Name: orgName,
		},
	})
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "bearer token required")
		return
	}
	principal, err := s.Authenticate(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": User{
			ID:          principal.UserID,
			Name:        principal.Name,
			Mail:        principal.Mail,
			GitHubLogin: principal.GitHubLogin,
			AvatarURL:   principal.AvatarURL,
		},
		"organization": Organization{
			ID:   principal.OrganizationID,
			Name: principal.OrgName,
		},
	})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "bearer token required")
		return
	}
	_, _ = s.db.Collection(SessionsCollection).DeleteOne(r.Context(), bson.M{"token_hash": hashToken(token)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) upsertGitHubUser(ctx context.Context, gh GitHubUser) (User, error) {
	githubID := strconv.FormatInt(gh.ID, 10)
	name := strings.TrimSpace(gh.Name)
	if name == "" {
		name = gh.Login
	}
	mail := strings.TrimSpace(gh.Email)
	if mail == "" {
		mail = gh.Login + "@users.noreply.github.com"
	}
	now := time.Now().UTC()
	coll := s.db.Collection(UsersCollection)
	var existing User
	err := coll.FindOne(ctx, bson.M{"github_id": githubID}).Decode(&existing)
	if err == nil {
		update := bson.M{
			"$set": bson.M{
				"name":         name,
				"mail":         mail,
				"github_login": gh.Login,
				"avatar_url":   gh.AvatarURL,
			},
		}
		if _, err := coll.UpdateByID(ctx, existing.ID, update); err != nil {
			return User{}, err
		}
		existing.Name = name
		existing.Mail = mail
		existing.GitHubLogin = gh.Login
		existing.AvatarURL = gh.AvatarURL
		return existing, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, err
	}
	user := User{
		ID:          bson.NewObjectID(),
		Name:        name,
		Mail:        mail,
		GitHubID:    githubID,
		GitHubLogin: gh.Login,
		AvatarURL:   gh.AvatarURL,
		CreatedAt:   now,
	}
	if _, err := coll.InsertOne(ctx, user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) ensureDefaultMembership(ctx context.Context, userID bson.ObjectID) (bson.ObjectID, string, error) {
	orgID := s.cfg.DefaultOrgID
	var org struct {
		Name string `bson:"name"`
	}
	err := s.db.Collection(OrgsCollection).FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return bson.ObjectID{}, "", fmt.Errorf("default organization %s not found; run seed", orgID.Hex())
		}
		return bson.ObjectID{}, "", err
	}
	members := s.db.Collection(MembersCollection)
	err = members.FindOne(ctx, bson.M{"organization_id": orgID, "user_id": userID}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		_, err = members.InsertOne(ctx, bson.M{
			"_id":             bson.NewObjectID(),
			"organization_id": orgID,
			"user_id":         userID,
			"role":            "member",
			"joined_at":       time.Now().UTC(),
		})
		if err != nil {
			// Unique race: another login joined concurrently.
			if !mongo.IsDuplicateKeyError(err) {
				return bson.ObjectID{}, "", err
			}
		}
	} else if err != nil {
		return bson.ObjectID{}, "", err
	}
	return orgID, org.Name, nil
}

func (s *Service) createSession(ctx context.Context, userID bson.ObjectID, userAgent string) (string, time.Time, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	now := time.Now().UTC()
	expires := now.Add(s.cfg.SessionTTL)
	doc := bson.M{
		"_id":        bson.NewObjectID(),
		"user_id":    userID,
		"token_hash": hashToken(token),
		"expires_at": expires,
		"created_at": now,
	}
	if ua := strings.TrimSpace(userAgent); ua != "" {
		if len(ua) > 512 {
			ua = ua[:512]
		}
		doc["user_agent"] = ua
	}
	if _, err := s.db.Collection(SessionsCollection).InsertOne(ctx, doc); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func decodeJSON(r *http.Request, dst any) error {
	ct := strings.TrimSpace(r.Header.Get("Content-Type"))
	if media, _, _ := strings.Cut(ct, ";"); strings.TrimSpace(media) != "application/json" {
		return errors.New("content-type")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<16)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("trailing")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

type httpGitHub struct {
	http *http.Client
}

func (g *httpGitHub) StartDevice(ctx context.Context, clientID string) (DeviceStart, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/device/code", strings.NewReader(form.Encode()))
	if err != nil {
		return DeviceStart{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := g.http.Do(req)
	if err != nil {
		return DeviceStart{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return DeviceStart{}, fmt.Errorf("github device start: %s", strings.TrimSpace(string(body)))
	}
	var raw struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return DeviceStart{}, err
	}
	if raw.Error != "" {
		return DeviceStart{}, fmt.Errorf("%s: %s", raw.Error, raw.ErrorDesc)
	}
	if raw.Interval <= 0 {
		raw.Interval = 5
	}
	return DeviceStart{
		DeviceCode:      raw.DeviceCode,
		UserCode:        raw.UserCode,
		VerificationURI: raw.VerificationURI,
		ExpiresIn:       raw.ExpiresIn,
		Interval:        raw.Interval,
	}, nil
}

func (g *httpGitHub) PollToken(ctx context.Context, clientID, clientSecret, deviceCode string) (string, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var raw struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", err
	}
	switch raw.Error {
	case "":
		if raw.AccessToken == "" {
			return "", errors.New("empty access_token")
		}
		return raw.AccessToken, nil
	case "authorization_pending":
		return "", ErrDevicePending
	case "slow_down":
		return "", ErrDeviceSlowDown
	case "expired_token":
		return "", ErrDeviceExpired
	case "access_denied":
		return "", ErrDeviceDenied
	case "incorrect_device_code":
		return "", ErrDeviceBadCode
	default:
		return "", fmt.Errorf("%s: %s", raw.Error, raw.ErrorDesc)
	}
}

func (g *httpGitHub) FetchUser(ctx context.Context, accessToken string) (GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return GitHubUser{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := g.http.Do(req)
	if err != nil {
		return GitHubUser{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return GitHubUser{}, fmt.Errorf("github user: %s", strings.TrimSpace(string(body)))
	}
	var raw struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return GitHubUser{}, err
	}
	if raw.ID == 0 || raw.Login == "" {
		return GitHubUser{}, errors.New("incomplete github user")
	}
	return GitHubUser{
		ID:        raw.ID,
		Login:     raw.Login,
		Name:      raw.Name,
		Email:     raw.Email,
		AvatarURL: raw.AvatarURL,
	}, nil
}

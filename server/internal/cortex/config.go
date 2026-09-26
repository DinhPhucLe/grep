package cortex

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	AccountURL string
	Token      string
	Model      string
	Timeout    time.Duration
}

func ConfigFromEnv() (Config, error) {
	c := Config{AccountURL: strings.TrimRight(os.Getenv("SNOWFLAKE_ACCOUNT_URL"), "/"), Token: os.Getenv("SNOWFLAKE_PAT"), Model: os.Getenv("SNOWFLAKE_MODEL"), Timeout: 60 * time.Second}
	if value := os.Getenv("EVALUATION_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 || duration > 5*time.Minute {
			return Config{}, fmt.Errorf("EVALUATION_TIMEOUT must be a positive duration up to 5m")
		}
		c.Timeout = duration
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	u, err := url.Parse(c.AccountURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("SNOWFLAKE_ACCOUNT_URL must be an HTTPS account URL without credentials, path, query, or fragment")
	}
	if strings.TrimSpace(c.Token) == "" || strings.ContainsAny(c.Token, "\r\n") {
		return fmt.Errorf("SNOWFLAKE_PAT is required and must not contain newlines")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("SNOWFLAKE_MODEL is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("Cortex timeout must be positive")
	}
	return nil
}

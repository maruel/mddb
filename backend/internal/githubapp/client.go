// Manages GitHub App JWT generation and installation token caching.

package githubapp

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Client manages GitHub App authentication.
type Client struct {
	appID      int64
	privateKey *rsa.PrivateKey
	httpClient *http.Client

	mu     sync.Mutex
	tokens map[int64]cachedToken // installationID -> cached token
}

type cachedToken struct {
	Token     string
	ExpiresAt time.Time
}

// Repo represents a GitHub repository returned by the installation API.
type Repo struct {
	FullName string `json:"full_name"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
	CloneURL string `json:"clone_url"`
}

// NewClient creates a new GitHub App client.
func NewClient(appID int64, privateKey *rsa.PrivateKey) *Client {
	return &Client{
		appID:      appID,
		privateKey: privateKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     make(map[int64]cachedToken),
	}
}

// GenerateJWT creates a signed JWT for GitHub App authentication.
// The JWT is valid for 10 minutes per GitHub's requirements.
func (c *Client) GenerateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // 60s clock drift
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    strconv.FormatInt(c.appID, 10),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(c.privateKey)
}

// GetInstallationToken returns a valid installation access token, using cache when possible.
func (c *Client) GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	c.mu.Lock()
	if cached, ok := c.tokens[installationID]; ok {
		// Use cached token if it expires more than 5 minutes from now.
		if time.Until(cached.ExpiresAt) > 5*time.Minute {
			c.mu.Unlock()
			return cached.Token, cached.ExpiresAt, nil
		}
	}
	c.mu.Unlock()

	jwtToken, err := c.GenerateJWT()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request installation token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode token response: %w", err)
	}

	c.mu.Lock()
	c.tokens[installationID] = cachedToken{Token: result.Token, ExpiresAt: result.ExpiresAt}
	c.mu.Unlock()

	return result.Token, result.ExpiresAt, nil
}

// ListInstallationRepos lists repositories accessible to an installation.
func (c *Client) ListInstallationRepos(ctx context.Context, installationID int64) ([]Repo, error) {
	token, _, err := c.GetInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/installation/repositories?per_page=100", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Repositories []struct {
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
			Name     string `json:"name"`
			Private  bool   `json:"private"`
			HTMLURL  string `json:"html_url"`
			CloneURL string `json:"clone_url"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode repos response: %w", err)
	}

	repos := make([]Repo, len(result.Repositories))
	for i, r := range result.Repositories {
		repos[i] = Repo{
			FullName: r.FullName,
			Owner:    r.Owner.Login,
			Name:     r.Name,
			Private:  r.Private,
			HTMLURL:  r.HTMLURL,
			CloneURL: r.CloneURL,
		}
	}
	return repos, nil
}

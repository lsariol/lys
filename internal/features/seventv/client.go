// Package seventv is Lys's client for 7TV's v4 GraphQL API: it reads a
// channel's 7TV account, sets and editors, and edits emotes as Lys's own 7TV
// account. It never deletes anything on 7TV.
package seventv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lsariol/coveclient"
)

// Endpoint is 7TV's GraphQL API, the one its website uses.
const Endpoint = "https://api.7tv.app/v4/gql"

// TokenKey is the Cove key that holds the session of Lys's 7TV account.
const TokenKey = "LYS_SEVENTV_TOKEN"

// reloadEvery is the least time between reading the session from Cove again
// after 7TV refused it or it was missing.
const reloadEvery = 2 * time.Minute

// maxBody bounds an answer; a full set of 1000 emotes is far smaller.
const maxBody = 8 << 20

var (
	// ErrNoToken means Cove has no session for Lys's 7TV account.
	ErrNoToken = errors.New("7TV: " + TokenKey + " isn't in Cove: log into 7tv.app as Lys's bot account and store the session there (cove shell: create " + TokenKey + ")")
	// ErrSession means 7TV refused the session: it expired or was logged out.
	ErrSession = errors.New("7TV: the session was refused (expired or logged out): log into 7tv.app as Lys's bot account and store the new session in Cove (cove shell: update " + TokenKey + ")")
	// ErrRateLimited means 7TV asked Lys to slow down.
	ErrRateLimited = errors.New("7TV: rate limited, try again in a moment")
)

// APIError is an error 7TV answered a request with.
type APIError struct {
	Op      string // the operation, such as "add emote"
	Code    string // 7TV's code, such as LACKING_PRIVILEGES or BAD_REQUEST
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("7TV: %s: %s (%s)", e.Op, e.Message, e.Code)
}

// Client calls 7TV. Reads are anonymous; changes use Lys's 7TV session,
// read from Cove when first needed and again after 7TV refuses it.
type Client struct {
	http     *http.Client
	endpoint string
	load     func(ctx context.Context) (string, error)

	mu        sync.Mutex
	token     string
	loadErr   error
	loadedAt  time.Time
	refusedAt time.Time // when a refusal last made it read Cove again
	self      *Account  // Lys's own account, for the current token
}

// New returns a client for endpoint; load reads the session.
func New(hc *http.Client, endpoint string, load func(ctx context.Context) (string, error)) *Client {
	return &Client{http: hc, endpoint: endpoint, load: load}
}

// FromCove reads the session from Cove's TokenKey.
func FromCove(cove *coveclient.Client) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		token, err := cove.GetSecretContext(ctx, TokenKey)
		switch {
		case errors.Is(err, coveclient.ErrNotFound):
			return "", ErrNoToken
		case err != nil:
			return "", fmt.Errorf("Cove: read %s: %w", TokenKey, err)
		case strings.TrimSpace(token) == "":
			return "", ErrNoToken
		}
		return strings.TrimSpace(token), nil
	}
}

// session is the current session, read from Cove when there's none yet.
func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" && (c.loadedAt.IsZero() || time.Since(c.loadedAt) >= reloadEvery) {
		c.reloadLocked(ctx)
	}
	if c.token == "" {
		return "", c.loadErr
	}
	return c.token, nil
}

// Reload reads the session from Cove again, so a new one takes effect
// without a restart. A failed read keeps the session Lys has.
func (c *Client) Reload(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadLocked(ctx)
	return c.loadErr
}

func (c *Client) reloadLocked(ctx context.Context) {
	c.loadedAt = time.Now()
	token, err := c.load(ctx)
	if err == nil && token == "" {
		err = ErrNoToken
	}
	c.loadErr = err
	if err != nil {
		if errors.Is(err, ErrNoToken) {
			c.token, c.self = "", nil
		}
		return
	}
	if token != c.token {
		c.token, c.self = token, nil
	}
}

// refused handles 7TV refusing token: it reads Cove again, at most every
// reloadEvery, and reports whether there's a different session to try.
func (c *Client) refused(ctx context.Context, token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != token {
		return c.token != ""
	}
	if !c.refusedAt.IsZero() && time.Since(c.refusedAt) < reloadEvery {
		return false
	}
	c.refusedAt = time.Now()
	c.reloadLocked(ctx)
	return c.token != "" && c.token != token
}

// Expiry is when the current session expires, from its exp claim; false if
// there's no session or it doesn't say.
func (c *Client) Expiry(ctx context.Context) (time.Time, bool) {
	token, err := c.session(ctx)
	if err != nil {
		return time.Time{}, false
	}
	return expiry(token)
}

// expiry reads a JWT's exp claim without checking its signature.
func expiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// request is a GraphQL request body.
type request struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
}

// answer is a GraphQL response body.
type answer struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string `json:"message"`
		Extensions struct {
			Code   string `json:"code"`
			Status int    `json:"status"`
		} `json:"extensions"`
	} `json:"errors"`
}

// read runs an anonymous query.
func (c *Client) read(ctx context.Context, op, name, query string, vars map[string]any, out any) error {
	_, err := c.send(ctx, op, request{name, query, vars}, "", out)
	return err
}

// authed runs a document with Lys's session; after a refusal it tries once
// more if Cove has a newer session, since 7TV made no change.
func (c *Client) authed(ctx context.Context, op, name, query string, vars map[string]any, out any) error {
	token, err := c.session(ctx)
	if err != nil {
		return err
	}
	unauthorized, err := c.send(ctx, op, request{name, query, vars}, token, out)
	if !unauthorized {
		return err
	}
	if !c.refused(ctx, token) {
		return ErrSession
	}
	if token, err = c.session(ctx); err != nil {
		return err
	}
	if unauthorized, err = c.send(ctx, op, request{name, query, vars}, token, out); unauthorized {
		return ErrSession
	}
	return err
}

// send posts one request and decodes data into out. unauthorized reports
// that 7TV refused the session.
func (c *Client) send(ctx context.Context, op string, body request, token string, out any) (unauthorized bool, err error) {
	b, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("7TV: %s: %w", op, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return false, fmt.Errorf("7TV: %s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("7TV: %s: %w", op, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return false, fmt.Errorf("7TV: %s: %w", op, err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized && token != "":
		return true, ErrSession
	case resp.StatusCode == http.StatusTooManyRequests:
		return false, ErrRateLimited
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("7TV: %s: status %d", op, resp.StatusCode)
	}

	var a answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return false, fmt.Errorf("7TV: %s: the answer isn't GraphQL (has the API changed?): %w", op, err)
	}
	if len(a.Errors) > 0 {
		e := a.Errors[0]
		switch {
		case e.Extensions.Code == "LOGIN_REQUIRED" && token != "":
			return true, ErrSession
		case e.Extensions.Code == "RATE_LIMIT_EXCEEDED":
			return false, ErrRateLimited
		}
		code := e.Extensions.Code
		if code == "" {
			code = "no code"
		}
		return false, &APIError{Op: op, Code: code, Message: e.Message}
	}
	if len(a.Data) == 0 || string(a.Data) == "null" {
		return false, fmt.Errorf("7TV: %s: the answer has no data (has the API changed?)", op)
	}
	if err := json.Unmarshal(a.Data, out); err != nil {
		return false, fmt.Errorf("7TV: %s: the answer doesn't have the expected shape (has the API changed?): %w", op, err)
	}
	return false, nil
}

// WarnBefore is how long before the session expires Lys starts warning.
const WarnBefore = 7 * 24 * time.Hour

// Renewal says the session expires (or expired) at exp and how to renew it.
func Renewal(exp, now time.Time) string {
	when := "expires"
	if !exp.After(now) {
		when = "expired"
	}
	return fmt.Sprintf("7TV session %s %s: log into 7tv.app as Lys's bot account and store the new session in Cove (cove shell: update %s)", when, exp.Format("2006-01-02 15:04"), TokenKey)
}

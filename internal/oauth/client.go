package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	indigooauth "github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// Type aliases for indigo OAuth types used throughout the application.
// These provide a single point of control if the upstream API changes.
type (
	ClientSession     = indigooauth.ClientSession
	ClientSessionData = indigooauth.ClientSessionData
	AuthRequestData   = indigooauth.AuthRequestData
)

// Client wraps indigo's ClientApp and exposes the subset of OAuth operations
// needed by the server and CLI.
type Client struct {
	app    *indigooauth.ClientApp
	logger *slog.Logger
}

// NewClient creates a new OAuth Client.
// callbackURL is the URL the auth server redirects to after authorization.
// scopes are the ATProto OAuth scopes to request.
// store must implement indigooauth.ClientAuthStore.
func NewClient(callbackURL string, scopes []string, store indigooauth.ClientAuthStore) *Client {
	oauthCfg := indigooauth.NewLocalhostConfig(callbackURL, scopes)
	app := indigooauth.NewClientApp(&oauthCfg, store)

	return &Client{
		app:    app,
		logger: slog.Default(),
	}
}

// StartFlow initiates an OAuth authorization flow for the given handle/identifier.
// It returns the authorization URL the user should visit.
func (c *Client) StartFlow(ctx context.Context, identifier string) (string, error) {
	c.logger.Info("starting OAuth flow", "identifier", identifier)

	authURL, err := c.app.StartAuthFlow(ctx, identifier)
	if err != nil {
		return "", fmt.Errorf("starting auth flow: %w", err)
	}

	return authURL, nil
}

// HandleCallback completes the OAuth flow using the callback query parameters.
func (c *Client) HandleCallback(ctx context.Context, params url.Values) (*ClientSessionData, error) {
	c.logger.Info("handling OAuth callback")

	sess, err := c.app.ProcessCallback(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("processing callback: %w", err)
	}

	return sess, nil
}

// ResumeSession resumes a previously authenticated session.
func (c *Client) ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*ClientSession, error) {
	c.logger.Info("resuming session", "did", did, "session_id", sessionID)

	session, err := c.app.ResumeSession(ctx, did, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resuming session: %w", err)
	}

	return session, nil
}

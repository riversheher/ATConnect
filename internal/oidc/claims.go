package oidc

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// idTokenTTL is the lifetime of issued ID Tokens.
const idTokenTTL = 1 * time.Hour

// buildIDToken constructs and signs a JWT ID Token for the given
// authorization code data.
//
// Claims follow the OIDC Core specification (§2), with additional
// ATProto-specific claims:
//
//   - iss: this provider's issuer URL
//   - sub: the user's ATProto DID (stable, unique identifier)
//   - aud: the relying party's client_id
//   - exp: token expiry (1 hour from now)
//   - iat: issued-at timestamp
//   - nonce: echoed from the authorization request (if provided)
//   - preferred_username: the user's ATProto handle
//   - atproto_pds: the user's PDS host URL
func (p *Provider) buildIDToken(grant *codeGrant) (string, error) {
	now := time.Now()

	claims := jwt.MapClaims{
		"iss": p.issuerURL,
		"sub": grant.DID,
		"aud": grant.ClientID,
		"exp": now.Add(idTokenTTL).Unix(),
		"iat": now.Unix(),
	}

	// Only include nonce if it was provided in the authorization request.
	if grant.Nonce != "" {
		claims["nonce"] = grant.Nonce
	}

	// ATProto identity claims.
	if grant.Handle != "" {
		claims["preferred_username"] = grant.Handle
	}
	if grant.PDS != "" {
		claims["atproto_pds"] = grant.PDS
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = p.keyManager.KeyID()

	signed, err := token.SignedString(p.keyManager.SigningKey())
	if err != nil {
		return "", fmt.Errorf("signing ID token: %w", err)
	}

	return signed, nil
}

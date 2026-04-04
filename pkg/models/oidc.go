package models

import "time"

// IDTokenClaims represents the claims included in an OIDC ID Token
// issued by this provider. Claims map ATProto session data to OIDC
// standard claims.
type IDTokenClaims struct {
	// Standard OIDC claims
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"` // ATProto DID
	Audience string `json:"aud"` // Relying party client_id
	Expiry   int64  `json:"exp"`
	IssuedAt int64  `json:"iat"`
	Nonce    string `json:"nonce,omitempty"`

	// Standard OIDC optional claims
	PreferredUsername string `json:"preferred_username,omitempty"` // ATProto handle

	// Custom ATProto claims
	AtprotoPDS string `json:"atproto_pds,omitempty"` // User's PDS host URL
}

// OIDCClient represents a registered OIDC relying party (client).
type OIDCClient struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"` // Empty for public clients
	RedirectURIs []string  `json:"redirect_uris"`
	Name         string    `json:"client_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProviderMetadata represents the OpenID Connect Discovery document
// served at /.well-known/openid-configuration.
type ProviderMetadata struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	UserInfoEndpoint       string   `json:"userinfo_endpoint,omitempty"`
	JWKSURI                string   `json:"jwks_uri"`
	ScopesSupported        []string `json:"scopes_supported"`
	ResponseTypesSupported []string `json:"response_types_supported"`
	ClaimsSupported        []string `json:"claims_supported"`
	SubjectTypesSupported  []string `json:"subject_types_supported"`
	IDTokenSigningAlg      []string `json:"id_token_signing_alg_values_supported"`
}

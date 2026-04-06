package oidc

import (
	"encoding/json"
	"net/http"

	"github.com/riversheher/atconnect/pkg/models"
)

// HandleDiscovery serves the OpenID Connect Discovery document at
// GET /.well-known/openid-configuration.
//
// This endpoint is unauthenticated and cacheable. Relying parties use it
// to discover the provider's endpoints, supported scopes, and signing
// algorithms.
func (p *Provider) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	meta := models.ProviderMetadata{
		Issuer:                 p.issuerURL,
		AuthorizationEndpoint:  p.issuerURL + "/authorize",
		TokenEndpoint:          p.issuerURL + "/token",
		JWKSURI:                p.issuerURL + "/jwks",
		ScopesSupported:        []string{"openid"},
		ResponseTypesSupported: []string{"code"},
		ClaimsSupported: []string{
			"iss", "sub", "aud", "exp", "iat", "nonce",
			"preferred_username", "atproto_pds",
		},
		SubjectTypesSupported: []string{"public"},
		IDTokenSigningAlg:     []string{"ES256"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(meta)
}

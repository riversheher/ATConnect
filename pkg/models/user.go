package models

// User represents an ATProto user's identity as resolved after authentication.
type User struct {
	DID    string `json:"did"`    // Decentralized Identifier (e.g., did:plc:abc123)
	Handle string `json:"handle"` // ATProto handle (e.g., user.bsky.social)
	PDS    string `json:"pds"`    // PDS host URL (e.g., https://bsky.social)
}

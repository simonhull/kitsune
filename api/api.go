// Package api holds Kitsune's native API contract: the OpenAPI document
// (openapi.yaml, embedded as Spec) and the Go types for its schemas, shared by
// the server and by clients.
package api

import (
	_ "embed"
	"time"
)

// Spec is the OpenAPI 3.1 document that defines the API.
//
//go:embed openapi.yaml
var Spec []byte

// Media types used by the API.
const (
	JSONMediaType    = "application/json"
	ProblemMediaType = "application/problem+json"
)

// Problem is an RFC 9457 problem, the body of every error response.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitzero"`
	RequestID string `json:"requestId,omitzero"`
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Cookie asks for the session in a cookie instead of in the response body.
	Cookie bool `json:"cookie,omitzero"`
}

// User is an account as the API shows it. It has no password-hash field.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Session is the response to a successful login.
type Session struct {
	// Token is present only when the login did not ask for a cookie.
	Token     string    `json:"token,omitzero"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      User      `json:"user"`
}

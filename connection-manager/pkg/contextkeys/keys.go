// Package contextkeys provides shared context keys for gRPC request data.
// Using a dedicated type and package ensures the same key is used when setting
// and reading values across interceptors and handlers, avoiding type mismatches.
package contextkeys

// ContextKey is a custom type for context keys to avoid collisions with string keys.
type ContextKey string

// AgentIDKey is the context key for the authenticated agent ID (set by auth interceptors).
const AgentIDKey ContextKey = "agent_id"

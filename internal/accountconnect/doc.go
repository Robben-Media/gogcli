// Package accountconnect owns browser OAuth connection lifecycle for native MCP.
//
// The web UI in this package's web/ subdirectory must call Controller methods
// or mount Handler. The HTML layer must not implement OAuth, PKCE, token
// storage, or account identity binding.
package accountconnect

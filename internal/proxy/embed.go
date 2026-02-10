// Package proxy provides helpers for managing the Anthropic-to-OpenAI proxy
// that enables Claude Code to use non-Anthropic models via OpenRouter.
//
// The proxy script is embedded at build time and uploaded to sprites during
// provisioning. At dispatch time, the proxy is started before Claude Code
// and translates Anthropic Messages API requests to OpenAI Chat Completions API.
package proxy

import _ "embed"

//go:embed anthropic-proxy.mjs
var ProxyScript []byte

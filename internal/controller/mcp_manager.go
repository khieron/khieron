/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

// MCPServerConfig represents one MCP server entry in mcp.json,
// following the Claude Code .mcp.json format.
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"` // "" = stdio, "http" = streamable HTTP
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPConfig represents the full mcp.json content.
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// TransportFactory creates a transport for an MCP server config.
// Override in tests to inject in-memory transports.
type TransportFactory func(name string, cfg MCPServerConfig) (mcp.Transport, error)

// ParseMCPConfig parses the mcp.json content from a ConfigMap data value.
func ParseMCPConfig(data string) (*MCPConfig, error) {
	var config MCPConfig
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return nil, fmt.Errorf("invalid mcp.json: %w", err)
	}
	if config.MCPServers == nil {
		return nil, fmt.Errorf("mcp.json must contain a \"mcpServers\" object")
	}
	return &config, nil
}

// CreateToolsets creates MCP toolsets from a parsed config.
// Returns the toolsets and a slice of io.Closers for cleanup.
// If transportFactory is nil, the default production transports are used.
func CreateToolsets(config *MCPConfig, transportFactory TransportFactory) ([]tool.Toolset, []io.Closer, error) {
	if transportFactory == nil {
		transportFactory = defaultTransportFactory
	}

	var toolsets []tool.Toolset
	var closers []io.Closer

	for name, serverCfg := range config.MCPServers {
		transport, err := transportFactory(name, serverCfg)
		if err != nil {
			closeAll(closers)
			return nil, nil, fmt.Errorf("failed to create transport for MCP server %q: %w", name, err)
		}

		ts, err := mcptoolset.New(mcptoolset.Config{
			Transport: transport,
		})
		if err != nil {
			closeAll(closers)
			return nil, nil, fmt.Errorf("failed to create MCP toolset for server %q: %w", name, err)
		}

		toolsets = append(toolsets, ts)
		if closer, ok := transport.(io.Closer); ok {
			closers = append(closers, closer)
		}
	}

	return toolsets, closers, nil
}

func defaultTransportFactory(name string, cfg MCPServerConfig) (mcp.Transport, error) {
	switch cfg.Type {
	case "", "stdio":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio MCP server %q requires a \"command\" field", name)
		}
		return newReconnectableCommandTransport(cfg), nil
	case "http":
		if cfg.URL == "" {
			return nil, fmt.Errorf("HTTP MCP server %q requires a \"url\" field", name)
		}
		httpClient := &http.Client{}
		if len(cfg.Headers) > 0 {
			httpClient.Transport = &headerTransport{
				base:    http.DefaultTransport,
				headers: cfg.Headers,
			}
		}
		return &mcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP server type %q for server %q", cfg.Type, name)
	}
}

// reconnectableCommandTransport creates a new exec.Cmd on each Connect(),
// because exec.Cmd.Start() can only be called once. This allows the ADK's
// connectionRefresher to reconnect after a stdio server process exits.
type reconnectableCommandTransport struct {
	command string
	args    []string
	env     []string
}

func newReconnectableCommandTransport(cfg MCPServerConfig) *reconnectableCommandTransport {
	var env []string
	if len(cfg.Env) > 0 {
		env = os.Environ()
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return &reconnectableCommandTransport{
		command: cfg.Command,
		args:    cfg.Args,
		env:     env,
	}
}

func (t *reconnectableCommandTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	cmd := exec.CommandContext(ctx, t.command, t.args...)
	if t.env != nil {
		cmd.Env = t.env
	}
	transport := &mcp.CommandTransport{Command: cmd}
	return transport.Connect(ctx)
}

// headerTransport is an http.RoundTripper that injects headers into every request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}

// MCPCleanupFunc creates a cleanup function that closes all MCP closers.
func MCPCleanupFunc(closers []io.Closer) func() {
	if len(closers) == 0 {
		return nil
	}
	return func() {
		closeAll(closers)
	}
}

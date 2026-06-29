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
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/tool/mcptoolset"
)

const testBadServerName = "bad"

type WeatherInput struct {
	City string `json:"city" jsonschema:"city name"`
}

type WeatherOutput struct {
	Summary string `json:"summary" jsonschema:"weather summary"`
}

func GetWeather(_ context.Context, _ *mcp.CallToolRequest, input WeatherInput) (*mcp.CallToolResult, WeatherOutput, error) {
	return nil, WeatherOutput{
		Summary: fmt.Sprintf("Sunny in %s", input.City),
	}, nil
}

var _ = Describe("MCP Manager", func() {

	Describe("ParseMCPConfig", func() {
		It("should parse a valid stdio server config", func() {
			json := `{
				"mcpServers": {
					"test-server": {
						"command": "echo",
						"args": ["hello"],
						"env": {"FOO": "bar"}
					}
				}
			}`
			config, err := ParseMCPConfig(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.MCPServers).To(HaveLen(1))
			Expect(config.MCPServers).To(HaveKey("test-server"))

			server := config.MCPServers["test-server"]
			Expect(server.Command).To(Equal("echo"))
			Expect(server.Args).To(Equal([]string{"hello"}))
			Expect(server.Env).To(HaveKeyWithValue("FOO", "bar"))
			Expect(server.Type).To(BeEmpty())
		})

		It("should parse a valid HTTP server config", func() {
			json := `{
				"mcpServers": {
					"remote": {
						"type": "http",
						"url": "https://mcp.example.com/api",
						"headers": {"Authorization": "Bearer token123"}
					}
				}
			}`
			config, err := ParseMCPConfig(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.MCPServers).To(HaveLen(1))

			server := config.MCPServers["remote"]
			Expect(server.Type).To(Equal("http"))
			Expect(server.URL).To(Equal("https://mcp.example.com/api"))
			Expect(server.Headers).To(HaveKeyWithValue("Authorization", "Bearer token123"))
		})

		It("should parse mixed stdio and HTTP configs", func() {
			json := `{
				"mcpServers": {
					"local": {
						"command": "/usr/local/bin/server",
						"args": ["--verbose"]
					},
					"remote": {
						"type": "http",
						"url": "https://mcp.example.com"
					}
				}
			}`
			config, err := ParseMCPConfig(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.MCPServers).To(HaveLen(2))
			Expect(config.MCPServers).To(HaveKey("local"))
			Expect(config.MCPServers).To(HaveKey("remote"))
		})

		It("should return error for invalid JSON", func() {
			_, err := ParseMCPConfig(`{not json}`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid mcp.json"))
		})

		It("should return error for missing mcpServers key", func() {
			_, err := ParseMCPConfig(`{"other": "data"}`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mcpServers"))
		})

		It("should parse config with no env or headers", func() {
			json := `{
				"mcpServers": {
					"simple": {
						"command": "my-server"
					}
				}
			}`
			config, err := ParseMCPConfig(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.MCPServers["simple"].Env).To(BeNil())
			Expect(config.MCPServers["simple"].Args).To(BeNil())
		})
	})

	Describe("CreateToolsets", func() {
		It("should create toolsets using a custom transport factory with in-memory MCP server", func() {
			ctx := context.Background()

			clientTransport, serverTransport := mcp.NewInMemoryTransports()

			server := mcp.NewServer(&mcp.Implementation{Name: "test-weather", Version: "1.0.0"}, nil)
			mcp.AddTool(server, &mcp.Tool{
				Name:        "get_weather",
				Description: "Returns weather for a city",
			}, GetWeather)
			_, err := server.Connect(ctx, serverTransport, nil)
			Expect(err).NotTo(HaveOccurred())

			config := &MCPConfig{
				MCPServers: map[string]MCPServerConfig{
					"weather": {Command: "ignored-by-factory"},
				},
			}

			factory := func(name string, cfg MCPServerConfig) (mcp.Transport, error) {
				return clientTransport, nil
			}

			toolsets, closers, err := CreateToolsets(config, factory)
			Expect(err).NotTo(HaveOccurred())
			Expect(toolsets).To(HaveLen(1))
			Expect(closers).To(BeEmpty())
		})

		It("should return error for stdio server without command", func() {
			config := &MCPConfig{
				MCPServers: map[string]MCPServerConfig{
					testBadServerName: {Type: "stdio"},
				},
			}
			_, _, err := CreateToolsets(config, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires a \"command\" field"))
		})

		It("should return error for HTTP server without URL", func() {
			config := &MCPConfig{
				MCPServers: map[string]MCPServerConfig{
					testBadServerName: {Type: "http"},
				},
			}
			_, _, err := CreateToolsets(config, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires a \"url\" field"))
		})

		It("should return error for unsupported server type", func() {
			config := &MCPConfig{
				MCPServers: map[string]MCPServerConfig{
					testBadServerName: {Type: "websocket"},
				},
			}
			_, _, err := CreateToolsets(config, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported MCP server type"))
		})
	})

	Describe("In-memory MCP toolset integration", func() {
		It("should expose MCP tools through a toolset created with mcptoolset.New", func() {
			ctx := context.Background()

			clientTransport, serverTransport := mcp.NewInMemoryTransports()

			server := mcp.NewServer(&mcp.Implementation{Name: "test-tools", Version: "1.0.0"}, nil)
			mcp.AddTool(server, &mcp.Tool{
				Name:        "get_weather",
				Description: "Returns weather for a city",
			}, GetWeather)
			_, err := server.Connect(ctx, serverTransport, nil)
			Expect(err).NotTo(HaveOccurred())

			ts, err := mcptoolset.New(mcptoolset.Config{
				Transport: clientTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ts).NotTo(BeNil())
			Expect(ts.Name()).To(Equal("mcp_tool_set"))
		})
	})

	Describe("MCPCleanupFunc", func() {
		It("should return nil when no closers", func() {
			cleanup := MCPCleanupFunc(nil)
			Expect(cleanup).To(BeNil())
		})

		It("should call all closers when invoked", func() {
			called := make([]bool, 2)
			closers := []io.Closer{
				&testCloser{onClose: func() { called[0] = true }},
				&testCloser{onClose: func() { called[1] = true }},
			}
			cleanup := MCPCleanupFunc(closers)
			Expect(cleanup).NotTo(BeNil())
			cleanup()
			Expect(called[0]).To(BeTrue())
			Expect(called[1]).To(BeTrue())
		})
	})

	Describe("AgentEntry MCP lifecycle", func() {
		It("should call MCPCleanup when deregistering an agent", func() {
			cleanupCalled := false
			loop := NewAgentRunnerLoop(k8sClient, k8sClient.Scheme(), "fake")

			loop.Register("test/skill", &AgentEntry{
				MCPCleanup: func() { cleanupCalled = true },
			})
			loop.Deregister("test/skill")
			Expect(cleanupCalled).To(BeTrue())
		})

		It("should call MCPCleanup when replacing an agent", func() {
			oldCleanupCalled := false
			loop := NewAgentRunnerLoop(k8sClient, k8sClient.Scheme(), "fake")

			loop.Register("test/skill", &AgentEntry{
				MCPCleanup: func() { oldCleanupCalled = true },
				SkillDir:   "/tmp/old",
			})
			loop.Register("test/skill", &AgentEntry{
				SkillDir: "/tmp/new",
			})
			Expect(oldCleanupCalled).To(BeTrue())
		})

		It("should return MCP ConfigMap RV when set", func() {
			loop := NewAgentRunnerLoop(k8sClient, k8sClient.Scheme(), "fake")

			loop.Register("test/skill", &AgentEntry{
				MCPConfigMapRV: "v42",
			})
			rv, exists := loop.GetMCPConfigMapRV("test/skill")
			Expect(exists).To(BeTrue())
			Expect(rv).To(Equal("v42"))
		})

		It("should return empty MCP ConfigMap RV when not set", func() {
			loop := NewAgentRunnerLoop(k8sClient, k8sClient.Scheme(), "fake")

			_, exists := loop.GetMCPConfigMapRV("nonexistent/skill")
			Expect(exists).To(BeFalse())
		})
	})
})

type testCloser struct {
	onClose func()
}

func (c *testCloser) Close() error {
	c.onClose()
	return nil
}

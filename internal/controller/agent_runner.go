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
	"os"
	"strings"
	"sync"
	"time"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// AgentEntry holds a cached agent and its associated metadata.
type AgentEntry struct {
	Agent          agent.Agent
	Runner         *runner.Runner
	SessionService session.Service
	SkillDir       string // temp dir to clean up on refresh
	ConfigMapRV    string // ConfigMap resource version for change detection
	Interval       time.Duration
	CRKey          types.NamespacedName
}

// AgentRunnerLoop is a manager.Runnable that periodically executes cached agents.
type AgentRunnerLoop struct {
	client.Client

	mu     sync.RWMutex
	agents map[string]*AgentEntry // keyed by CR namespaced name string

	// notify channel signals the runner loop to re-evaluate schedules
	notify chan struct{}
}

// NewAgentRunnerLoop creates a new AgentRunnerLoop.
func NewAgentRunnerLoop(c client.Client) *AgentRunnerLoop {
	return &AgentRunnerLoop{
		Client: c,
		agents: make(map[string]*AgentEntry),
		notify: make(chan struct{}, 1),
	}
}

// Register adds or updates a cached agent entry. Called by the reconciler.
func (l *AgentRunnerLoop) Register(key string, entry *AgentEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clean up old skill dir if replacing an existing entry
	if old, exists := l.agents[key]; exists && old.SkillDir != entry.SkillDir {
		// Best effort cleanup - don't block on errors
		_ = removeSkillDir(old.SkillDir)
	}

	l.agents[key] = entry

	// Signal the runner loop to re-evaluate
	select {
	case l.notify <- struct{}{}:
	default:
	}
}

// Deregister removes a cached agent entry. Called when a CR is deleted.
func (l *AgentRunnerLoop) Deregister(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if old, exists := l.agents[key]; exists {
		_ = removeSkillDir(old.SkillDir)
		delete(l.agents, key)
	}
}

// GetConfigMapRV returns the cached ConfigMap resource version for a given CR key.
func (l *AgentRunnerLoop) GetConfigMapRV(key string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if entry, exists := l.agents[key]; exists {
		return entry.ConfigMapRV, true
	}
	return "", false
}

// Start implements manager.Runnable. It runs the periodic agent execution loop.
func (l *AgentRunnerLoop) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("agent-runner")
	log.Info("Agent runner loop started")

	// Tick every 30 seconds to check if any agent is due for execution
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Agent runner loop stopping")
			l.cleanupAll()
			return nil
		case <-ticker.C:
			l.runDueAgents(ctx)
		case <-l.notify:
			// Agent was registered/updated, re-evaluate immediately
			l.runDueAgents(ctx)
		}
	}
}

// runDueAgents checks each cached agent and runs it if enough time has elapsed.
func (l *AgentRunnerLoop) runDueAgents(ctx context.Context) {
	l.mu.RLock()
	entries := make([]*AgentEntry, 0, len(l.agents))
	for _, entry := range l.agents {
		entries = append(entries, entry)
	}
	l.mu.RUnlock()

	for _, entry := range entries {
		if err := l.runSkillAgentIfDue(ctx, entry); err != nil {
			log := logf.FromContext(ctx).WithName("agent-runner")
			log.Info("Agent run failed", "cr", entry.CRKey.String(), "error", err.Error())
		}
	}
}

// runSkillAgentIfDue checks the CR's lastAnalyzedAt and runs the agent if the interval has elapsed.
func (l *AgentRunnerLoop) runSkillAgentIfDue(ctx context.Context, entry *AgentEntry) error {
	log := logf.FromContext(ctx).WithName("agent-runner")

	// Fetch the current CR to check lastAnalyzedAt
	var skill agencyv1alpha1.Skill
	if err := l.Get(ctx, entry.CRKey, &skill); err != nil {
		return fmt.Errorf("failed to get CR %q: %v", entry.CRKey, err)
	}

	// Check if the agent is enabled
	if !skill.Spec.EnableAgent {
		log.Info("Agent disabled, skipping run", "cr", entry.CRKey.String())
		return nil
	}

	// Check if enough time has elapsed
	if skill.Status.LastAnalyzedAt != nil {
		elapsed := time.Since(skill.Status.LastAnalyzedAt.Time)
		if elapsed < entry.Interval {
			return nil // Not due yet
		}
	}

	// Verify skill directory still exists
	if _, err := os.Stat(entry.SkillDir); err != nil {
		log.Info("Skill directory missing, agent needs rebuild",
			"cr", entry.CRKey.String(), "skillDir", entry.SkillDir, "error", err.Error())
		return fmt.Errorf("skill directory %q does not exist: %v", entry.SkillDir, err)
	}
	log.Info("Running agent", "cr", entry.CRKey.String(), "skillDir", entry.SkillDir)

	// Create a new session for each run
	createResp, err := entry.SessionService.Create(ctx, &session.CreateRequest{
		AppName: "kueue-intelligence",
		UserID:  "controller",
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	userMsg := genai.NewContentFromText(
		"Investigate Kueue on this system",
		genai.RoleUser,
	)

	var responseText string
	var toolErrors []string
	var runTokens agencyv1alpha1.TokenUsage

	for event, err := range entry.Runner.Run(ctx, "controller", createResp.Session.ID(), userMsg, agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("agent run error: %v", err)
		}
		// Accumulate token usage from each LLM call
		if event.UsageMetadata != nil {
			usage := event.UsageMetadata
			runTokens.PromptTokenCount += usage.PromptTokenCount
			runTokens.CandidatesTokenCount += usage.CandidatesTokenCount
			runTokens.ThoughtsTokenCount += usage.ThoughtsTokenCount
			runTokens.ToolUsePromptTokenCount += usage.ToolUsePromptTokenCount
			runTokens.TotalTokenCount += usage.TotalTokenCount
		}
		// Inspect tool call results for errors
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part.FunctionResponse != nil {
					resp := part.FunctionResponse.Response
					exitCode, hasExitCode := resp["exit_code"]
					if hasExitCode {
						code, isFloat := exitCode.(float64)
						if isFloat && code != 0 {
							detail := fmt.Sprintf("tool %q failed (exit_code=%d): %v",
								part.FunctionResponse.Name, int(code), resp)
							toolErrors = append(toolErrors, detail)
							log.Info("Tool execution failed", "cr", entry.CRKey.String(), "detail", detail)
						}
					}
				}
			}
		}
		if event.IsFinalResponse() && event.Content != nil {
			for _, part := range event.Content.Parts {
				responseText += part.Text
			}
		}
	}

	if len(toolErrors) > 0 {
		log.Info("Tool errors during agent run", "cr", entry.CRKey.String(),
			"errors", strings.Join(toolErrors, "; "))
	}
	log.Info("Agent response", "cr", entry.CRKey.String(),
		"response", responseText,
		"promptTokens", runTokens.PromptTokenCount,
		"candidatesTokens", runTokens.CandidatesTokenCount,
		"thoughtsTokens", runTokens.ThoughtsTokenCount,
		"toolUseTokens", runTokens.ToolUsePromptTokenCount,
		"totalTokens", runTokens.TotalTokenCount)

	// Re-fetch the CR to avoid conflicts with stale resourceVersion
	if err := l.Get(ctx, entry.CRKey, &skill); err != nil {
		return fmt.Errorf("failed to re-fetch CR for status update: %v", err)
	}

	// Update status with token usage and timestamp
	now := metav1.Now()
	skill.Status.LastAnalyzedAt = &now
	skill.Status.TokensLastRun = &runTokens

	// Accumulate totals
	if skill.Status.TokensTotal == nil {
		skill.Status.TokensTotal = &agencyv1alpha1.TokensAccumulated{}
	}
	skill.Status.TokensTotal.TotalTokenCount += int64(runTokens.TotalTokenCount)
	skill.Status.TokensTotal.RunCount++

	if err := l.Status().Update(ctx, &skill); err != nil {
		return fmt.Errorf("failed to update status: %v", err)
	}

	if len(toolErrors) > 0 {
		return fmt.Errorf("agent error. %s", responseText)
	}

	return nil
}

// RunWithPrompt runs a cached agent with a custom prompt. Used for executing approved proposals.
func (l *AgentRunnerLoop) RunWithPrompt(ctx context.Context, crKey types.NamespacedName, prompt string) error {
	log := logf.FromContext(ctx).WithName("agent-runner")

	l.mu.RLock()
	entry, exists := l.agents[crKey.String()]
	l.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no cached agent for %q", crKey)
	}

	// Verify skill directory still exists
	if _, err := os.Stat(entry.SkillDir); err != nil {
		return fmt.Errorf("skill directory %q does not exist: %v", entry.SkillDir, err)
	}

	log.Info("Running agent with custom prompt", "cr", crKey.String(), "prompt", prompt)

	createResp, err := entry.SessionService.Create(ctx, &session.CreateRequest{
		AppName: "kueue-intelligence",
		UserID:  "controller",
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	userMsg := genai.NewContentFromText(prompt, genai.RoleUser)

	var responseText string
	for event, err := range entry.Runner.Run(ctx, "controller", createResp.Session.ID(), userMsg, agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("agent run error: %v", err)
		}
		if event.IsFinalResponse() && event.Content != nil {
			for _, part := range event.Content.Parts {
				responseText += part.Text
			}
		}
	}

	log.Info("Agent proposal execution response", "cr", crKey.String(), "response", responseText)
	return nil
}

// cleanupAll removes all cached skill directories on shutdown.
func (l *AgentRunnerLoop) cleanupAll() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, entry := range l.agents {
		_ = removeSkillDir(entry.SkillDir)
	}
}

// removeSkillDir removes a skill temp directory if it exists.
func removeSkillDir(dir string) error {
	if dir != "" {
		return os.RemoveAll(dir)
	}
	return nil
}

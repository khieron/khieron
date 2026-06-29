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
	"iter"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const RunRequestedAnnotation = "khieron.io/run-requested"
const KHIERON = "khieron"

// AgentEntry holds a cached agent and its associated metadata.
type AgentEntry struct {
	Agent          agent.Agent
	Runner         *runner.Runner
	SessionService session.Service
	SkillDir       string // temp dir to clean up on refresh
	ConfigMapRV    string // ConfigMap resource version for change detection
	MCPConfigMapRV string // MCP ConfigMap resource version for change detection
	MCPCleanup     func() // called on deregister/replace to clean up MCP connections
	Interval       time.Duration
	CRKey          types.NamespacedName
}

// AgentRunnerLoop is a manager.Runnable that periodically executes cached agents.
type AgentRunnerLoop struct {
	client.Client
	Scheme *runtime.Scheme

	Model      model.LLM
	modelName  string
	modelReady bool

	mu     sync.RWMutex
	agents map[string]*AgentEntry // keyed by CR namespaced name string

	// notify channel signals the runner loop to re-evaluate schedules
	notify chan struct{}
}

// NewAgentRunnerLoop creates a new AgentRunnerLoop.
func NewAgentRunnerLoop(c client.Client, scheme *runtime.Scheme, modelName string) *AgentRunnerLoop {
	return &AgentRunnerLoop{
		Client:    c,
		Scheme:    scheme,
		modelName: modelName,
		agents:    make(map[string]*AgentEntry),
		notify:    make(chan struct{}, 1),
	}
}

// ReadyzCheck returns a healthz.Checker that reports healthy only after
// the LLM model has been successfully created.
func (l *AgentRunnerLoop) ReadyzCheck() healthz.Checker {
	return func(_ *http.Request) error {
		if !l.modelReady {
			return fmt.Errorf("LLM model %q not yet initialized", l.modelName)
		}
		return nil
	}
}

// Register adds or updates a cached agent entry. Called by the reconciler.
func (l *AgentRunnerLoop) Register(key string, entry *AgentEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clean up old entry if replacing
	if old, exists := l.agents[key]; exists {
		if old.SkillDir != entry.SkillDir {
			_ = removeSkillDir(old.SkillDir)
		}
		if old.MCPCleanup != nil {
			old.MCPCleanup()
		}
	}

	l.agents[key] = entry

	// Signal the runner loop to re-evaluate
	select {
	case l.notify <- struct{}{}:
	default:
	}
}

// Notify signals the runner loop to re-evaluate schedules immediately.
func (l *AgentRunnerLoop) Notify() {
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
		if old.MCPCleanup != nil {
			old.MCPCleanup()
		}
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

// GetMCPConfigMapRV returns the cached MCP ConfigMap resource version for a given CR key.
func (l *AgentRunnerLoop) GetMCPConfigMapRV(key string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if entry, exists := l.agents[key]; exists {
		return entry.MCPConfigMapRV, true
	}
	return "", false
}

// Start implements manager.Runnable. It runs the periodic agent execution loop.
func (l *AgentRunnerLoop) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("agent-runner")

	llmModel, err := l.createModel(ctx)
	if err != nil {
		return fmt.Errorf("failed to create LLM model: %w", err)
	}
	l.Model = llmModel
	l.modelReady = true
	log.Info("Agent runner loop started", "model", l.modelName)

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

// agentRunResult holds the outcome of an agent execution.
type agentRunResult struct {
	ResponseText    string
	ToolErrors      []string
	AdvisoryCreated bool
	Tokens          agencyv1alpha1.TokenUsage
}

// isRunDue checks whether the skill agent should run now based on interval, enable flag, and force-run annotation.
func isRunDue(skill *agencyv1alpha1.Skill, interval time.Duration) bool {
	if !skill.Spec.EnableAgent {
		return false
	}
	if requested, ok := skill.Annotations[RunRequestedAnnotation]; ok {
		requestedTime, err := time.Parse(time.RFC3339, requested)
		if err == nil {
			if skill.Status.LastAnalyzedAt == nil || requestedTime.After(skill.Status.LastAnalyzedAt.Time) {
				return true
			}
		}
	}
	if skill.Status.LastAnalyzedAt != nil {
		if time.Since(skill.Status.LastAnalyzedAt.Time) < interval {
			return false
		}
	}
	return true
}

// processAgentEvents consumes the event stream from an agent run and collects the result.
func processAgentEvents(ctx context.Context, events iter.Seq2[*session.Event, error], crKey string) (*agentRunResult, error) {
	log := logf.FromContext(ctx).WithName("agent-runner")
	result := &agentRunResult{}

	for event, err := range events {
		if err != nil {
			return nil, fmt.Errorf("agent run error: %v", err)
		}
		if event.UsageMetadata != nil {
			usage := event.UsageMetadata
			result.Tokens.PromptTokenCount += usage.PromptTokenCount
			result.Tokens.CandidatesTokenCount += usage.CandidatesTokenCount
			result.Tokens.ThoughtsTokenCount += usage.ThoughtsTokenCount
			result.Tokens.ToolUsePromptTokenCount += usage.ToolUsePromptTokenCount
			result.Tokens.TotalTokenCount += usage.TotalTokenCount
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part.FunctionCall != nil && part.FunctionCall.Name == "create_advisory" {
					result.AdvisoryCreated = true
				}
				if part.FunctionResponse != nil {
					resp := part.FunctionResponse.Response
					if exitCode, ok := resp["exit_code"]; ok {
						if code, isFloat := exitCode.(float64); isFloat && code != 0 {
							detail := fmt.Sprintf("tool %q failed (exit_code=%d): %v",
								part.FunctionResponse.Name, int(code), resp)
							result.ToolErrors = append(result.ToolErrors, detail)
							log.Info("Tool execution failed", "cr", crKey, "detail", detail)
						}
					}
				}
			}
		}
		if event.IsFinalResponse() && event.Content != nil {
			for _, part := range event.Content.Parts {
				result.ResponseText += part.Text
			}
		}
	}
	return result, nil
}

// runSkillAgentIfDue checks the CR's lastAnalyzedAt and runs the agent if the interval has elapsed.
func (l *AgentRunnerLoop) runSkillAgentIfDue(ctx context.Context, entry *AgentEntry) error {
	log := logf.FromContext(ctx).WithName("agent-runner")

	var skill agencyv1alpha1.Skill
	if err := l.Get(ctx, entry.CRKey, &skill); err != nil {
		return fmt.Errorf("failed to get CR %q: %v", entry.CRKey, err)
	}

	if !isRunDue(&skill, entry.Interval) {
		return nil
	}

	if _, err := os.Stat(entry.SkillDir); err != nil {
		return fmt.Errorf("skill directory %q does not exist: %v", entry.SkillDir, err)
	}
	log.Info("Running agent", "cr", entry.CRKey.String(), "skillDir", entry.SkillDir)

	createResp, err := entry.SessionService.Create(ctx, &session.CreateRequest{
		AppName: KHIERON,
		UserID:  "controller",
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	userMsg := genai.NewContentFromText(
		fmt.Sprintf("Investigate this system. You are running in namespace %q for skill %q.",
			entry.CRKey.Namespace, entry.CRKey.Name),
		genai.RoleUser,
	)
	events := entry.Runner.Run(ctx, "controller", createResp.Session.ID(), userMsg, agent.RunConfig{})

	result, err := processAgentEvents(ctx, events, entry.CRKey.String())
	if err != nil {
		return err
	}

	if len(result.ToolErrors) > 0 {
		log.Info("Tool errors during agent run", "cr", entry.CRKey.String(),
			"errors", strings.Join(result.ToolErrors, "; "))
	}
	if len(result.ToolErrors) > 0 && !result.AdvisoryCreated {
		log.Info("Agent did not create advisory for tool errors, creating fallback advisory",
			"cr", entry.CRKey.String())
		if err := l.createFallbackAdvisory(ctx, entry.CRKey, result.ToolErrors); err != nil {
			log.Info("Failed to create fallback advisory", "cr", entry.CRKey.String(), "error", err.Error())
		}
	}
	log.Info("Agent response", "cr", entry.CRKey.String(),
		"response", result.ResponseText,
		"promptTokens", result.Tokens.PromptTokenCount,
		"candidatesTokens", result.Tokens.CandidatesTokenCount,
		"thoughtsTokens", result.Tokens.ThoughtsTokenCount,
		"toolUseTokens", result.Tokens.ToolUsePromptTokenCount,
		"totalTokens", result.Tokens.TotalTokenCount)

	if err := l.Get(ctx, entry.CRKey, &skill); err != nil {
		return fmt.Errorf("failed to re-fetch CR for status update: %v", err)
	}

	now := metav1.Now()
	skill.Status.LastAnalyzedAt = &now
	skill.Status.TokensLastRun = &result.Tokens
	if skill.Status.TokensTotal == nil {
		skill.Status.TokensTotal = &agencyv1alpha1.TokensAccumulated{}
	}
	skill.Status.TokensTotal.TotalTokenCount += int64(result.Tokens.TotalTokenCount)
	skill.Status.TokensTotal.RunCount++

	if err := l.Status().Update(ctx, &skill); err != nil {
		return fmt.Errorf("failed to update status: %v", err)
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
		AppName: KHIERON,
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

// createFallbackAdvisory creates an advisory when the agent failed to do so after tool errors.
func (l *AgentRunnerLoop) createFallbackAdvisory(ctx context.Context, crKey types.NamespacedName, toolErrors []string) error {
	var owner agencyv1alpha1.Skill
	if err := l.Get(ctx, crKey, &owner); err != nil {
		return fmt.Errorf("failed to get owner Skill: %v", err)
	}

	now := metav1.Now()
	advisory := &agencyv1alpha1.Advisory{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-tool-failed-", owner.Name),
			Namespace:    crKey.Namespace,
		},
	}

	if err := ctrl.SetControllerReference(&owner, advisory, l.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %v", err)
	}

	if err := l.Create(ctx, advisory); err != nil {
		return fmt.Errorf("failed to create Advisory: %v", err)
	}

	advisory.Status = agencyv1alpha1.AdvisoryStatus{
		Advisory:    fmt.Sprintf("Tool execution failed during %s agent run", owner.Name),
		Explanation: strings.Join(toolErrors, "; "),
		Proposal:    "Investigate the tool failures and consider disabling the agent until the issue is resolved (set spec.enableAgent to false on the Skill CR)",
		Updated:     &now,
	}
	if err := l.Status().Update(ctx, advisory); err != nil {
		return fmt.Errorf("failed to update Advisory status: %v", err)
	}

	return nil
}

// fakeModel is a no-op LLM used for e2e tests where no real API key is available.
type fakeModel struct{}

func (m *fakeModel) Name() string { return "fake" }

func (m *fakeModel) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText("No issues found.", genai.RoleModel),
			FinishReason: genai.FinishReasonStop,
			TurnComplete: true,
		}, nil)
	}
}

// createModel creates the LLM model using either Vertex AI or the Gemini API,
// depending on environment variables. If the model name is "fake", a no-op
// stub is returned instead.
func (l *AgentRunnerLoop) createModel(ctx context.Context) (model.LLM, error) {
	log := logf.FromContext(ctx)
	if l.modelName == "fake" {
		log.Info("Using fake model for testing")
		return &fakeModel{}, nil
	}
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	location := os.Getenv("GOOGLE_CLOUD_LOCATION")
	if project != "" && location != "" {
		log.Info("Creating model via Vertex AI", "model", l.modelName, "project", project, "location", location)
		return gemini.NewModel(ctx, l.modelName, &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Project:  project,
			Location: location,
		})
	}
	log.Info("Creating model via Gemini API", "model", l.modelName)
	return gemini.NewModel(ctx, l.modelName, &genai.ClientConfig{
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
}

// cleanupAll removes all cached skill directories on shutdown.
func (l *AgentRunnerLoop) cleanupAll() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, entry := range l.agents {
		_ = removeSkillDir(entry.SkillDir)
		if entry.MCPCleanup != nil {
			entry.MCPCleanup()
		}
	}
}

// removeSkillDir removes a skill temp directory if it exists.
func removeSkillDir(dir string) error {
	if dir != "" {
		return os.RemoveAll(dir)
	}
	return nil
}

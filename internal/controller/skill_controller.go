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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/genai"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
)

// ModelFactory creates a model.LLM instance for use by the agent.
type ModelFactory func(ctx context.Context) (model.LLM, error)

// NewGeminiModelFactory returns a ModelFactory that creates a Gemini model.
func NewGeminiModelFactory(modelName string) ModelFactory {
	return func(ctx context.Context) (model.LLM, error) {
		return gemini.NewModel(ctx, modelName, &genai.ClientConfig{
			APIKey:  os.Getenv("GOOGLE_API_KEY"),
			Backend: genai.BackendGeminiAPI,
		})
	}
}

// SkillReconciler reconciles a Skill object
type SkillReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	RunnerLoop   *AgentRunnerLoop
	ModelFactory ModelFactory
}

// UpdateOwnerArgs defines the input for the update_owner tool.
type UpdateOwnerArgs struct {
	EnableAgent *bool  `json:"enableagent,omitempty"`
	Interval    *int16 `json:"intervalminute,omitempty"`
}

// UpdateOwnerResult defines the output of the update_owner tool.
type UpdateOwnerResult struct {
	Updated bool   `json:"updated"`
	Message string `json:"message"`
}

// newUpdateOwnerTool creates a tool that updates the owning Skill CR's spec fields.
func newUpdateOwnerTool(k8sClient client.Client, ownerKey types.NamespacedName) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "update_owner",
		Description: "Updates the owning Skill CR's spec fields. Can set enableagent (bool) to enable/disable the agent, or intervalminute (int) to change the run interval.",
	}, func(ctx tool.Context, args UpdateOwnerArgs) (UpdateOwnerResult, error) {
		var owner agencyv1alpha1.Skill
		if err := k8sClient.Get(ctx, ownerKey, &owner); err != nil {
			return UpdateOwnerResult{}, fmt.Errorf("failed to get owner Skill: %v", err)
		}

		var changes []string
		if args.EnableAgent != nil {
			owner.Spec.EnableAgent = *args.EnableAgent
			changes = append(changes, fmt.Sprintf("enableagent=%v", *args.EnableAgent))
		}
		if args.Interval != nil {
			owner.Spec.IntervalMinute = *args.Interval
			changes = append(changes, fmt.Sprintf("intervalminute=%d", *args.Interval))
		}

		if len(changes) == 0 {
			return UpdateOwnerResult{Updated: false, Message: "no fields to update"}, nil
		}

		if err := k8sClient.Update(ctx, &owner); err != nil {
			return UpdateOwnerResult{}, fmt.Errorf("failed to update Skill: %v", err)
		}

		msg := fmt.Sprintf("updated Skill %s: %s", ownerKey.Name, strings.Join(changes, ", "))
		return UpdateOwnerResult{Updated: true, Message: msg}, nil
	})
}

// SetAdvisoryLabelsArgs defines the input for the set_advisory_labels tool.
type SetAdvisoryLabelsArgs struct {
	AdvisoryName string `json:"advisory_name"`
	JobName      string `json:"job_name"`
	JobNamespace string `json:"job_namespace"`
}

// SetAdvisoryLabelsResult defines the output of the set_advisory_labels tool.
type SetAdvisoryLabelsResult struct {
	Updated bool   `json:"updated"`
	Message string `json:"message"`
}

// newSetAdvisoryLabelsTool creates a tool that labels a Advisory with the related Job's
// name and namespace, enabling the controller to clean up advisories when the Job is deleted.
func newSetAdvisoryLabelsTool(k8sClient client.Client, advisoryNamespace string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "set_advisory_labels",
		Description: "Labels a Advisory with the related Job's name and namespace. This allows the controller to track which Job an advisory relates to, and clean up the advisory when the Job is deleted. Requires the advisory name, and the job name and namespace.",
	}, func(ctx tool.Context, args SetAdvisoryLabelsArgs) (SetAdvisoryLabelsResult, error) {
		if args.AdvisoryName == "" || args.JobName == "" || args.JobNamespace == "" {
			return SetAdvisoryLabelsResult{}, fmt.Errorf("advisory_name, job_name, and job_namespace are all required")
		}

		// Fetch the Advisory
		var advisory agencyv1alpha1.Advisory
		advisoryKey := types.NamespacedName{Name: args.AdvisoryName, Namespace: advisoryNamespace}
		if err := k8sClient.Get(ctx, advisoryKey, &advisory); err != nil {
			return SetAdvisoryLabelsResult{}, fmt.Errorf("failed to get Advisory %s/%s: %v", advisoryNamespace, args.AdvisoryName, err)
		}

		// Set labels for the related Job
		labels := advisory.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}

		// Check if the labels are already set correctly
		if labels["khieron.io/job-name"] == args.JobName &&
			labels["khieron.io.io/job-namespace"] == args.JobNamespace {
			return SetAdvisoryLabelsResult{
				Updated: false,
				Message: fmt.Sprintf("Advisory %s already labelled with Job %s/%s", args.AdvisoryName, args.JobNamespace, args.JobName),
			}, nil
		}

		labels["khieron.io/job-name"] = args.JobName
		labels["khieron.io/job-namespace"] = args.JobNamespace
		advisory.SetLabels(labels)

		if err := k8sClient.Update(ctx, &advisory); err != nil {
			return SetAdvisoryLabelsResult{}, fmt.Errorf("failed to update Advisory: %v", err)
		}

		return SetAdvisoryLabelsResult{
			Updated: true,
			Message: fmt.Sprintf("Labelled Advisory %s with Job %s/%s", args.AdvisoryName, args.JobNamespace, args.JobName),
		}, nil
	})
}

// RunScriptArgs defines the input for the run_script tool.
type RunScriptArgs struct {
	ScriptPath string   `json:"script_path"`
	Args       []string `json:"args,omitempty"`
}

// RunScriptResult defines the output of the run_script tool.
type RunScriptResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// CreateAdvisoryArgs defines the input for the create_advisory tool.
type CreateAdvisoryArgs struct {
	Name        string `json:"name"`
	Advisory    string `json:"advisory"`
	Explanation string `json:"explanation"`
	Proposal    string `json:"proposal"`
}

// CreateAdvisoryResult defines the output of the create_advisory tool.
type CreateAdvisoryResult struct {
	Created bool   `json:"created"`
	Name    string `json:"name"`
}

// newCreateAdvisoryTool creates a tool that creates Advisory CRs.
func newCreateAdvisoryTool(k8sClient client.Client, scheme *runtime.Scheme, ownerKey types.NamespacedName) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "create_advisory",
		Description: "Creates a Advisory custom resource to alert a human operator about an issue that needs attention.",
	}, func(ctx tool.Context, args CreateAdvisoryArgs) (CreateAdvisoryResult, error) {
		// Fetch the owner Skill CR for the owner reference
		var owner agencyv1alpha1.Skill
		if err := k8sClient.Get(ctx, ownerKey, &owner); err != nil {
			return CreateAdvisoryResult{}, fmt.Errorf("failed to get owner Skill: %v", err)
		}

		// Check for an existing advisory with the same text owned by this Skill
		var existingAdvisories agencyv1alpha1.AdvisoryList
		if err := k8sClient.List(ctx, &existingAdvisories,
			client.InNamespace(ownerKey.Namespace)); err != nil {
			return CreateAdvisoryResult{}, fmt.Errorf("failed to list advisories: %v", err)
		}

		for i := range existingAdvisories.Items {
			existing := &existingAdvisories.Items[i]
			if existing.Status.Advisory == args.Advisory {
				for _, ref := range existing.GetOwnerReferences() {
					if ref.UID == owner.UID {
						// Update the existing advisory
						now := metav1.Now()
						existing.Status.Explanation = args.Explanation
						existing.Status.Proposal = args.Proposal
						existing.Status.Updated = &now
						if err := k8sClient.Status().Update(ctx, existing); err != nil {
							return CreateAdvisoryResult{}, fmt.Errorf("failed to update existing advisory: %v", err)
						}
						return CreateAdvisoryResult{Created: false, Name: existing.Name}, nil
					}
				}
			}
		}

		// No existing match, create a new advisory
		now := metav1.Now()
		advisory := &agencyv1alpha1.Advisory{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-%s-", owner.Name, args.Name),
				Namespace:    ownerKey.Namespace,
			},
		}

		// Set owner reference so the advisory is cleaned up with the Skill
		if err := ctrl.SetControllerReference(&owner, advisory, scheme); err != nil {
			return CreateAdvisoryResult{}, fmt.Errorf("failed to set owner reference: %v", err)
		}

		if err := k8sClient.Create(ctx, advisory); err != nil {
			return CreateAdvisoryResult{}, fmt.Errorf("failed to create Advisory: %v", err)
		}

		// Update the status with the advisory details
		advisory.Status = agencyv1alpha1.AdvisoryStatus{
			Advisory:    args.Advisory,
			Explanation: args.Explanation,
			Proposal:    args.Proposal,
			Updated:     &now,
		}
		if err := k8sClient.Status().Update(ctx, advisory); err != nil {
			return CreateAdvisoryResult{}, fmt.Errorf("failed to update Advisory status: %v", err)
		}

		return CreateAdvisoryResult{
			Created: true,
			Name:    advisory.Name,
		}, nil
	})
}

// newRunScriptTool creates a tool that executes scripts from the skill's scripts/ directory.
func newRunScriptTool(skillDir string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "run_script",
		Description: "Executes a script from the skill's scripts/ directory and returns its output. Pass arguments to the script via the args array.",
	}, func(ctx tool.Context, args RunScriptArgs) (RunScriptResult, error) {
		log := logf.FromContext(ctx)
		cleanPath := filepath.Clean(args.ScriptPath)
		log.Info("run_script called", "requestedPath", args.ScriptPath, "cleanPath", cleanPath, "skillDir", skillDir)
		if !strings.HasPrefix(cleanPath, "scripts/") {
			return RunScriptResult{}, fmt.Errorf("script path %q must be within the scripts/ directory", args.ScriptPath)
		}

		fullPath := filepath.Join(skillDir, cleanPath)
		log.Info("run_script executing", "fullPath", fullPath)
		cmdArgs := append([]string{fullPath}, args.Args...)
		cmd := exec.CommandContext(ctx, "bash", cmdArgs...)

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				return RunScriptResult{}, fmt.Errorf("failed to run script: %w", err)
			}
		}

		return RunScriptResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	})
}

// +kubebuilder:rbac:groups=agency.khieron.io,resources=skills,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agency.khieron.io,resources=skills/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agency.khieron.io,resources=skills/finalizers,verbs=update

// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Skill object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *SkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	crKey := req.String()

	// Fetch the Skill CR
	var skillCr agencyv1alpha1.Skill
	if err := r.Get(ctx, req.NamespacedName, &skillCr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// CR was deleted, deregister the agent
			log.Info("Skill deleted, deregistering agent", "cr", crKey)
			r.RunnerLoop.Deregister(crKey)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Skill", "cr", crKey)

	// Fetch the referenced ConfigMap
	var configMap corev1.ConfigMap
	configMapKey := types.NamespacedName{
		Name:      skillCr.Spec.SkillConfigRef.Name,
		Namespace: req.Namespace,
	}
	if err := r.Get(ctx, configMapKey, &configMap); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ConfigMap %q: %v", configMapKey, err)
	}

	// Check if the cached agent is still valid (ConfigMap hasn't changed)
	if cachedRV, exists := r.RunnerLoop.GetConfigMapRV(crKey); exists && cachedRV == configMap.ResourceVersion {
		if _, hasRunRequest := skillCr.Annotations[RunRequestedAnnotation]; hasRunRequest {
			r.RunnerLoop.Notify()
		}
		log.Info("Agent already cached and up to date", "cr", crKey)
		return ctrl.Result{}, nil
	}

	log.Info("Building agent", "cr", crKey, "configMap", configMapKey.Name,
		"resourceVersion", configMap.ResourceVersion)

	// Write ConfigMap data to a temp directory for the skill toolset
	skillDir, err := os.MkdirTemp("", "khieron-skills-*")
	log.Info("Created skill directory", "skillDir", skillDir)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create temp dir for skills: %v", err)
	}
	// defer os.RemoveAll(skillDir)

	for filename, content := range configMap.Data {
		filePath := filepath.Join(skillDir, strings.ReplaceAll(filename, "___", "/"))
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create dir for %q: %v", filename, err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to write skill file %q: %v", filename, err)
		}
	}

	skillToolset, err := skilltoolset.New(ctx, skilltoolset.Config{
		Source: skill.NewFileSystemSource(os.DirFS(skillDir)),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to load skills from ConfigMap %q: %v", configMapKey.Name, err)
	}
	log.Info("Skill toolset loaded", "cr", crKey, "configMap", configMapKey.Name)

	// Create run_script tools for each skill subdirectory
	var scriptTools []tool.Tool
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to read skill dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			scriptsDir := filepath.Join(skillDir, entry.Name(), "scripts")
			if _, err := os.Stat(scriptsDir); err == nil {
				scriptTool, err := newRunScriptTool(filepath.Join(skillDir, entry.Name()))
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to create run_script tool for %q: %v", entry.Name(), err)
				}
				scriptTools = append(scriptTools, scriptTool)
				log.Info("Script tool created", "cr", crKey, "skill", entry.Name())
			}
		}
	}

	// Create the create_advisory tool
	advisoryTool, err := newCreateAdvisoryTool(r.Client, r.Scheme, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create create_advisory tool: %v", err)
	}
	scriptTools = append(scriptTools, advisoryTool)

	// Create the update_owner tool
	updateOwnerTool, err := newUpdateOwnerTool(r.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create update_owner tool: %v", err)
	}
	scriptTools = append(scriptTools, updateOwnerTool)

	// Create the set_advisory_labels tool
	setAdvisoryLabelsTool, err := newSetAdvisoryLabelsTool(r.Client, req.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create set_advisory_labels tool: %v", err)
	}
	scriptTools = append(scriptTools, setAdvisoryLabelsTool)

	llmModel, err := r.ModelFactory(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create model: %v", err)
	}

	skillAgent, err := llmagent.New(llmagent.Config{
		Name:        "skill_user_agent",
		Model:       llmModel,
		Description: "Monitor System",
		Instruction: `You are an autonomous SRE agent. On each run you MUST:
1. Call list_skills to discover available skills.
2. Call load_skill for each skill to read its SKILL.md instructions.
3. Follow those instructions exactly, step by step.
4. Use the run_script tool to execute scripts from the skill's scripts/ directory. Do NOT assume scripts are missing — always attempt to run them.
5. Use load_skill_resource to read templates from assets/ or references/.
6. Use create_advisory to raise advisories, then set_advisory_labels to label them.
7. Use update_owner if a skill instruction tells you to modify the owning Skill CR.
Never skip steps or assume failure without calling the tools.

CRITICAL ERROR HANDLING RULE: When ANY tool call fails or any script returns a non-zero exit code, you MUST:
  a) Call load_skill_resource to load the advisory template from the skill's assets/ directory (e.g. "assets/kueue-advisory-tool-failed.json").
  b) Fill in the template's placeholder fields with specific details about the failure.
  c) Call create_advisory with the filled-in name, advisory, explanation, and proposal fields.
NEVER respond with just text about an error. You MUST ALWAYS call create_advisory for every error.`,
		Tools:    scriptTools,
		Toolsets: []tool.Toolset{skillToolset},
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create agent: %v", err)
	}

	sessionService := session.InMemoryService()
	agentRunner, err := runner.New(runner.Config{
		AppName:        KHIERON,
		Agent:          skillAgent,
		SessionService: sessionService,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create runner: %v", err)
	}

	// Register the agent with the runner loop
	r.RunnerLoop.Register(crKey, &AgentEntry{
		Agent:          skillAgent,
		Runner:         agentRunner,
		SessionService: sessionService,
		SkillDir:       skillDir,
		ConfigMapRV:    configMap.ResourceVersion,
		Interval:       time.Duration(skillCr.Spec.IntervalMinute) * time.Minute,
		CRKey:          req.NamespacedName,
	})

	log.Info("Agent registered with runner loop", "cr", crKey)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SkillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agencyv1alpha1.Skill{}).
		Owns(&agencyv1alpha1.Advisory{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForConfigMap),
		).
		Named("skill").
		Complete(r)
}

func (r *SkillReconciler) findObjectsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap := obj.(*corev1.ConfigMap)
	var list agencyv1alpha1.SkillList

	// We list all Skills in the same namespace as the ConfigMap
	if err := r.List(ctx, &list, client.InNamespace(configMap.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, item := range list.Items {
		// Only trigger Reconcile if this specific CR references this ConfigMap
		if item.Spec.SkillConfigRef.Name == configMap.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.Name,
					Namespace: item.Namespace,
				},
			})
		}
	}
	return requests
}

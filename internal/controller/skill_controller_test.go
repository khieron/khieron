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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
)

// createSkillConfigMap creates the standard skill ConfigMap from example-skills.
func createSkillConfigMap(ctx context.Context, name, namespace string) {
	skillDir := filepath.Join("..", "..", "example-skills", "monitor-pods-skill", "skill-files")
	cmData := map[string]string{}
	for key, relPath := range map[string]string{
		"monitor-pods-skill___skill-files___SKILL.md":                    "SKILL.md",
		"monitor-pods-skill___skill-files___assets___pods-stuck.json":    filepath.Join("assets", "pods-stuck.json"),
		"monitor-pods-skill___skill-files___scripts___get-stuck-pods.sh": filepath.Join("scripts", "get-stuck-pods.sh"),
		"monitor-pods-skill___skill-files___references___REFERENCE.md":   filepath.Join("references", "REFERENCE.md"),
	} {
		data, err := os.ReadFile(filepath.Join(skillDir, relPath))
		Expect(err).NotTo(HaveOccurred())
		cmData[key] = string(data)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: cmData,
	}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &corev1.ConfigMap{})
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}
}

// newTestReconciler creates a SkillReconciler with a fake model for testing.
func newTestReconciler(instructionFile string) (*SkillReconciler, *AgentRunnerLoop) {
	runnerLoop := NewAgentRunnerLoop(k8sClient, k8sClient.Scheme(), "fake")
	runnerLoop.Model = &fakeModel{}
	runnerLoop.modelReady = true
	reconciler := &SkillReconciler{
		Client:          k8sClient,
		Scheme:          k8sClient.Scheme(),
		RunnerLoop:      runnerLoop,
		InstructionPath: instructionFile,
	}
	return reconciler, runnerLoop
}

var _ = Describe("Skill Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const configMapName = "monitor-pods-skill"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		skill := &agencyv1alpha1.Skill{}

		BeforeEach(func() {
			By("creating the ConfigMap with skill contents")
			createSkillConfigMap(ctx, configMapName, testNamespace)

			By("creating the custom resource for the Kind Skill")
			err := k8sClient.Get(ctx, typeNamespacedName, skill)
			if err != nil && errors.IsNotFound(err) {
				resource := &agencyv1alpha1.Skill{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testNamespace,
					},
					Spec: agencyv1alpha1.SkillSpec{
						SkillConfigRef: corev1.LocalObjectReference{
							Name: configMapName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agencyv1alpha1.Skill{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Skill")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleanup the ConfigMap")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: testNamespace}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Creating a temporary instruction file")
			instructionFile := filepath.Join(GinkgoT().TempDir(), "instruction.txt")
			Expect(os.WriteFile(instructionFile, []byte("You are a test agent."), 0644)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler, _ := newTestReconciler(instructionFile)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a resource with MCP config", func() {
		const resourceName = "test-mcp-resource"
		const configMapName = "monitor-pods-skill-mcp"
		const mcpConfigMapName = "test-mcp-config"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testNamespace,
		}

		BeforeEach(func() {
			By("creating the skill ConfigMap")
			createSkillConfigMap(ctx, configMapName, testNamespace)

			By("creating the MCP ConfigMap")
			mcpCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mcpConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"mcp.json": `{
						"mcpServers": {
							"test-server": {
								"type": "http",
								"url": "http://localhost:9999/mcp"
							}
						}
					}`,
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: mcpConfigMapName, Namespace: testNamespace}, &corev1.ConfigMap{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, mcpCM)).To(Succeed())
			}

			By("creating the Skill CR with MCP config ref")
			skill := &agencyv1alpha1.Skill{}
			err = k8sClient.Get(ctx, typeNamespacedName, skill)
			if err != nil && errors.IsNotFound(err) {
				resource := &agencyv1alpha1.Skill{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testNamespace,
					},
					Spec: agencyv1alpha1.SkillSpec{
						SkillConfigRef: corev1.LocalObjectReference{
							Name: configMapName,
						},
						MCPConfigRef: &corev1.LocalObjectReference{
							Name: mcpConfigMapName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &agencyv1alpha1.Skill{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: testNamespace}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())

			mcpCM := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mcpConfigMapName, Namespace: testNamespace}, mcpCM)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, mcpCM)).To(Succeed())
		})

		It("should reconcile with MCP toolsets and store MCP ConfigMap RV", func() {
			By("Creating a temporary instruction file")
			instructionFile := filepath.Join(GinkgoT().TempDir(), "instruction.txt")
			Expect(os.WriteFile(instructionFile, []byte("You are a test agent."), 0644)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler, runnerLoop := newTestReconciler(instructionFile)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the agent was registered with MCP ConfigMap RV")
			crKey := typeNamespacedName.String()
			mcpRV, exists := runnerLoop.GetMCPConfigMapRV(crKey)
			Expect(exists).To(BeTrue())
			Expect(mcpRV).NotTo(BeEmpty())
		})

		It("should fail if MCP ConfigMap is missing mcp.json key", func() {
			By("Replacing the MCP ConfigMap with one missing the mcp.json key")
			mcpCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mcpConfigMapName, Namespace: testNamespace}, mcpCM)).To(Succeed())
			mcpCM.Data = map[string]string{"other-key": "value"}
			Expect(k8sClient.Update(ctx, mcpCM)).To(Succeed())

			instructionFile := filepath.Join(GinkgoT().TempDir(), "instruction.txt")
			Expect(os.WriteFile(instructionFile, []byte("You are a test agent."), 0644)).To(Succeed())

			controllerReconciler, _ := newTestReconciler(instructionFile)

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mcp.json"))
		})
	})

	Context("findObjectsForConfigMap", func() {
		It("should return reconcile requests for Skills referencing an MCP ConfigMap", func() {
			const skillName = "test-mcp-watch"
			const configMapName = "skill-cm-watch"
			const mcpConfigMapName = "mcp-cm-watch"

			ctx := context.Background()

			By("creating the skill and MCP ConfigMaps")
			createSkillConfigMap(ctx, configMapName, testNamespace)
			mcpCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mcpConfigMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"mcp.json": `{"mcpServers":{}}`,
				},
			}
			Expect(k8sClient.Create(ctx, mcpCM)).To(Succeed())

			By("creating a Skill referencing the MCP ConfigMap")
			resource := &agencyv1alpha1.Skill{
				ObjectMeta: metav1.ObjectMeta{
					Name:      skillName,
					Namespace: testNamespace,
				},
				Spec: agencyv1alpha1.SkillSpec{
					SkillConfigRef: corev1.LocalObjectReference{
						Name: configMapName,
					},
					MCPConfigRef: &corev1.LocalObjectReference{
						Name: mcpConfigMapName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("verifying findObjectsForConfigMap returns the Skill for the MCP ConfigMap")
			instructionFile := filepath.Join(GinkgoT().TempDir(), "instruction.txt")
			Expect(os.WriteFile(instructionFile, []byte("test"), 0644)).To(Succeed())
			reconciler, _ := newTestReconciler(instructionFile)

			requests := reconciler.findObjectsForConfigMap(ctx, mcpCM)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(skillName))

			By("cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mcpCM)).To(Succeed())
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: testNamespace}, cm)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
	})
})

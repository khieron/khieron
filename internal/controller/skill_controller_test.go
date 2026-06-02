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
	"iter"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
)

type mockModel struct{}

func (m *mockModel) Name() string { return "mock" }

func (m *mockModel) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText("No issues found.", genai.RoleModel),
			FinishReason: genai.FinishReasonStop,
			TurnComplete: true,
		}, nil)
	}
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
					Name:      configMapName,
					Namespace: testNamespace,
				},
				Data: cmData,
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: testNamespace}, &corev1.ConfigMap{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			}

			By("creating the custom resource for the Kind Skill")
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
			controllerReconciler := &SkillReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				RunnerLoop:      NewAgentRunnerLoop(k8sClient, k8sClient.Scheme()),
				InstructionPath: instructionFile,
				ModelFactory: func(_ context.Context) (model.LLM, error) {
					return &mockModel{}, nil
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

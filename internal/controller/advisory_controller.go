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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agencyv1alpha1 "github.com/khieron/khieron/api/v1alpha1"
)

// AdvisoryReconciler reconciles a Advisory object
type AdvisoryReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	RunnerLoop *AgentRunnerLoop
}

// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agency.khieron.io,resources=advisories/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Advisory object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *AdvisoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var advisory agencyv1alpha1.Advisory
	if err := r.Get(ctx, req.NamespacedName, &advisory); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If approver is set and not yet approved, mark as approved and execute proposal
	if advisory.Spec.Approver != "" && advisory.Status.Approved == nil {
		now := metav1.Now()
		advisory.Status.Approved = &now
		advisory.Status.Updated = &now

		if err := r.Status().Update(ctx, &advisory); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update advisory status: %v", err)
		}

		log.Info("Advisory approved", "name", advisory.Name, "approver", advisory.Spec.Approver)
		r.Recorder.Eventf(&advisory, "Normal", "Approved",
			"Advisory approved by %s", advisory.Spec.Approver)

		// If there's a proposal, find the owner Skill and run the agent to execute it
		if advisory.Status.Proposal != "" {
			ownerKey := r.findOwnerSkill(advisory.GetOwnerReferences(), advisory.Namespace)
			if ownerKey != nil {
				prompt := fmt.Sprintf(
					"An advisory has been approved by %s. Execute the following proposal: %s\n\nAdvisory context: %s\nExplanation: %s",
					advisory.Spec.Approver,
					advisory.Status.Proposal,
					advisory.Status.Advisory,
					advisory.Status.Explaination,
				)
				if err := r.RunnerLoop.RunWithPrompt(ctx, *ownerKey, prompt); err != nil {
					log.Info("Failed to execute proposal", "name", advisory.Name, "error", err.Error())
					r.Recorder.Eventf(&advisory, "Warning", "ProposalFailed",
						"Failed to execute proposal: %s", err.Error())
				} else {
					log.Info("Proposal executed", "name", advisory.Name, "proposal", advisory.Status.Proposal)
					r.Recorder.Eventf(&advisory, "Normal", "ProposalExecuted",
						"Proposal executed: %s", advisory.Status.Proposal)
				}
			} else {
				log.Info("No owner Skill found for advisory", "name", advisory.Name)
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AdvisoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agencyv1alpha1.Advisory{}).
		Named("advisory").
		Complete(r)
}

// findOwnerSkill finds the owner Skill from the advisory's owner references.
func (r *AdvisoryReconciler) findOwnerSkill(refs []metav1.OwnerReference, namespace string) *types.NamespacedName {
	for _, ref := range refs {
		if ref.Kind == "Skill" {
			return &types.NamespacedName{
				Name:      ref.Name,
				Namespace: namespace,
			}
		}
	}
	return nil
}

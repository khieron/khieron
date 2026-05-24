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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SkillSpec defines the desired state of Skill.
type SkillSpec struct {
	// Skill is the agentic skill that will analyze the Kueue deployment
	// These skills should be targeted and granular
	// Add more KueueSkill resources if you need them
	// +kubebuilder:validation:Required
	SkillConfigRef corev1.LocalObjectReference `json:"skillconfigref"`

	// The analysis will run periodically at this interval, or on a change of spec
	// +kubebuilder:default:=5
	IntervalMinute int16 `json:"intervalminute,omitempty"`

	// +kubebuilder:default:=true
	EnableAgent bool `json:"enableagent"`
}

// TokenUsage tracks token counts from a Gemini API run.
type TokenUsage struct {
	// Tokens in the prompt
	PromptTokenCount int32 `json:"promptTokenCount,omitempty"`
	// Tokens in the model's response
	CandidatesTokenCount int32 `json:"candidatesTokenCount,omitempty"`
	// Tokens used for model reasoning
	ThoughtsTokenCount int32 `json:"thoughtsTokenCount,omitempty"`
	// Tokens from tool results fed back to the model
	ToolUsePromptTokenCount int32 `json:"toolUsePromptTokenCount,omitempty"`
	// Total tokens (prompt + candidates + tool use + thoughts)
	TotalTokenCount int32 `json:"totalTokenCount,omitempty"`
}

// TokensAccumulated tracks cumulative token usage across all runs.
type TokensAccumulated struct {
	// Total tokens consumed across all runs
	TotalTokenCount int64 `json:"totalTokenCount,omitempty"`
	// Number of completed agent runs
	RunCount int32 `json:"runCount,omitempty"`
}

// SkillStatus defines the observed state of Skill.
type SkillStatus struct {
	// Time of last analysis
	LastAnalyzedAt *metav1.Time `json:"lastanalysedat,omitempty"`
	// Token usage from the most recent run
	TokensLastRun *TokenUsage `json:"tokensLastRun,omitempty"`
	// Cumulative token usage across all runs
	TokensTotal *TokensAccumulated `json:"tokensTotal,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Skill is the Schema for the skills API.
type Skill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillSpec   `json:"spec,omitempty"`
	Status SkillStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SkillList contains a list of Skill.
type SkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skill `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Skill{}, &SkillList{})
}

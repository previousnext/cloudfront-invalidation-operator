package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type InvalidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Invalidation `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Invalidation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              InvalidationSpec   `json:"spec"`
	Status            InvalidationStatus `json:"status,omitempty"`
}

type InvalidationSpec struct {
	ConfigMap string `json:"configMap"`
	Path      string `json:"path"`
}

type Phase string

const PhaseCompleted Phase = "Completed"

type InvalidationStatus struct {
	Phase Phase `json:"phase"`
}

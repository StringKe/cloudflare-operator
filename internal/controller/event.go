/*
Copyright 2025 Adyanth H.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// RecordEventAndSetCondition records an event and sets a condition on the resource
// This is a common pattern used throughout the controllers
func RecordEventAndSetCondition(
	recorder record.EventRecorder,
	obj runtime.Object,
	conditions *[]metav1.Condition,
	eventType string,
	reason string,
	message string,
	conditionStatus metav1.ConditionStatus,
) {
	recorder.Event(obj, eventType, reason, message)
	SetCondition(conditions, "Ready", conditionStatus, reason, message)
}

// RecordSuccessEventAndCondition records a success event and sets Ready condition to True
func RecordSuccessEventAndCondition(
	recorder record.EventRecorder,
	obj runtime.Object,
	conditions *[]metav1.Condition,
	reason string,
	message string,
) {
	RecordEventAndSetCondition(
		recorder,
		obj,
		conditions,
		corev1.EventTypeNormal,
		reason,
		message,
		metav1.ConditionTrue,
	)
}

// RecordWarningEventAndCondition records a warning event and sets Ready condition to False
func RecordWarningEventAndCondition(
	recorder record.EventRecorder,
	obj runtime.Object,
	conditions *[]metav1.Condition,
	reason string,
	message string,
) {
	RecordEventAndSetCondition(
		recorder,
		obj,
		conditions,
		corev1.EventTypeWarning,
		reason,
		message,
		metav1.ConditionFalse,
	)
}

// RecordErrorEventAndCondition records an error event and sets Ready condition to False
// It sanitizes the error message to remove sensitive information
func RecordErrorEventAndCondition(
	recorder record.EventRecorder,
	obj runtime.Object,
	conditions *[]metav1.Condition,
	reason string,
	err error,
) {
	message := cf.SanitizeErrorMessage(err)
	RecordEventAndSetCondition(
		recorder,
		obj,
		conditions,
		corev1.EventTypeWarning,
		reason,
		message,
		metav1.ConditionFalse,
	)
}

// RecordError is a shorthand for recording an error event with sanitized message
// Does not modify conditions
func RecordError(recorder record.EventRecorder, obj runtime.Object, reason string, err error) {
	recorder.Event(obj, corev1.EventTypeWarning, reason, cf.SanitizeErrorMessage(err))
}

// RecordSuccess is a shorthand for recording a success event
// Does not modify conditions
func RecordSuccess(recorder record.EventRecorder, obj runtime.Object, reason string, message string) {
	recorder.Event(obj, corev1.EventTypeNormal, reason, message)
}

// Additional event reasons not defined in constants.go
const (
	// Initialization events
	EventReasonSecretNotFound  = "SecretNotFound"
	EventReasonCredentialError = "CredentialError"

	// Finalizer events
	EventReasonFinalizerAdded = "FinalizerAdded"

	// Conflict events
	EventReasonConflict         = "Conflict"
	EventReasonManagedByAnother = "ManagedByAnother"
)

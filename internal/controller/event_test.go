// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

type fakeEvent struct {
	Object    runtime.Object
	EventType string
	Reason    string
	Message   string
}

type fakeEventRecorder struct {
	Events []fakeEvent
}

func (f *fakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	f.Events = append(f.Events, fakeEvent{
		Object:    object,
		EventType: eventtype,
		Reason:    reason,
		Message:   message,
	})
}

func (*fakeEventRecorder) Eventf(_ runtime.Object, _, _, _ string, _ ...interface{}) {
	// Not used in tests
}

func (*fakeEventRecorder) AnnotatedEventf(
	_ runtime.Object,
	_ map[string]string,
	_, _, _ string,
	_ ...interface{},
) {
	// Not used in tests
}

var _ record.EventRecorder = &fakeEventRecorder{}

func TestRecordEventAndSetCondition(t *testing.T) {
	tests := []struct {
		name            string
		eventType       string
		reason          string
		message         string
		conditionStatus metav1.ConditionStatus
	}{
		{
			name:            "success event",
			eventType:       corev1.EventTypeNormal,
			reason:          "Created",
			message:         "Resource created successfully",
			conditionStatus: metav1.ConditionTrue,
		},
		{
			name:            "warning event",
			eventType:       corev1.EventTypeWarning,
			reason:          "Failed",
			message:         "Failed to create resource",
			conditionStatus: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &fakeEventRecorder{}
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}
			var conditions []metav1.Condition

			RecordEventAndSetCondition(
				recorder,
				obj,
				&conditions,
				tt.eventType,
				tt.reason,
				tt.message,
				tt.conditionStatus,
			)

			// Verify event was recorded
			assert.Len(t, recorder.Events, 1)
			assert.Equal(t, tt.eventType, recorder.Events[0].EventType)
			assert.Equal(t, tt.reason, recorder.Events[0].Reason)
			assert.Equal(t, tt.message, recorder.Events[0].Message)

			// Verify condition was set
			assert.Len(t, conditions, 1)
			assert.Equal(t, "Ready", conditions[0].Type)
			assert.Equal(t, tt.conditionStatus, conditions[0].Status)
			assert.Equal(t, tt.reason, conditions[0].Reason)
			assert.Equal(t, tt.message, conditions[0].Message)
		})
	}
}

func TestRecordSuccessEventAndCondition(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	var conditions []metav1.Condition

	RecordSuccessEventAndCondition(
		recorder,
		obj,
		&conditions,
		"Reconciled",
		"Resource reconciled successfully",
	)

	// Verify event was recorded as Normal
	assert.Len(t, recorder.Events, 1)
	assert.Equal(t, corev1.EventTypeNormal, recorder.Events[0].EventType)
	assert.Equal(t, "Reconciled", recorder.Events[0].Reason)

	// Verify condition was set to True
	assert.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
}

func TestRecordWarningEventAndCondition(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	var conditions []metav1.Condition

	RecordWarningEventAndCondition(
		recorder,
		obj,
		&conditions,
		"ConfigError",
		"Invalid configuration detected",
	)

	// Verify event was recorded as Warning
	assert.Len(t, recorder.Events, 1)
	assert.Equal(t, corev1.EventTypeWarning, recorder.Events[0].EventType)
	assert.Equal(t, "ConfigError", recorder.Events[0].Reason)

	// Verify condition was set to False
	assert.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, conditions[0].Status)
}

func TestRecordErrorEventAndCondition(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	var conditions []metav1.Condition

	testErr := errors.New("connection refused")

	RecordErrorEventAndCondition(
		recorder,
		obj,
		&conditions,
		"ConnectionError",
		testErr,
	)

	// Verify event was recorded as Warning
	assert.Len(t, recorder.Events, 1)
	assert.Equal(t, corev1.EventTypeWarning, recorder.Events[0].EventType)
	assert.Equal(t, "ConnectionError", recorder.Events[0].Reason)
	// Message should be sanitized (in this case, no sensitive info)
	assert.Contains(t, recorder.Events[0].Message, "connection refused")

	// Verify condition was set to False
	assert.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, conditions[0].Status)
}

func TestRecordError(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	testErr := errors.New("something went wrong")

	RecordError(recorder, obj, "TestError", testErr)

	// Verify event was recorded as Warning
	assert.Len(t, recorder.Events, 1)
	assert.Equal(t, corev1.EventTypeWarning, recorder.Events[0].EventType)
	assert.Equal(t, "TestError", recorder.Events[0].Reason)
	assert.Contains(t, recorder.Events[0].Message, "something went wrong")
}

func TestRecordSuccess(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	RecordSuccess(recorder, obj, "Created", "Resource created successfully")

	// Verify event was recorded as Normal
	assert.Len(t, recorder.Events, 1)
	assert.Equal(t, corev1.EventTypeNormal, recorder.Events[0].EventType)
	assert.Equal(t, "Created", recorder.Events[0].Reason)
	assert.Equal(t, "Resource created successfully", recorder.Events[0].Message)
}

func TestEventReasonConstants(t *testing.T) {
	// Verify event reason constants are defined correctly
	assert.Equal(t, "SecretNotFound", EventReasonSecretNotFound)
	assert.Equal(t, "CredentialError", EventReasonCredentialError)
	assert.Equal(t, "FinalizerAdded", EventReasonFinalizerAdded)
	assert.Equal(t, "Conflict", EventReasonConflict)
	assert.Equal(t, "ManagedByAnother", EventReasonManagedByAnother)
}

func TestMultipleEventRecording(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	// Record multiple events
	RecordSuccess(recorder, obj, "Step1", "First step completed")
	RecordSuccess(recorder, obj, "Step2", "Second step completed")
	RecordError(recorder, obj, "Step3Failed", errors.New("third step failed"))
	RecordSuccess(recorder, obj, "Recovered", "Recovered from error")

	// Verify all events were recorded
	assert.Len(t, recorder.Events, 4)
	assert.Equal(t, "Step1", recorder.Events[0].Reason)
	assert.Equal(t, "Step2", recorder.Events[1].Reason)
	assert.Equal(t, "Step3Failed", recorder.Events[2].Reason)
	assert.Equal(t, "Recovered", recorder.Events[3].Reason)
}

func TestConditionUpdates(t *testing.T) {
	recorder := &fakeEventRecorder{}
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	var conditions []metav1.Condition

	// First set condition to True
	RecordSuccessEventAndCondition(recorder, obj, &conditions, "Success1", "First success")
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)

	// Then set condition to False
	RecordWarningEventAndCondition(recorder, obj, &conditions, "Warning1", "Warning occurred")
	// meta.SetStatusCondition should update existing condition
	assert.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, conditions[0].Status)

	// Set back to True
	RecordSuccessEventAndCondition(recorder, obj, &conditions, "Success2", "Recovered")
	assert.Len(t, conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
}

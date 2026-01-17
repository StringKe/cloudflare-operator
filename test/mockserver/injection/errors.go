// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package injection provides error injection capabilities for testing.
package injection

import (
	"math/rand"
	"regexp"
	"sync"
)

// ErrorType defines the type of error to inject.
type ErrorType string

const (
	// ErrorTypeRateLimit simulates a 429 rate limit error.
	ErrorTypeRateLimit ErrorType = "rate_limit"
	// ErrorTypeServerError simulates a 500 server error.
	ErrorTypeServerError ErrorType = "server_error"
	// ErrorTypeTimeout simulates a request timeout.
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeConflict simulates a 409 conflict error.
	ErrorTypeConflict ErrorType = "conflict"
	// ErrorTypeNotFound simulates a 404 not found error.
	ErrorTypeNotFound ErrorType = "not_found"
)

// TriggerMode defines when to trigger the error.
type TriggerMode string

const (
	// TriggerModeAlways always triggers the error.
	TriggerModeAlways TriggerMode = "always"
	// TriggerModeProbability triggers based on probability.
	TriggerModeProbability TriggerMode = "probability"
	// TriggerModeCount triggers for a specific number of requests.
	TriggerModeCount TriggerMode = "count"
)

// ErrorInjection defines an error injection rule.
type ErrorInjection struct {
	// PathPattern is a regex pattern to match request paths.
	PathPattern string
	// MethodPattern is a regex pattern to match HTTP methods (empty matches all).
	MethodPattern string
	// ErrorType is the type of error to inject.
	ErrorType ErrorType
	// TriggerMode defines when to trigger the error.
	TriggerMode TriggerMode
	// Probability is the probability of triggering (0-100) for TriggerModeProbability.
	Probability int
	// CountLimit is the number of times to trigger for TriggerModeCount.
	CountLimit int

	// Internal state
	triggerCount int
	pathRegex    *regexp.Regexp
	methodRegex  *regexp.Regexp
}

// InjectedError represents an injected error.
type InjectedError struct {
	Type    ErrorType
	Message string
}

// ErrorInjector manages error injection rules.
type ErrorInjector struct {
	mu         sync.RWMutex
	injections []*ErrorInjection
}

// NewErrorInjector creates a new error injector.
func NewErrorInjector() *ErrorInjector {
	return &ErrorInjector{
		injections: make([]*ErrorInjection, 0),
	}
}

// Add adds a new error injection rule.
func (e *ErrorInjector) Add(injection ErrorInjection) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Compile path regex
	pathRegex, err := regexp.Compile(injection.PathPattern)
	if err != nil {
		return err
	}
	injection.pathRegex = pathRegex

	// Compile method regex (default to match all)
	if injection.MethodPattern == "" {
		injection.MethodPattern = ".*"
	}
	methodRegex, err := regexp.Compile(injection.MethodPattern)
	if err != nil {
		return err
	}
	injection.methodRegex = methodRegex

	e.injections = append(e.injections, &injection)
	return nil
}

// AddSimple is a convenience method to add a simple error injection.
func (e *ErrorInjector) AddSimple(pathPattern string, errorType ErrorType) error {
	return e.Add(ErrorInjection{
		PathPattern: pathPattern,
		ErrorType:   errorType,
		TriggerMode: TriggerModeAlways,
	})
}

// AddWithCount adds an error injection that triggers for a specific count.
func (e *ErrorInjector) AddWithCount(pathPattern string, errorType ErrorType, count int) error {
	return e.Add(ErrorInjection{
		PathPattern: pathPattern,
		ErrorType:   errorType,
		TriggerMode: TriggerModeCount,
		CountLimit:  count,
	})
}

// AddWithProbability adds an error injection with a probability.
func (e *ErrorInjector) AddWithProbability(pathPattern string, errorType ErrorType, probability int) error {
	return e.Add(ErrorInjection{
		PathPattern: pathPattern,
		ErrorType:   errorType,
		TriggerMode: TriggerModeProbability,
		Probability: probability,
	})
}

// Check checks if an error should be injected for the given path and method.
func (e *ErrorInjector) Check(path, method string) *InjectedError {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, injection := range e.injections {
		if !injection.pathRegex.MatchString(path) {
			continue
		}
		if !injection.methodRegex.MatchString(method) {
			continue
		}

		switch injection.TriggerMode {
		case TriggerModeAlways:
			return &InjectedError{
				Type:    injection.ErrorType,
				Message: "Injected error (always)",
			}
		case TriggerModeProbability:
			if rand.Intn(100) < injection.Probability {
				return &InjectedError{
					Type:    injection.ErrorType,
					Message: "Injected error (probability)",
				}
			}
		case TriggerModeCount:
			if injection.triggerCount < injection.CountLimit {
				injection.triggerCount++
				return &InjectedError{
					Type:    injection.ErrorType,
					Message: "Injected error (count)",
				}
			}
		}
	}

	return nil
}

// Remove removes error injections matching the given path pattern.
func (e *ErrorInjector) Remove(pathPattern string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	newInjections := make([]*ErrorInjection, 0)
	for _, injection := range e.injections {
		if injection.PathPattern != pathPattern {
			newInjections = append(newInjections, injection)
		}
	}
	e.injections = newInjections
}

// Clear removes all error injections.
func (e *ErrorInjector) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.injections = make([]*ErrorInjection, 0)
}

// ResetCounts resets all trigger counts.
func (e *ErrorInjector) ResetCounts() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, injection := range e.injections {
		injection.triggerCount = 0
	}
}

// Count returns the number of active injections.
func (e *ErrorInjector) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.injections)
}

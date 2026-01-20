// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	"context"
	"testing"
)

func TestPagesProjectValidator_ValidateCreate(t *testing.T) {
	validator := &PagesProjectValidator{}

	tests := []struct {
		name    string
		project *PagesProject
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid project with latest production target",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "latest",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with specific version target",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "v1.0.0",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with empty production target",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid production target - version not found",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "v2.0.0",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
					},
				},
			},
			wantErr: true,
			errMsg:  "version \"v2.0.0\" not found",
		},
		{
			name: "invalid - duplicate version names",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v1.0.0"},
					},
				},
			},
			wantErr: true,
			errMsg:  "Duplicate value",
		},
		{
			name: "valid - unique version names",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
						{Name: "v0.8.0"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateCreate(context.Background(), tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateCreate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestPagesProjectValidator_ValidateUpdate(t *testing.T) {
	validator := &PagesProjectValidator{}

	oldProject := &PagesProject{
		Spec: PagesProjectSpec{
			ProductionTarget: "v1.0.0",
			Versions: []ProjectVersion{
				{Name: "v1.0.0"},
			},
		},
	}

	newProject := &PagesProject{
		Spec: PagesProjectSpec{
			ProductionTarget: "v2.0.0",
			Versions: []ProjectVersion{
				{Name: "v1.0.0"},
			},
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldProject, newProject)
	if err == nil {
		t.Error("ValidateUpdate() expected error for invalid production target, got nil")
	}
}

func TestPagesProjectValidator_validateProductionTarget(t *testing.T) {
	validator := &PagesProjectValidator{}

	tests := []struct {
		name    string
		project *PagesProject
		wantErr bool
	}{
		{
			name: "latest is always valid",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "latest",
					Versions:         []ProjectVersion{},
				},
			},
			wantErr: false,
		},
		{
			name: "empty target is valid",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "",
				},
			},
			wantErr: false,
		},
		{
			name: "existing version is valid",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "v1.0.0",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "non-existing version is invalid",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionTarget: "v99.99.99",
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateProductionTarget(tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProductionTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPagesProjectValidator_validateVersionUniqueness(t *testing.T) {
	validator := &PagesProjectValidator{}

	tests := []struct {
		name    string
		project *PagesProject
		wantErr bool
	}{
		{
			name: "all unique versions",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
						{Name: "v0.8.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate versions",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					Versions: []ProjectVersion{
						{Name: "v1.0.0"},
						{Name: "v0.9.0"},
						{Name: "v1.0.0"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty versions list",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					Versions: []ProjectVersion{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateVersionUniqueness(tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVersionUniqueness() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

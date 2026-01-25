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
			name: "valid project with no version management",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with gitops policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyGitOps,
						GitOps: &GitOpsVersionConfig{
							PreviewVersion: "v1.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with gitops policy and both versions",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyGitOps,
						GitOps: &GitOpsVersionConfig{
							PreviewVersion:    "v2.0.0",
							ProductionVersion: "v1.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with targetVersion policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyTargetVersion,
						TargetVersion: &TargetVersionSpec{
							Version: "sha-abc123",
							SourceTemplate: SourceTemplate{
								Type: HTTPSourceTemplateType,
								HTTP: &HTTPSourceTemplate{
									URLTemplate: "https://example.com/{{.Version}}/dist.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid project with declarativeVersions policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyDeclarativeVersions,
						DeclarativeVersions: &DeclarativeVersionsSpec{
							Versions: []string{"v1.0.0", "v0.9.0"},
							SourceTemplate: SourceTemplate{
								Type: HTTPSourceTemplateType,
								HTTP: &HTTPSourceTemplate{
									URLTemplate: "https://example.com/{{.Version}}/dist.tar.gz",
								},
							},
							ProductionTarget: "latest",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - targetVersion policy without targetVersion config",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyTargetVersion,
					},
				},
			},
			wantErr: true,
			errMsg:  "targetVersion is required",
		},
		{
			name: "invalid - declarativeVersions policy without config",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyDeclarativeVersions,
					},
				},
			},
			wantErr: true,
			errMsg:  "declarativeVersions is required",
		},
		{
			name: "invalid - gitops policy without config",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyGitOps,
					},
				},
			},
			wantErr: true,
			errMsg:  "gitops is required",
		},
		{
			name: "valid - none policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyNone,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - latestPreview policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy:        VersionPolicyLatestPreview,
						LatestPreview: &LatestPreviewConfig{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - autoPromote policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy:      VersionPolicyAutoPromote,
						AutoPromote: &AutoPromoteConfig{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid - external policy",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyExternal,
						External: &ExternalVersionConfig{
							CurrentVersion:    "v1.0.0",
							ProductionVersion: "v1.0.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - duplicate versions in declarativeVersions",
			project: &PagesProject{
				Spec: PagesProjectSpec{
					ProductionBranch: "main",
					VersionManagement: &VersionManagement{
						Policy: VersionPolicyDeclarativeVersions,
						DeclarativeVersions: &DeclarativeVersionsSpec{
							Versions: []string{"v1.0.0", "v1.0.0"},
							SourceTemplate: SourceTemplate{
								Type: HTTPSourceTemplateType,
								HTTP: &HTTPSourceTemplate{
									URLTemplate: "https://example.com/{{.Version}}/dist.tar.gz",
								},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "Duplicate value",
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
			ProductionBranch: "main",
			VersionManagement: &VersionManagement{
				Policy: VersionPolicyGitOps,
				GitOps: &GitOpsVersionConfig{
					PreviewVersion: "v1.0.0",
				},
			},
		},
	}

	newProject := &PagesProject{
		Spec: PagesProjectSpec{
			ProductionBranch: "main",
			VersionManagement: &VersionManagement{
				Policy: VersionPolicyGitOps,
				GitOps: &GitOpsVersionConfig{
					PreviewVersion:    "v2.0.0",
					ProductionVersion: "v1.0.0",
				},
			},
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldProject, newProject)
	if err != nil {
		t.Errorf("ValidateUpdate() unexpected error: %v", err)
	}
}

func TestPagesProjectValidator_validateGitOps(t *testing.T) {
	validator := &PagesProjectValidator{}

	tests := []struct {
		name    string
		gitops  *GitOpsVersionConfig
		wantErr bool
	}{
		{
			name: "valid with preview version only",
			gitops: &GitOpsVersionConfig{
				PreviewVersion: "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "valid with both versions",
			gitops: &GitOpsVersionConfig{
				PreviewVersion:    "v2.0.0",
				ProductionVersion: "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "valid with source template",
			gitops: &GitOpsVersionConfig{
				PreviewVersion: "v1.0.0",
				SourceTemplate: &SourceTemplate{
					Type: HTTPSourceTemplateType,
					HTTP: &HTTPSourceTemplate{
						URLTemplate: "https://example.com/{{.Version}}/dist.tar.gz",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty config is valid",
			gitops:  &GitOpsVersionConfig{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateGitOps(nil, tt.gitops)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("validateGitOps() errors = %v, wantErr %v", errs, tt.wantErr)
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

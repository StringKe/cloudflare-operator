// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package handlers provides HTTP handlers for the mock Cloudflare API server.
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/StringKe/cloudflare-operator/test/mockserver/models"
)

// Response writes a standard Cloudflare API response.
func Response[T any](w http.ResponseWriter, status int, result T, errors []models.APIError) {
	resp := models.Response[T]{
		Success:  len(errors) == 0,
		Errors:   errors,
		Messages: []string{},
		Result:   result,
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// ResponseWithResultInfo writes a response with pagination info.
func ResponseWithResultInfo[T any](w http.ResponseWriter, status int, result T, resultInfo *models.ResultInfo) {
	resp := models.Response[T]{
		Success:    true,
		Errors:     []models.APIError{},
		Messages:   []string{},
		Result:     result,
		ResultInfo: resultInfo,
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// Success writes a successful response.
func Success[T any](w http.ResponseWriter, result T) {
	Response(w, http.StatusOK, result, nil)
}

// Created writes a successful creation response.
func Created[T any](w http.ResponseWriter, result T) {
	Response(w, http.StatusCreated, result, nil)
}

// Error writes an error response.
func Error(w http.ResponseWriter, status int, code int, message string) {
	var empty interface{}
	Response(w, status, empty, []models.APIError{{Code: code, Message: message}})
}

// NotFound writes a not found error response.
func NotFound(w http.ResponseWriter, resource string) {
	Error(w, http.StatusNotFound, 10000, resource+" not found")
}

// BadRequest writes a bad request error response.
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, 10001, message)
}

// Conflict writes a conflict error response.
func Conflict(w http.ResponseWriter, message string) {
	Error(w, http.StatusConflict, 10002, message)
}

// InternalError writes an internal server error response.
func InternalError(w http.ResponseWriter, message string) {
	Error(w, http.StatusInternalServerError, 10003, message)
}

// ReadJSON reads and unmarshals JSON from the request body.
func ReadJSON[T any](r *http.Request) (*T, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GenerateID generates a random UUID-like string.
func GenerateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}

// GenerateToken generates a random token string.
func GenerateToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GetPathParam extracts a path parameter from the request.
func GetPathParam(r *http.Request, name string) string {
	return r.PathValue(name)
}

// GetQueryParam extracts a query parameter from the request.
func GetQueryParam(r *http.Request, name string) string {
	return r.URL.Query().Get(name)
}

// BoolPtr returns a pointer to a bool.
func BoolPtr(b bool) *bool {
	return &b
}

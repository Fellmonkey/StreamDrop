// Package apierr contains the machine-readable error model shared by every
// transport. The server never sends localized prose for interface elements: it
// sends a stable code plus the values needed to build the message on the client
// (see back_spec.md §5.3).
package apierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Kind is the coarse error category. It matches transfers.error_kind and the
// frontend contract.
type Kind string

const (
	KindNetwork Kind = "network"
	KindAuth    Kind = "auth"
	KindSource  Kind = "source"
	KindCloud   Kind = "cloud"
	KindAborted Kind = "aborted"
)

// Code is a stable machine-readable error code.
type Code string

const (
	CodeInspectFailed          Code = "inspect_failed"
	CodeInspectHTTPError       Code = "inspect_http_error"
	CodeInspectRedirectLimit   Code = "inspect_redirect_limit"
	CodeInspectHTMLSource      Code = "inspect_html_source"
	CodeInspectNotHeadSupport  Code = "inspect_not_head_supported"
	CodeSourceUnreachable      Code = "source_unreachable"
	CodeSourceInterrupted      Code = "source_interrupted"
	CodeAuthRequired           Code = "auth_required"
	CodeAuthRejected           Code = "auth_rejected"
	CodeAuthTokenExpired       Code = "auth_token_expired"
	CodeAuthOAuthRequired      Code = "auth_oauth_required"
	CodeAuthClientNotEnabled   Code = "auth_client_not_enabled"
	CodeCloudUnreachable       Code = "cloud_unreachable"
	CodeCloudGoogleAPIDisabled Code = "cloud_google_api_disabled"
	CodeCloudQuotaExceeded     Code = "cloud_quota_exceeded"
	CodeCloudWriteFailed       Code = "cloud_write_failed"
	CodeCloudCleanupFailed     Code = "cloud_cleanup_failed"
	CodeEngineUnavailable      Code = "engine_unavailable"
	CodeDependencyMissing      Code = "dependency_missing"
	CodeDependencyCorrupted    Code = "dependency_corrupted"
	CodeTransferAborted        Code = "transfer_aborted"
	CodeQueuePaused            Code = "queue_paused"
	CodeRemoteNotFound         Code = "remote_not_found"
	CodeValidationFailed       Code = "validation_failed"
	CodeTelegramChatInvalid    Code = "telegram_chat_invalid"
	CodeLocalAuthRequired      Code = "local_auth_required"

	// Extension codes. They exist in the catalog below just like the codes
	// from the specification so that every code a REST response can carry is
	// documented in one place.
	CodeInternalError Code = "internal_error"
	CodeNotFound      Code = "not_found"
)

// Spec is the HTTP status and category a code maps to.
type Spec struct {
	HTTP int  `json:"http_status"`
	Kind Kind `json:"kind"`
}

// Catalog is the single source of truth for response codes.
var Catalog = map[Code]Spec{
	CodeInspectFailed:          {http.StatusBadGateway, KindSource},
	CodeInspectHTTPError:       {http.StatusBadGateway, KindSource},
	CodeInspectRedirectLimit:   {http.StatusBadGateway, KindSource},
	CodeInspectHTMLSource:      {http.StatusUnprocessableEntity, KindSource},
	CodeInspectNotHeadSupport:  {http.StatusBadGateway, KindSource},
	CodeSourceUnreachable:      {http.StatusBadGateway, KindNetwork},
	CodeSourceInterrupted:      {http.StatusBadGateway, KindNetwork},
	CodeAuthRequired:           {http.StatusUnauthorized, KindAuth},
	CodeAuthRejected:           {http.StatusUnauthorized, KindAuth},
	CodeAuthTokenExpired:       {http.StatusUnauthorized, KindAuth},
	CodeAuthOAuthRequired:      {http.StatusUnauthorized, KindAuth},
	CodeAuthClientNotEnabled:   {http.StatusUnauthorized, KindAuth},
	CodeCloudUnreachable:       {http.StatusBadGateway, KindCloud},
	CodeCloudGoogleAPIDisabled: {http.StatusBadGateway, KindCloud},
	CodeCloudQuotaExceeded:     {http.StatusInsufficientStorage, KindCloud},
	CodeCloudWriteFailed:       {http.StatusBadGateway, KindCloud},
	CodeCloudCleanupFailed:     {http.StatusInternalServerError, KindCloud},
	CodeEngineUnavailable:      {http.StatusServiceUnavailable, KindSource},
	CodeDependencyMissing:      {http.StatusServiceUnavailable, KindSource},
	CodeDependencyCorrupted:    {http.StatusServiceUnavailable, KindSource},
	CodeTransferAborted:        {499, KindAborted},
	CodeQueuePaused:            {http.StatusConflict, KindAborted},
	CodeRemoteNotFound:         {http.StatusNotFound, KindCloud},
	CodeValidationFailed:       {http.StatusBadRequest, KindSource},
	CodeLocalAuthRequired:      {http.StatusUnauthorized, KindAuth},
	CodeInternalError:          {http.StatusInternalServerError, KindSource},
	CodeNotFound:               {http.StatusNotFound, KindSource},
}

// Error is the error value all internal layers return when they need to
// control the code the client sees.
type Error struct {
	Code    Code           `json:"code"`
	Kind    Kind           `json:"kind"`
	HTTP    int            `json:"http_status"`
	Params  map[string]any `json:"params,omitempty"`
	Details string         `json:"details,omitempty"`
	Cause   error          `json:"-"`
}

// New builds an error for code with the catalog defaults.
func New(code Code, params map[string]any) *Error {
	spec, ok := Catalog[code]
	if !ok {
		spec = Catalog[CodeInternalError]
	}
	return &Error{Code: code, Kind: spec.Kind, HTTP: spec.HTTP, Params: params}
}

// Wrap builds an error for code and keeps the technical cause in Details and
// Cause. Details are never shown in primary UI hints.
func Wrap(code Code, cause error, params map[string]any) *Error {
	e := New(code, params)
	if cause != nil {
		e.Details = cause.Error()
		e.Cause = cause
	}
	return e
}

// WithDetail adds a technical detail string.
func (e *Error) WithDetail(detail string) *Error {
	e.Details = detail
	return e
}

func (e *Error) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Details)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Cause }

// From normalizes any error into an *Error. Unknown errors become
// internal_error with the technical text kept in Details.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	out := New(CodeInternalError, nil)
	out.Details = err.Error()
	out.Cause = err
	return out
}

// Envelope is the wire format: {"error": {...}}.
type Envelope struct {
	Error Payload `json:"error"`
}

// Payload is the error object itself.
type Payload struct {
	Code       Code           `json:"code"`
	Kind       Kind           `json:"kind"`
	HTTPStatus int            `json:"http_status"`
	Params     map[string]any `json:"params,omitempty"`
	Details    string         `json:"details,omitempty"`
}

// EnvelopeOf builds the wire envelope.
func EnvelopeOf(err error) Envelope {
	e := From(err)
	return Envelope{Error: Payload{
		Code:       e.Code,
		Kind:       e.Kind,
		HTTPStatus: e.HTTP,
		Params:     e.Params,
		Details:    e.Details,
	}}
}

// Write serializes the error envelope with the right HTTP status.
func Write(w http.ResponseWriter, err error) {
	e := From(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(e.HTTP)
	_ = json.NewEncoder(w).Encode(EnvelopeOf(e))
}

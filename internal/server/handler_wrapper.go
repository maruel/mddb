package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"

	apierrors "github.com/maruel/mddb/internal/errors"
)

// Wrap wraps a handler function to work as an http.Handler.
// The function must have signature: func(context.Context, In) (*Out, error)
// where In can be unmarshalled from JSON and Out is a struct.
// Path parameters can be extracted by tagging struct fields with `path:"name"`.
//
// Example:
//
//	type GetPageRequest struct {
//	    ID string `path:"id"`
//	}
//
//	func (h *Handler) GetPage(ctx context.Context, req GetPageRequest) (*Response, error)
func Wrap[In any, Out any](fn func(context.Context, In) (*Out, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Read request body
		body, err := io.ReadAll(r.Body)
		if err2 := r.Body.Close(); err == nil {
			err = err2
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to read request body", "err", err)
			writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body")
			return
		}
		var input In
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
				return
			}
		}

		// Extract path parameters and populate request struct
		populatePathParams(r, &input)
		// Extract query parameters and populate request struct
		populateQueryParams(r, &input)

		output, err := fn(ctx, input)
		if err != nil {
			statusCode := http.StatusInternalServerError
			errorCode := apierrors.ErrInternal
			details := make(map[string]any)

			var ewsErr apierrors.ErrorWithStatus
			if errors.As(err, &ewsErr) {
				statusCode = ewsErr.StatusCode()
				errorCode = ewsErr.Code()
				if d := ewsErr.Details(); d != nil {
					details = d
				}
			}

			slog.ErrorContext(ctx, "Handler error", "err", err, "statusCode", statusCode, "code", errorCode)
			writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(output); err != nil {
			slog.ErrorContext(ctx, "Failed to encode response", "err", err)
		}
	})
}

// populatePathParams extracts path parameters from the request and populates
// struct fields tagged with `path:"paramName"`.
func populatePathParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Ptr {
		return // Skip if not a pointer
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return // Skip if not a struct
	}

	typ := elem.Type()
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := field.Tag.Get("path")
		if tag == "" {
			continue
		}

		paramValue := r.PathValue(tag)
		if paramValue == "" {
			continue
		}

		// Set the field value if it's a string field
		if field.Type.Kind() == reflect.String {
			elem.Field(i).SetString(paramValue)
		}
	}
}

// populateQueryParams extracts query parameters from the request and populates
// struct fields tagged with `query:"paramName"`.
func populateQueryParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Ptr {
		return // Skip if not a pointer
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return // Skip if not a struct
	}

	query := r.URL.Query()
	typ := elem.Type()
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := field.Tag.Get("query")
		if tag == "" {
			continue
		}

		paramValue := query.Get(tag)
		if paramValue == "" {
			continue
		}

		// Set the field value based on its type
		//nolint:exhaustive // Only string and int are supported for query params currently
		switch field.Type.Kind() {
		case reflect.String:
			elem.Field(i).SetString(paramValue)
		case reflect.Int:
			if intVal, err := strconv.Atoi(paramValue); err == nil {
				elem.Field(i).SetInt(int64(intVal))
			}
		default:
			// Other types are not supported for query params yet
		}
	}
}

// writeErrorResponse writes an error response as JSON.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	writeErrorResponseWithCode(w, statusCode, apierrors.ErrInternal, message, nil)
}

// writeErrorResponseWithCode writes a detailed error response as JSON with code and details.
func writeErrorResponseWithCode(w http.ResponseWriter, statusCode int, code apierrors.ErrorCode, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}

	if len(details) > 0 {
		response["details"] = details
	}

	_ = json.NewEncoder(w).Encode(response)
}

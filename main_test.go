package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanChirp(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "leaves clean text unchanged", input: "hello world", want: "hello world"},
		{name: "replaces profanity regardless of case", input: "Kerfuffle SHARBERT fornax", want: "**** **** ****"},
		{name: "does not replace partial words", input: "kerfuffles fornax!", want: "kerfuffles fornax!"},
		{name: "preserves spacing", input: "one  kerfuffle three", want: "one  **** three"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanChirp(tt.input); got != tt.want {
				t.Errorf("cleanChirp(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandlerValidateChirp(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "accepts and cleans chirp", body: `{"body":"This is Kerfuffle"}`, wantStatus: http.StatusOK, wantBody: `{"cleaned_body":"This is ****"}`},
		{name: "rejects malformed JSON", body: `{`, wantStatus: http.StatusBadRequest, wantBody: ""},
		{name: "rejects empty chirp", body: `{"body":""}`, wantStatus: http.StatusBadRequest, wantBody: `{"Error":"invalid body: empty"}`},
		{name: "rejects chirp over 140 characters", body: `{"body":"` + strings.Repeat("a", 141) + `"}`, wantStatus: http.StatusBadRequest, wantBody: `{"Error":"invalid body: too long"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handlerValidateChirp(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("body = %s, want %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRespondWithJSONAndError(t *testing.T) {
	t.Run("JSON response", func(t *testing.T) {
		rec := httptest.NewRecorder()
		respondWithJSON(rec, http.StatusCreated, map[string]string{"message": "created"})

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("response is not JSON: %v", err)
		}
		if got["message"] != "created" {
			t.Errorf("message = %q, want created", got["message"])
		}
	})

	t.Run("error response", func(t *testing.T) {
		rec := httptest.NewRecorder()
		respondWithError(rec, http.StatusBadRequest, "bad input")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != `{"Error":"bad input"}` {
			t.Errorf("body = %s, want error JSON", got)
		}
	})
}

func TestHandlerReadiness(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerReadiness(rec, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if got := rec.Body.String(); got != "OK" {
		t.Errorf("body = %q, want OK", got)
	}
}

func TestMetricsHandlers(t *testing.T) {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(7)

	metricsRec := httptest.NewRecorder()
	cfg.handlerMetrics(metricsRec, httptest.NewRequest(http.MethodGet, "/admin/metrics", nil))
	if metricsRec.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want %d", metricsRec.Code, http.StatusOK)
	}
	if got := metricsRec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("metrics Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if !strings.Contains(metricsRec.Body.String(), "visited 7 times") {
		t.Errorf("metrics body does not include visit count: %q", metricsRec.Body.String())
	}

	resetRec := httptest.NewRecorder()
	cfg.handlerMetricsReset(resetRec, httptest.NewRequest(http.MethodPost, "/admin/reset", nil))
	if resetRec.Code != http.StatusOK {
		t.Errorf("reset status = %d, want %d", resetRec.Code, http.StatusOK)
	}
	if got := cfg.fileserverHits.Load(); got != 0 {
		t.Errorf("visit count = %d, want 0", got)
	}
	if got := resetRec.Body.String(); got != "Visits have been reset to 0" {
		t.Errorf("reset body = %q, want confirmation", got)
	}
}

func TestMiddlewareMetricsInc(t *testing.T) {
	cfg := apiConfig{}
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	cfg.middlewareMetricsInc(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/", nil))

	if !nextCalled {
		t.Error("next handler was not called")
	}
	if got := cfg.fileserverHits.Load(); got != 1 {
		t.Errorf("visit count = %d, want 1", got)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

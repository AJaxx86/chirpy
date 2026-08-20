package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AJaxx86/chirpy/internal/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
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

func TestHandlerCreateChirp(t *testing.T) {
	t.Run("accepts, cleans, and creates a chirp", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create mock database: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
		chirpID := uuid.MustParse("0e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
		createdAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery("INSERT INTO chirps").
			WithArgs("This is ****", userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
				AddRow(chirpID, createdAt, createdAt, "This is ****", userID))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		cfg.handlerCreateChirp(rec, httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(`{"body":"This is Kerfuffle","user_id":"3e9f4e1f-3a2a-4d41-a31f-616b84dcd068"}`)))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		var response struct {
			Body   string `json:"body"`
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Body != "This is ****" || response.UserID != userID.String() {
			t.Errorf("response = %+v, want cleaned body and user ID", response)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("database expectations: %v", err)
		}
	})

	for _, tt := range []struct {
		name, body, wantBody string
		wantStatus           int
	}{
		{name: "rejects malformed JSON", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "rejects empty chirp", body: `{"body":""}`, wantStatus: http.StatusBadRequest, wantBody: `{"Error":"invalid body: empty"}`},
		{name: "rejects chirp over 140 characters", body: `{"body":"` + strings.Repeat("a", 141) + `"}`, wantStatus: http.StatusBadRequest, wantBody: `{"Error":"invalid body: too long"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			(&apiConfig{}).handlerCreateChirp(rec, httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
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

func TestHandlerMetrics(t *testing.T) {
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

}

func TestHandlerCreateUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := apiConfig{db: database.New(db)}
	createdAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("INSERT INTO users").
		WithArgs("person@example.com", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password"}).
			AddRow("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068", createdAt, createdAt, "person@example.com", "hashed_secret"))

	rec := httptest.NewRecorder()
	cfg.handlerCreateUser(rec, httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"email":"person@example.com","password":"secretpassword"}`)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var response struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Email != "person@example.com" {
		t.Errorf("email = %q, want person@example.com", response.Email)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database expectations: %v", err)
	}
}

func TestHandlerCreateUserRejectsInvalidEmail(t *testing.T) {
	cfg := apiConfig{}
	rec := httptest.NewRecorder()
	cfg.handlerCreateUser(rec, httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"email":"invalid"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"Error":"invalid email"}` {
		t.Errorf("body = %s, want invalid-email error", got)
	}
}

func TestHandlerReset(t *testing.T) {
	t.Setenv("PLATFORM", "dev")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("TRUNCATE TABLE users").WillReturnResult(sqlmock.NewResult(0, 0))

	cfg := apiConfig{db: database.New(db)}
	cfg.fileserverHits.Store(7)
	rec := httptest.NewRecorder()
	cfg.handlerReset(rec, httptest.NewRequest(http.MethodPost, "/admin/reset", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := cfg.fileserverHits.Load(); got != 0 {
		t.Errorf("visit count = %d, want 0", got)
	}
	if got := rec.Body.String(); got != "Visits reset to 0, all users removed." {
		t.Errorf("body = %q, want reset confirmation", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database expectations: %v", err)
	}
}

func TestHandlerResetForbiddenOutsideDevelopment(t *testing.T) {
	t.Setenv("PLATFORM", "production")
	cfg := apiConfig{}
	rec := httptest.NewRecorder()
	cfg.handlerReset(rec, httptest.NewRequest(http.MethodPost, "/admin/reset", nil))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
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

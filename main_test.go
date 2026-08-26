package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AJaxx86/chirpy/internal/auth"
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
	secret := "test-secret"
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	token, err := auth.MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error = %v", err)
	}

	t.Run("accepts, cleans, and creates a chirp", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create mock database: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		chirpID := uuid.MustParse("0e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
		createdAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery("INSERT INTO chirps").
			WithArgs("This is ****", userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
				AddRow(chirpID, createdAt, createdAt, "This is ****", userID))

		cfg := apiConfig{db: database.New(db), secret: secret}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(`{"body":"This is Kerfuffle"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		cfg.handlerCreateChirp(rec, req)

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
			req := httptest.NewRequest(http.MethodPost, "/api/chirps", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			(&apiConfig{secret: secret}).handlerCreateChirp(rec, req)
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
			AddRow("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068", createdAt, createdAt, "person@example.com", "hashed_secret", false))

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

func TestHandlerLogin(t *testing.T) {
	password := "correct-password"
	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	t.Run("successful login", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM users WHERE email = \\$1").
			WithArgs("user@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
				AddRow(userID, now, now, "user@example.com", hashedPassword, false))

		mock.ExpectQuery("INSERT INTO refresh_tokens").
			WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"token", "created_at", "updated_at", "user_id", "expires_at", "revoked_at"}).
				AddRow("refresh-token-val", now, now, userID, now.Add(60*24*time.Hour), nil))

		cfg := apiConfig{
			db:                       database.New(db),
			secret:                   "test-secret",
			jwtExpireInSeconds:       3600,
			refreshTokenExpireInDays: 60,
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"user@example.com","password":"correct-password"}`))
		cfg.handlerLogin(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var resp struct {
			ID           uuid.UUID `json:"id"`
			Email        string    `json:"email"`
			Token        string    `json:"token"`
			RefreshToken string    `json:"refresh_token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.ID != userID || resp.Email != "user@example.com" || resp.Token == "" || resp.RefreshToken == "" {
			t.Errorf("unexpected response: %+v", resp)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations: %v", err)
		}
	})

	t.Run("rejects invalid json", func(t *testing.T) {
		cfg := apiConfig{}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{`))
		cfg.handlerLogin(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("rejects unknown user", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM users WHERE email = \\$1").
			WithArgs("unknown@example.com").
			WillReturnError(sql.ErrNoRows)

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"unknown@example.com","password":"password"}`))
		cfg.handlerLogin(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("rejects incorrect password", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM users WHERE email = \\$1").
			WithArgs("user@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "email", "hashed_password", "is_chirpy_red"}).
				AddRow(userID, now, now, "user@example.com", hashedPassword, false))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"email":"user@example.com","password":"wrong-password"}`))
		cfg.handlerLogin(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestHandlerRefresh(t *testing.T) {
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	validToken := "valid-refresh-token"

	t.Run("successfully generates new JWT", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs(validToken).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", validToken, time.Now().Add(time.Hour), nil))

		cfg := apiConfig{
			db:                 database.New(db),
			secret:             "test-secret",
			jwtExpireInSeconds: 3600,
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		cfg.handlerRefresh(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var resp struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Token == "" {
			t.Error("expected non-empty token")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations: %v", err)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		cfg := apiConfig{}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		cfg.handlerRefresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("token not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("non-existent-token").
			WillReturnError(sql.ErrNoRows)

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		req.Header.Set("Authorization", "Bearer non-existent-token")
		cfg.handlerRefresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("revoked token", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		revokedTime := time.Now().Add(-10 * time.Minute)
		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("revoked-token").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", "revoked-token", time.Now().Add(time.Hour), revokedTime))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		req.Header.Set("Authorization", "Bearer revoked-token")
		cfg.handlerRefresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		expiredTime := time.Now().Add(-1 * time.Hour)
		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("expired-token").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", "expired-token", expiredTime, nil))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
		req.Header.Set("Authorization", "Bearer expired-token")
		cfg.handlerRefresh(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestHandlerRevoke(t *testing.T) {
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	now := time.Now()

	t.Run("successfully revokes token", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("active-token").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", "active-token", now.Add(time.Hour), nil))

		mock.ExpectQuery("UPDATE refresh_tokens").
			WithArgs("active-token").
			WillReturnRows(sqlmock.NewRows([]string{"token", "created_at", "updated_at", "user_id", "expires_at", "revoked_at"}).
				AddRow("active-token", now, now, userID, now.Add(time.Hour), now))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/revoke", nil)
		req.Header.Set("Authorization", "Bearer active-token")
		cfg.handlerRevoke(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations: %v", err)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		cfg := apiConfig{}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/revoke", nil)
		cfg.handlerRevoke(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("token not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("missing-token").
			WillReturnError(sql.ErrNoRows)

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/revoke", nil)
		req.Header.Set("Authorization", "Bearer missing-token")
		cfg.handlerRevoke(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("token already revoked", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("already-revoked").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", "already-revoked", now.Add(time.Hour), now.Add(-5*time.Minute)))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/revoke", nil)
		req.Header.Set("Authorization", "Bearer already-revoked")
		cfg.handlerRevoke(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("token already expired", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT (.+) FROM refresh_tokens JOIN users").
			WithArgs("already-expired").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "token", "expires_at", "revoked_at"}).
				AddRow(userID, "user@example.com", "already-expired", now.Add(-1*time.Hour), nil))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/revoke", nil)
		req.Header.Set("Authorization", "Bearer already-expired")
		cfg.handlerRevoke(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestHandlerGetChirps(t *testing.T) {
	chirpID := uuid.MustParse("0e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	t.Run("returns list of chirps", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT id, created_at, updated_at, body, user_id FROM chirps ORDER BY created_at ASC").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
				AddRow(chirpID, now, now, "Hello world", userID))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chirps", nil)
		cfg.handlerGetChirps(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var chirps []struct {
			ID   uuid.UUID `json:"id"`
			Body string    `json:"body"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &chirps); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(chirps) != 1 || chirps[0].Body != "Hello world" {
			t.Errorf("unexpected chirps response: %+v", chirps)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations: %v", err)
		}
	})

	t.Run("handles db query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT id, created_at, updated_at, body, user_id FROM chirps").
			WillReturnError(errors.New("db connection failure"))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chirps", nil)
		cfg.handlerGetChirps(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandlerGetChirpByID(t *testing.T) {
	chirpID := uuid.MustParse("0e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	t.Run("returns chirp by id", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT id, created_at, updated_at, body, user_id FROM chirps WHERE id = \\$1").
			WithArgs(chirpID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "body", "user_id"}).
				AddRow(chirpID, now, now, "Specific chirp", userID))

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chirps/"+chirpID.String(), nil)
		req.SetPathValue("chirpID", chirpID.String())
		cfg.handlerGetChirpByID(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var chirp struct {
			ID   uuid.UUID `json:"id"`
			Body string    `json:"body"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &chirp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if chirp.ID != chirpID || chirp.Body != "Specific chirp" {
			t.Errorf("unexpected chirp: %+v", chirp)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations: %v", err)
		}
	})

	t.Run("returns 400 on invalid uuid", func(t *testing.T) {
		cfg := apiConfig{}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chirps/invalid-id", nil)
		req.SetPathValue("chirpID", "invalid-id")
		cfg.handlerGetChirpByID(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns 404 when chirp not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		mock.ExpectQuery("SELECT id, created_at, updated_at, body, user_id FROM chirps WHERE id = \\$1").
			WithArgs(chirpID).
			WillReturnError(sql.ErrNoRows)

		cfg := apiConfig{db: database.New(db)}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chirps/"+chirpID.String(), nil)
		req.SetPathValue("chirpID", chirpID.String())
		cfg.handlerGetChirpByID(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

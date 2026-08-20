package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.MustParse("3e9f4e1f-3a2a-4d41-a31f-616b84dcd068")
	secret := "correct-secret"

	tests := []struct {
		name           string
		expiresIn      time.Duration
		validationKey  string
		wantValidation bool
	}{
		{
			name:           "valid token returns its user ID",
			expiresIn:      time.Hour,
			validationKey:  secret,
			wantValidation: true,
		},
		{
			name:           "expired token is rejected",
			expiresIn:      -time.Hour,
			validationKey:  secret,
			wantValidation: false,
		},
		{
			name:           "token signed with another secret is rejected",
			expiresIn:      time.Hour,
			validationKey:  "wrong-secret",
			wantValidation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := MakeJWT(userID, secret, tt.expiresIn)
			if err != nil {
				t.Fatalf("MakeJWT() error = %v", err)
			}

			gotUserID, err := ValidateJWT(token, tt.validationKey)
			if tt.wantValidation {
				if err != nil {
					t.Fatalf("ValidateJWT() error = %v", err)
				}
				if gotUserID != userID {
					t.Errorf("ValidateJWT() user ID = %s, want %s", gotUserID, userID)
				}
				return
			}

			if err == nil {
				t.Errorf("ValidateJWT() error = nil, want rejection")
			}
		})
	}
}

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		headers   http.Header
		wantToken string
		wantErr   bool
	}{
		{
			name: "valid bearer token",
			headers: http.Header{
				"Authorization": []string{"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
			},
			wantToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantErr:   false,
		},
		{
			name: "valid bearer token with extra whitespace",
			headers: http.Header{
				"Authorization": []string{"Bearer   my-token-123   "},
			},
			wantToken: "my-token-123",
			wantErr:   false,
		},
		{
			name:      "missing authorization header",
			headers:   http.Header{},
			wantToken: "",
			wantErr:   true,
		},
		{
			name: "empty bearer token",
			headers: http.Header{
				"Authorization": []string{"Bearer "},
			},
			wantToken: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToken, err := GetBearerToken(tt.headers)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetBearerToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotToken != tt.wantToken {
				t.Errorf("GetBearerToken() gotToken = %q, want %q", gotToken, tt.wantToken)
			}
		})
	}
}

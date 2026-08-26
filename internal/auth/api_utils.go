package auth

import (
	"net/http"
	"strings"
	"errors"
)


func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("no Authorization header")
	}
	apiKey := strings.TrimPrefix(authHeader, "ApiKey ")
	if apiKey == "" {
		return "", errors.New("no ApiKey in Authorization header")
	}
	return apiKey, nil
}

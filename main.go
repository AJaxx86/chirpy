package main

import (
	"os"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"github.com/joho/godotenv"
	"github.com/AJaxx86/chirpy/internal/database"
	"github.com/AJaxx86/chirpy/internal/auth"
	_ "github.com/lib/pq"
	"github.com/google/uuid"
	"time"
	"errors"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db *database.Queries
	secret string
	jwtExpireInSeconds int
	refreshTokenExpireInDays int
}


func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	secretToken := os.Getenv("SECRET")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("Error loading DB: %s", err)
		return
	}

	apiCfg := apiConfig{db: database.New(db), secret: secretToken, jwtExpireInSeconds: 3600, refreshTokenExpireInDays: 60}
	router := http.NewServeMux()
	server := &http.Server{
		Addr: ":8080",
		Handler: router,
	}

	handler := http.FileServer(http.Dir("."))
	router.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", handler)))
	router.HandleFunc("GET /api/healthz", handlerReadiness)
	router.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	router.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	router.HandleFunc("PUT /api/users", apiCfg.handlerUpdateUser)
	router.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	router.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	router.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)
	router.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
	router.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDeleteChirp)
	router.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirpByID)
	router.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	router.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	server.ListenAndServe()
}


func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string
	}
	respondWithJSON(w, code, errorResponse{Error: msg})
}


func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}


func cleanChirp(chirp string) string {
	profanity := []string{"kerfuffle", "sharbert", "fornax"}
	words := strings.Split(chirp, " ")

	for i, w := range(words) {
		for _, prof := range(profanity) {
			if strings.EqualFold(w, prof) {
				words[i] = "****"
			}
		}
	}

	cleaned := strings.Join(words, " ")
	return cleaned
}


func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte("OK")
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}


func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type loginRequest struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	type response struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email string `json:"email"`
		AccessToken string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	req := &loginRequest{}
	err := decoder.Decode(req)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := cfg.db.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email")
		return
	}

	passwordCorrect, err := auth.CheckPasswordHash(req.Password, user.HashedPassword)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !passwordCorrect {
		respondWithError(w, http.StatusUnauthorized, "Incorrect password")
		return
	}

	jwtToken, err := auth.MakeJWT(user.ID, cfg.secret, time.Duration(cfg.jwtExpireInSeconds) * time.Second)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	refreshToken := auth.MakeRefreshToken()
	rtParams := database.StoreRefreshTokenParams{
		Token: refreshToken,
		UserID: user.ID,
		ExpiresAt: time.Now().Add(time.Duration(cfg.refreshTokenExpireInDays) * 24 * time.Hour),
	}
	_, err = cfg.db.StoreRefreshToken(r.Context(), rtParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := response{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
		AccessToken: jwtToken,
		RefreshToken: refreshToken,
	}
	respondWithJSON(w, http.StatusOK, res)
}


func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type requestBody struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	type responseBody struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email string `json:"email"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldn't parse auth token")
		return
	}

	req := &requestBody{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(req); err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}

	newEmail := req.Email
	newPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	params := database.UpdateUserInfoParams{
		ID: userID,
		Email: newEmail,
		HashedPassword: newPassword,
	}
	newInfo, err := cfg.db.UpdateUserInfo(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := responseBody{
		ID: newInfo.ID,
		CreatedAt: newInfo.CreatedAt,
		UpdatedAt: newInfo.UpdatedAt,
		Email: newInfo.Email,
	}
	respondWithJSON(w, http.StatusOK, res)
}


func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type response struct {
		NewJWT string `json:"token"`
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldn't parse auth token")
		return
	}

	tokenDetails, err := cfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	if tokenDetails.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "refresh token revoked")
		return
	}
	if time.Now().After(tokenDetails.ExpiresAt) {
		respondWithError(w, http.StatusUnauthorized, "refresh token expired")
		return
	}

	jwt, err := auth.MakeJWT(tokenDetails.UserID, cfg.secret, time.Duration(cfg.jwtExpireInSeconds) * time.Second)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := response{NewJWT: jwt}
	respondWithJSON(w, http.StatusOK, res)
}


func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "couldn't parse auth token")
		return
	}

	tokenDetails, err := cfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokenDetails.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "refresh token already revoked")
		return
	}
	if time.Now().After(tokenDetails.ExpiresAt) {
		respondWithError(w, http.StatusUnauthorized, "refresh token already expired")
		return
	}

	_, err = cfg.db.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	type fixedChirp struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	chirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fixedChirps := make([]fixedChirp, len(chirps))
	for i, chirp := range chirps {
		fixedChirps[i] = fixedChirp{
			ID: chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body: chirp.Body,
			UserID: chirp.UserID,
		}
	}
	respondWithJSON(w, http.StatusOK, fixedChirps)
}


func (cfg *apiConfig) handlerGetChirpByID(w http.ResponseWriter, r *http.Request) {
	type chirpRes struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	res := chirpRes{
		ID: chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body: chirp.Body,
		UserID: chirp.UserID,
	}
	respondWithJSON(w, http.StatusOK, res)
}


func (cfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Body string `json:"body"`
	}
	type response struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	bearerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}
	tokenID, err := auth.ValidateJWT(bearerToken, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}

	decoder := json.NewDecoder(r.Body)
	req := &request{}
	err = decoder.Decode(req)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.Body) == 0 {
		respondWithError(w, http.StatusBadRequest, "invalid body: empty")
		return
	}
	if len(req.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "invalid body: too long")
		return
	}

	params := database.CreateChirpParams{
		Body: cleanChirp(req.Body),
		UserID: tokenID,
	}

	chirp, err := cfg.db.CreateChirp(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := response{
		ID: chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body: chirp.Body,
		UserID: chirp.UserID,
	}
	respondWithJSON(w, http.StatusCreated, res)
}


func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "chirp doesn't exist")
		return
	}

	if userID != chirp.UserID {
		respondWithError(w, http.StatusForbidden, "attempted to delete someone else's chirp")
		return
	}

	if err := cfg.db.DeleteChirpByID(r.Context(), chirpID); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	type response struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email string `json:"email"`
	}

	req := &request{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(req); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Email == "" || !strings.Contains(req.Email, "@") {
		respondWithError(w, http.StatusBadRequest, "invalid email")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	params := database.CreateUserParams{
		Email: req.Email,
		HashedPassword: hashedPassword,
	}
	user, err := cfg.db.CreateUser(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := response{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}

	respondWithJSON(w, http.StatusCreated, res)
}


func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	visits := cfg.fileserverHits.Load()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)

	html := `<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
	</html>`
	body := []byte(fmt.Sprintf(html, visits))
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}


func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("PLATFORM") != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	cfg.fileserverHits.Swap(0)

	if err := cfg.db.ClearUsersTable(r.Context()); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	body := []byte("Visits reset to 0, all users removed.")
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}


func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

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
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db *database.Queries
}


func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("Error loading DB: %s", err)
		return
	}

	apiCfg := apiConfig{db: database.New(db)}
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
	router.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)
	router.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
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

	res := response{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	respondWithJSON(w, http.StatusOK, res)
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
		UserID uuid.UUID `json:"user_id"`
	}
	type response struct {
		ID uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	req := &request{}
	err := decoder.Decode(req)
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
		UserID: req.UserID,
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
		Password string `json:"password"`
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
		Password: req.Password,
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

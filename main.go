package main

import (
	"net/http"
	"fmt"
	"strings"
	"sync/atomic"
	"encoding/json"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}


func main() {
	apiCfg := apiConfig{}
	router := http.NewServeMux()
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	handler := http.FileServer(http.Dir("."))
	router.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", handler)))
	router.HandleFunc("GET /api/healthz", handlerReadiness)
	router.HandleFunc("POST /api/validate_chirp", handlerValidateChirp)
	router.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	router.HandleFunc("POST /admin/reset", apiCfg.handlerMetricsReset)
	server.ListenAndServe()
}


func handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Body string `json:"body"`
	}
	type response struct {
		CleanedBody string `json:"cleaned_body"`
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

	cleanedBody := cleanChirp(req.Body)
	respondWithJSON(w, http.StatusOK, response{CleanedBody: cleanedBody})
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


func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte("OK")
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
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


func (cfg *apiConfig) handlerMetricsReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Swap(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte("Visits have been reset to 0")
	_, err := w.Write(body)
	if err != nil {
		fmt.Sprintf("%s", err)
	}
}


func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

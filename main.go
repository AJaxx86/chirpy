package main

import (
	"net/http"
	"fmt"
	"sync/atomic"
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
	router.HandleFunc("GET /healthz", handlerReadiness)
	router.HandleFunc("GET /metrics", apiCfg.handlerMetrics)
	router.HandleFunc("POST /reset", apiCfg.handlerMetricsReset)
	server.ListenAndServe()
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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte(fmt.Sprintf("Hits: %v", visits))
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}


func (cfg *apiConfig) handlerMetricsReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Swap(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte(fmt.Sprintf("Hits: %v", 0))
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
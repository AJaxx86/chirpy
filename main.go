package main

import (
	"net/http"
	"fmt"
)


func main() {
	router := http.NewServeMux()
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	handler := http.FileServer(http.Dir("."))
	router.Handle("/app/", http.StripPrefix("/app", handler))
	router.HandleFunc("/healthz", readinessHandler)
	server.ListenAndServe()
}


func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)

	body := []byte("OK")
	_, err := w.Write(body)
	if err != nil {
		fmt.Printf("%s", err)
	}
}

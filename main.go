package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type apiConfig struct {
	filerserverHits int
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.filerserverHits++
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Content-type: text/plain; charset=utf8"))
	w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.filerserverHits)))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.filerserverHits = 0
}

func middlewareCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Content-type: text/plain; charset=utf8"))
	w.Write([]byte("OK"))
}

func main() {
	cfg := new(apiConfig)
	fs := cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	r := chi.NewRouter()
	r.Handle("/app/*", fs)
	r.Handle("/app", fs)
	r.Get("/healthz", healthzHandler)
	r.Get("/metrics", cfg.metricsHandler)
	r.HandleFunc("/reset", cfg.resetHandler)
	corsr := middlewareCors(r)

	var server http.Server
	server.Handler = corsr
	server.Addr = "localhost:8080"
	log.Println("Chirpy listening and serving at", server.Addr)
	log.Fatalf(server.ListenAndServe().Error())
	return
}

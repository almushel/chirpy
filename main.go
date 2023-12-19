package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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
	w.Header().Set("Content-type", "text/html; charset=utf8")
	body := fmt.Sprintf(
		`<html>

<body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
</body>

</html>
`,
		cfg.filerserverHits)
	w.Write([]byte(body))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.filerserverHits = 0
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	body, _ := json.Marshal(errorResponse{
		Error: msg,
	})

	w.WriteHeader(code)
	w.Write(body)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.WriteHeader(code)
	body, _ := json.Marshal(payload)
	w.Write(body)
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	profaneWords := [3]string{
		"kerfuffle", "sharbert", "fornax",
	}

	type parameters struct {
		Body string `json:"body"`
	}
	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)

	if err != nil {
		log.Printf("Error decoding request: %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	} else if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	rb := returnVals{CleanedBody: params.Body}
	for _, word := range profaneWords {
		lower := strings.ToLower(rb.CleanedBody)
		i := strings.Index(lower, word)
		if i >= 0 {
			rb.CleanedBody = rb.CleanedBody[:i] + "****" + rb.CleanedBody[i+len(word):]
		}
	}

	respondWithJSON(w, 200, rb)
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
	w.Write([]byte("OK"))
}

func main() {
	cfg := new(apiConfig)

	r := chi.NewRouter()
	fs := cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	r.Handle("/app/*", fs)
	r.Handle("/app", fs)

	apiRouter := chi.NewRouter()
	apiRouter.Get("/healthz", healthzHandler)
	apiRouter.HandleFunc("/reset", cfg.resetHandler)
	apiRouter.Post("/validate_chirp", validateHandler)
	r.Mount("/api", apiRouter)

	adminRouter := chi.NewRouter()
	adminRouter.Get("/metrics", cfg.metricsHandler)
	r.Mount("/admin", adminRouter)

	var server http.Server
	server.Handler = middlewareCors(r)
	server.Addr = "localhost:8080"
	log.Println("Chirpy listening and serving at", server.Addr)
	log.Fatalf(server.ListenAndServe().Error())
	return
}

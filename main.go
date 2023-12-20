package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	. "github.com/almushel/chirpy/internal/chirpydb"
)

type apiConfig struct {
	filerserverHits int
	db              *DB
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

func (cfg *apiConfig) postChirpsHandler(w http.ResponseWriter, r *http.Request) {
	profaneWords := [3]string{
		"kerfuffle", "sharbert", "fornax",
	}
	type parameters struct {
		Body string `json:"body"`
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)

	if err != nil {
		log.Printf("(decade(params)) %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	} else if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	rb, err := cfg.db.CreateChirp(params.Body)
	if err != nil {
		log.Printf("(db.CreateChirp) %s", err)
		respondWithError(w, 500, "DB failed to create chirp")
		return
	}

	for _, word := range profaneWords {
		lower := strings.ToLower(rb.Body)
		i := strings.Index(lower, word)
		if i >= 0 {
			rb.Body = rb.Body[:i] + "****" + rb.Body[i+len(word):]
		}
	}

	respondWithJSON(w, 201, rb)
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	rb, err := cfg.db.GetChirps()
	if err != nil {
		log.Printf("(db.loadDB) %s", err)
		respondWithError(w, 500, "Failed to load chirp database")
		return
	}

	idStr := chi.URLParam(r, "chirpID")
	if len(idStr) > 0 {
		id, _ := strconv.Atoi(idStr)
		if id > len(rb) {
			respondWithError(w, 404, fmt.Sprintf("Chirp %d not found", id))
			return
		}
		respondWithJSON(w, 200, rb[id-1])
		return
	}

	respondWithJSON(w, 200, rb)
}

func (cfg *apiConfig) postUsersHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)

	if err != nil {
		log.Printf("(decade(params)) %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	}

	rb, err := cfg.db.CreateUser(params.Email)
	if err != nil {
		log.Printf("(db.CreateUser) %s", err)
		respondWithError(w, 500, "Failed to create user")
		return
	}

	respondWithJSON(w, 201, rb)
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
	dbg := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()
	if *dbg {
		os.Remove("database.json")
	}

	cfg := new(apiConfig)
	cfg.db, _ = NewDB("database.json")

	r := chi.NewRouter()
	fs := cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	r.Handle("/app/*", fs)
	r.Handle("/app", fs)

	apiRouter := chi.NewRouter()
	apiRouter.Get("/healthz", healthzHandler)
	apiRouter.HandleFunc("/reset", cfg.resetHandler)
	apiRouter.Post("/chirps", cfg.postChirpsHandler)
	apiRouter.Get("/chirps", cfg.getChirpsHandler)
	apiRouter.Post("/users", cfg.postUsersHandler)
	apiRouter.Get("/chirps/{chirpID}", cfg.getChirpsHandler)
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

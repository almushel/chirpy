package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	. "github.com/almushel/chirpy/internal/chirpapi"
)

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

func parseEnv() {
	buf, err := os.ReadFile(".env")
	if err != nil {
		log.Println(err)
		return
	}

	envStr := string(buf)
	lines := strings.Split(envStr, "\n")
	for _, line := range lines {
		ev, param, found := strings.Cut(line, "=")
		if found != true {
			log.Println("Failed to parse .env line:", line)
			continue
		}
		os.Setenv(strings.TrimSpace(ev), strings.TrimSpace(param))
	}
}

func InitServer(cfg *ApiConfig, addr string) (*http.Server, error) {
	var server http.Server

	r := chi.NewRouter()
	fs := cfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	r.Handle("/app/*", fs)
	r.Handle("/app", fs)

	apiRouter := chi.NewRouter()
	apiRouter.Get("/healthz", healthzHandler)
	apiRouter.HandleFunc("/reset", cfg.ResetHandler)

	apiRouter.Post("/chirps", cfg.PostChirpsHandler)
	apiRouter.Get("/chirps", cfg.GetChirpsHandler)
	apiRouter.Get("/chirps/{chirpID}", cfg.GetChirpsHandler)
	apiRouter.Delete("/chirps/{chirpID}", cfg.DeleteChirpsHandler)

	apiRouter.Post("/users", cfg.PostUsersHandler)
	apiRouter.Put("/users", cfg.PutUsersHandler)

	apiRouter.Post("/login", cfg.PostLoginHandler)
	apiRouter.Post("/refresh", cfg.PostRefreshHandler)
	apiRouter.Post("/revoke", cfg.PostRevokeHandler)

	apiRouter.Post("/polka/webhooks", cfg.PolkaWebhookHandler)

	r.Mount("/api", apiRouter)

	adminRouter := chi.NewRouter()
	adminRouter.Get("/metrics", cfg.MetricsHandler)
	r.Mount("/admin", adminRouter)

	server.Handler = middlewareCors(r)
	server.Addr = addr

	return &server, nil
}

func main() {
	dbg := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()
	if *dbg {
		os.Remove("database.json")
	}

	parseEnv()
	jwt, found := os.LookupEnv("JWT_SECRET")
	if !found {
		log.Println("Failed to load jwt secret from .env")
		return
	}
	pk, found := os.LookupEnv("POLKA_KEY")
	if !found {
		log.Println("Failed to load Polka API key")
		return
	}

	cfg, err := NewChirpAPI("database.json", jwt, pk)
	if err != nil {
		log.Fatalln(err)
	}
	server, err := InitServer(cfg, "localhost:8080")
	log.Println("Chirpy listening and serving at", server.Addr)
	log.Fatalf(server.ListenAndServe().Error())
	return
}

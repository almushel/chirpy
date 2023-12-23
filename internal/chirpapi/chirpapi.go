package chirpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"github.com/almushel/chirpy/internal/chirpydb"
)

type ApiConfig struct {
	filerserverHits int
	db              *chirpydb.DB
	jwtSecret       string
}

func NewChirpAPI(dbPath, jwtSecret string) (*ApiConfig, error) {
	var err error
	result := new(ApiConfig)
	result.db, err = chirpydb.NewDB(dbPath)
	if err != nil {
		return nil, err
	}
	result.jwtSecret = jwtSecret

	return result, nil
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

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.filerserverHits++
		next.ServeHTTP(w, r)
	})
}

func (cfg *ApiConfig) MetricsHandler(w http.ResponseWriter, r *http.Request) {
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

func (cfg *ApiConfig) ResetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.filerserverHits = 0
}

func (cfg *ApiConfig) PostChirpsHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("(decode(params)) %s", err)
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

func (cfg *ApiConfig) GetChirpsHandler(w http.ResponseWriter, r *http.Request) {
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

func (cfg *ApiConfig) PostUsersHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)

	if err != nil {
		log.Printf("(decode(params)) %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	}

	rb, err := cfg.db.CreateUser(params.Email, params.Password)
	if err != nil {
		log.Printf("(db.CreateUser) %s", err)
		respondWithError(w, 500, "Failed to create user")
		return
	}

	respondWithJSON(w, 201, rb)
}

func (cfg *ApiConfig) PutUsersHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	ts := r.Header.Get("Authorization")[len("Bearer "):]
	if len(ts) == 0 {
		log.Println("No Authorization header in request")
		respondWithError(w, 401, "No authorization header")
		return
	}

	token, err := jwt.ParseWithClaims(ts, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) { return []byte(cfg.jwtSecret), nil })
	if err != nil {
		log.Println("jwt.ParseWithClaims()", err)
		respondWithError(w, 401, "Invalid authorization token")
		return
	}
	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		log.Println("token.Claims.GetIssuer()", err)
		respondWithError(w, 401, "Invalid authorization token")
		return
	}
	if issuer != "chirpy-access" {
		respondWithError(w, 401, "Invalid authorization token")
		return
	}
	idStr, err := token.Claims.GetSubject()
	if err != nil {
		log.Println("token.Claims.GetSubject()", err)
		respondWithError(w, 401, "Invalid authorization token")
		return
	}
	id, err := strconv.Atoi(idStr)

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(params)
	if err != nil {
		log.Printf("(decode(params)) %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	}
	var rb chirpydb.User
	rb, err = cfg.db.UpdateUser(id, params.Email, params.Password)

	respondWithJSON(w, 200, rb)
}

func (cfg *ApiConfig) PostLoginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	type response struct {
		chirpydb.User
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)

	if err != nil {
		log.Printf("(decode(params)) %s", err)
		respondWithError(w, 500, "Failed to decode request body")
		return
	}
	var rb response
	rb.User, err = cfg.db.UserLogin(params.Email, params.Password)
	if err != nil {
		log.Printf("db.UserLogin() %s\n", err)
		respondWithError(w, 401, "Invalid email or password")
		return
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    "chirpy-access",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Subject:   fmt.Sprint(rb.User.ID),
		},
	)
	rToken := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    "chirpy-refresh",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * 24 * time.Hour)),
			Subject:   fmt.Sprint(rb.User.ID),
		},
	)

	rb.Token, err = token.SignedString([]byte(cfg.jwtSecret))
	if err == nil {
		rb.RefreshToken, err = rToken.SignedString([]byte(cfg.jwtSecret))
	}
	if err != nil {
		log.Println("token.SignedString()", err)
		respondWithError(w, 500, "Token creation failed")
		return
	}

	respondWithJSON(w, 200, rb)
}

func (cfg *ApiConfig) PostRefreshHandler(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Token string `json:"token"`
	}
	var err error
	defer func() {
		if err != nil {
			log.Println(err)
			respondWithError(w, 401, "Invalid authorization token")
			return
		}
	}()

	ts := r.Header.Get("Authorization")[len("Bearer "):]
	if len(ts) == 0 {
		err = errors.New("No authorization header")
		return
	}
	_, err = cfg.db.GetTokenRevocation(ts)
	if err == nil {
		err = errors.New("Authorization token has been revoked")
		return
	}

	token, err := jwt.ParseWithClaims(ts, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) { return []byte(cfg.jwtSecret), nil })
	if err != nil {
		return
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return
	}
	if issuer != "chirpy-refresh" {
		err = errors.New("Invalid refresh token issuer")
		return
	}

	idStr, err := token.Claims.GetSubject()
	if err != nil {
		return
	}

	var rb response
	rToken := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    "chirpy-access",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * 24 * time.Hour)),
			Subject:   idStr,
		},
	)

	rb.Token, err = rToken.SignedString([]byte(cfg.jwtSecret))
	if err != nil {
		return
	}

	respondWithJSON(w, 200, rb)
}

func (cfg *ApiConfig) PostRevokeHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	defer func() {
		if err != nil {
			log.Println(err.Error())
			respondWithError(w, 401, "Invalid authorization token")
		} else {
			respondWithJSON(w, 200, "")
		}
	}()

	ts := r.Header.Get("Authorization")[len("Bearer "):]
	if len(ts) == 0 {
		err = errors.New("No authorization header")
		return
	}
	_, err = cfg.db.GetTokenRevocation(ts)
	if err == nil {
		// NOTE: Should this be an error???
		err = errors.New("Authorization token has already been revoked")
		return
	}

	token, err := jwt.ParseWithClaims(ts, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) { return []byte(cfg.jwtSecret), nil })
	if err != nil {
		return
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return
	}
	if issuer != "chirpy-refresh" {
		err = errors.New("Invalid token issuer")
		return
	}

	cfg.db.RevokeToken(ts)
}

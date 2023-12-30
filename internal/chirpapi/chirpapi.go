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
	polkaKey        string
}

const (
	AccessIssuer  = "chirpy-access"
	RefreshIssuer = "chirpy-refresh"
)

func NewChirpAPI(dbPath, jwtSecret, polkaKey string) (*ApiConfig, error) {
	var err error
	result := new(ApiConfig)
	result.db, err = chirpydb.NewDB(dbPath)
	if err != nil {
		return nil, err
	}
	result.jwtSecret = jwtSecret
	result.polkaKey = polkaKey

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

func (cfg *ApiConfig) checkAuthorization(tokenString, issuer string) (id int, err error) {
	if len(tokenString) == 0 {
		err = errors.New("No authorization header")
		return
	}

	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) { return []byte(cfg.jwtSecret), nil })
	if err != nil {
		return
	}

	i, err := token.Claims.GetIssuer()
	if i != issuer {
		err = errors.New("Invalid authorization issuer")
		return
	}
	if i == RefreshIssuer && cfg.db.IsTokenRevoked(tokenString) {
		err = errors.New("Refresh token has been revoked")
	}

	expires, err := token.Claims.GetExpirationTime()
	if err != nil {
		return
	}

	if time.Now().After(expires.Time) {
		err = errors.New("Authorization token is expired")
		return
	}

	idStr, err := token.Claims.GetSubject()
	if err != nil {
		return
	}
	id, err = strconv.Atoi(idStr)

	return
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
	type parameters struct {
		Body string `json:"body"`
	}

	var err error
	var code int
	defer func() {
		if err != nil {
			respondWithError(w, code, err.Error())
		}
	}()

	ts := r.Header.Get("Authorization")
	if len(ts) < len("Bearer ") {
		err = errors.New("No authorization header")
		code = 401
		return
	}
	id, err := cfg.checkAuthorization(ts[len("Bearer "):], AccessIssuer)
	if err != nil {
		code = 401
		return
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(params)

	if err != nil {
		code = 400
		return
	} else if len(params.Body) > 140 {
		code = 400
		err = errors.New("Chirp is too long")
		return
	}

	rb, err := cfg.db.CreateChirp(params.Body, id)
	if err != nil {
		code = 500
		return
	}

	profaneWords := [3]string{
		"kerfuffle", "sharbert", "fornax",
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
	chirps, err := cfg.db.GetChirps()
	if err != nil {
		respondWithError(w, 500, "Failed to load chirp database")
		return
	}

	idStr := chi.URLParam(r, "chirpID")
	if len(idStr) > 0 {
		id, _ := strconv.Atoi(idStr)
		if id > len(chirps) {
			respondWithError(w, 404, fmt.Sprintf("Chirp %d not found", id))
			return
		}
		respondWithJSON(w, 200, chirps[id-1])
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort == "desc" {
		for i, e := 0, len(chirps)-1; i < e; i, e = i+1, e-1 {
			chirps[i], chirps[e] = chirps[e], chirps[i]
		}
	}

	var rb []chirpydb.Chirp
	authorIDStr := r.URL.Query().Get("author_id")
	if len(authorIDStr) > 0 {
		authorID, err := strconv.Atoi(authorIDStr)
		if err != nil {
			respondWithError(w, 400, "Invalid author_id")
			return
		}
		for _, chirp := range chirps {
			if chirp.AuthorID == authorID {
				rb = append(rb, chirp)
			}
		}
	} else {
		rb = chirps
	}

	respondWithJSON(w, 200, rb)
}

func (cfg *ApiConfig) DeleteChirpsHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var code int
	defer func() {
		if err != nil {
			respondWithError(w, code, err.Error())
		}
	}()

	ts := r.Header.Get("Authorization")
	if len(ts) < len("Bearer ") {
		err = errors.New("Invalid authorization header")
		code = 401
		return
	}
	id, err := cfg.checkAuthorization(ts[len("Bearer "):], AccessIssuer)
	if err != nil {
		code = 401
		return
	}

	idStr := chi.URLParam(r, "chirpID")
	if len(idStr) == 0 {
		err = errors.New("No chirp ID param")
		code = 404
		return
	}
	chirpID, err := strconv.Atoi(idStr)
	if err != nil {
		code = 500
		return
	}

	chirp, err := cfg.db.GetChirp(chirpID)
	if err != nil {
		code = 404
		return
	}

	if chirp.AuthorID != id {
		err = errors.New("Not authorized chirp author")
		code = 403
		return
	}

	err = cfg.db.DeleteChirp(chirpID)
	if err != nil {
		code = 400
		return
	}

	respondWithJSON(w, 200, "OK")
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
		respondWithError(w, 400, "Failed to decode request body")
		return
	}

	rb, err := cfg.db.CreateUser(params.Email, params.Password)
	if err != nil {
		respondWithError(w, 500, err.Error())
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
	id, err := cfg.checkAuthorization(ts, AccessIssuer)
	if err != nil {
		respondWithError(w, 401, err.Error())
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(params)
	if err != nil {
		respondWithError(w, 400, "Failed to decode request body")
		return
	}
	var rb chirpydb.User

	rb, err = cfg.db.UpdateUser(id, map[string]string{"email": params.Email, "password": params.Password})

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
		respondWithError(w, 400, "Failed to decode request body")
		return
	}

	var rb response
	rb.User, err = cfg.db.UserLogin(params.Email, params.Password)
	if err != nil {
		respondWithError(w, 401, "Invalid email or password")
		return
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    AccessIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Subject:   fmt.Sprint(rb.User.ID),
		},
	)
	rToken := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    RefreshIssuer,
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
			respondWithError(w, 401, "Invalid authorization token")
			return
		}
	}()

	ts := r.Header.Get("Authorization")
	if len(ts) < len("Bearer ") {
		err = errors.New("Invalid authorization header")
		return
	}
	id, err := cfg.checkAuthorization(ts[len("Bearer "):], AccessIssuer)
	if err != nil {
		return
	}

	var rb response
	rToken := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    AccessIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * 24 * time.Hour)),
			Subject:   fmt.Sprintf("%d", id),
		},
	)

	rb.Token, err = rToken.SignedString([]byte(cfg.jwtSecret))
	if err != nil {
		log.Println("(PostRefreshHandler) Creation of signed access token string failed")
		return
	}

	respondWithJSON(w, 200, rb)
}

func (cfg *ApiConfig) PostRevokeHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	defer func() {
		if err != nil {
			respondWithError(w, 401, "Invalid authorization token")
		} else {
			respondWithJSON(w, 200, "OK")
		}
	}()

	ts := r.Header.Get("Authorization")
	if len(ts) < len("Bearer ") {
		err = errors.New("Invalid authorization header")
		return
	}
	_, err = cfg.checkAuthorization(ts[len("Bearer "):], RefreshIssuer)
	if err != nil {
		return
	}

	cfg.db.RevokeToken(ts)
}

func (cfg *ApiConfig) PolkaWebhookHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string         `json:"event"`
		Data  map[string]int `json:"data"`
	}

	auth := r.Header.Get("Authorization")
	if auth[min(len("Apikey "), len(auth)):] != cfg.polkaKey {
		respondWithError(w, 401, "Unauthorized")
		return
	}

	params := new(parameters)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(params)
	if err != nil {
		respondWithError(w, 400, "Invalid request body")
		return
	}

	if params.Event == "user.upgraded" {
		userID, ok := params.Data["user_id"]
		if !ok {
			respondWithError(w, 404, "User ID not found")
			return
		}
		cfg.db.UpdateUser(userID, map[string]string{"is_chirpy_red": "true"})
	}
	respondWithJSON(w, 200, "")
}

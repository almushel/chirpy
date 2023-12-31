package chirpydb

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Chirp struct {
	ID       int    `json:"id"`
	AuthorID int    `json:"author_id"`
	Body     string `json:"body"`
}

type User struct {
	ID          int    `json:"id"`
	Email       string `json:"email"`
	IsChirpyRed bool   `json:"is_chirpy_red"`
}

type dbUser struct {
	User
	PWH []byte `json:"pwh"`
}

type DB struct {
	path string
	mux  *sync.RWMutex

	chirpID, userID int
}

type DBStructure struct {
	Chirps      map[int]Chirp
	Users       map[int]dbUser
	Emails      map[string]int
	Revocations map[string]time.Time
}

func NewDB(path string) (*DB, error) {
	result := new(DB)
	result.mux = new(sync.RWMutex)
	result.chirpID = 1
	result.userID = 1
	err := result.initDB(path)

	return result, err
}

func (db *DB) initDB(path string) error {
	db.mux = new(sync.RWMutex)
	db.mux.Lock()
	defer db.mux.Unlock()

	db.path = path
	_, err := os.Stat(db.path)
	if errors.Is(err, os.ErrNotExist) {
		dbs := new(DBStructure)
		buff, err := json.Marshal(dbs)
		if err != nil {
			return err
		}
		return os.WriteFile(db.path, buff, 0666)
	}

	return err
}

func (db *DB) loadDB() (DBStructure, error) {
	db.mux.RLock()
	defer db.mux.RUnlock()

	var dbs DBStructure
	buff, err := os.ReadFile(db.path)
	if err != nil {
		return dbs, err
	}
	err = json.Unmarshal(buff, &dbs)
	return dbs, err
}

func (db *DB) writeDB(dbs DBStructure) error {
	db.mux.Lock()
	defer db.mux.Unlock()

	buff, err := json.Marshal(dbs)
	if err != nil {
		return err
	}
	err = os.WriteFile(db.path, buff, 0)
	return err
}

func (db *DB) CreateChirp(msg string, authorID int) (Chirp, error) {
	var result Chirp

	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	if len(dbs.Chirps) == 0 {
		dbs.Chirps = make(map[int]Chirp)
	}
	result = Chirp{
		ID:       db.chirpID,
		AuthorID: authorID,
		Body:     msg,
	}
	dbs.Chirps[result.ID] = result
	db.chirpID++

	err = db.writeDB(dbs)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (db *DB) GetChirp(id int) (Chirp, error) {
	dbs, err := db.loadDB()
	if err != nil {
		return Chirp{}, err
	}

	result, ok := dbs.Chirps[id]
	if !ok {
		return result, errors.New("Invalid chirp ID")
	}

	return result, nil
}

func (db *DB) DeleteChirp(id int) (err error) {
	dbs, err := db.loadDB()
	if err != nil {
		return
	}

	delete(dbs.Chirps, id)
	err = db.writeDB(dbs)

	return err
}

func (db *DB) GetChirps() ([]Chirp, error) {
	var chirps []Chirp
	dbs, err := db.loadDB()
	if err != nil {
		return chirps, err
	}

	for i := 1; i < db.chirpID; i++ {
		chirp, ok := dbs.Chirps[i]
		if ok {
			chirps = append(chirps, chirp)
		}
	}

	return chirps, nil
}

func (db *DB) CreateUser(email, password string) (User, error) {
	var result dbUser

	dbs, err := db.loadDB()
	if err != nil {
		return User{}, err
	}

	if len(dbs.Users) == 0 {
		dbs.Users = make(map[int]dbUser)
	}
	if len(dbs.Emails) == 0 {
		dbs.Emails = make(map[string]int)
	}
	pwh, err := bcrypt.GenerateFromPassword([]byte(password), 0)
	if err != nil {
		return User{}, err
	}
	result = dbUser{
		User: User{ID: db.userID, Email: email},
		PWH:  pwh,
	}
	_, exists := dbs.Emails[email]
	if exists {
		return User{}, errors.New("User already exists for " + email)
	}
	dbs.Emails[email] = result.ID
	dbs.Users[result.ID] = result
	db.userID++

	err = db.writeDB(dbs)
	if err != nil {
		return result.User, err
	}

	return result.User, nil
}

func (db *DB) UpdateUser(id int, properties map[string]string) (User, error) {
	dbs, err := db.loadDB()
	if err != nil {
		return User{}, err
	}
	user, ok := dbs.Users[id]
	if !ok {
		return User{}, errors.New("User id does not exist")
	}

	for key, prop := range properties {
		switch key {
		case "password":
			user.PWH, err = bcrypt.GenerateFromPassword([]byte(prop), 0)
			if err != nil {
				return User{}, nil
			}
		case "email":
			if prop != user.Email {
				delete(dbs.Emails, user.Email)
				dbs.Emails[prop] = user.ID
				user.Email = prop
			}
		case "is_chirpy_red":
			user.IsChirpyRed = (prop == "true")
		}
	}

	dbs.Users[id] = user
	db.writeDB(dbs)

	return user.User, nil
}

func (db *DB) GetUsers() ([]User, error) {
	var result []User
	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	for _, u := range dbs.Users {
		result = append(result, u.User)
	}

	return result, nil
}

func (db *DB) UserLogin(email, password string) (User, error) {
	var result User
	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}
	id, ok := dbs.Emails[email]
	if !ok {
		return result, errors.New("Email does not exist")
	}
	user := dbs.Users[id]

	err = bcrypt.CompareHashAndPassword(user.PWH, []byte(password))
	if err != nil {
		return result, err
	}

	result = user.User
	return result, nil
}

func (db *DB) RevokeToken(token string) error {
	dbs, err := db.loadDB()
	if err != nil {
		return err
	}

	if len(dbs.Revocations) == 0 {
		dbs.Revocations = make(map[string]time.Time)
	}

	_, ok := dbs.Revocations[token]
	if ok {
		// Token has already been revoked
		return nil
	}
	dbs.Revocations[token] = time.Now()

	err = db.writeDB(dbs)
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) GetTokenRevocation(token string) (time.Time, error) {
	var result time.Time
	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	result, ok := dbs.Revocations[token]
	if !ok {
		return result, errors.New("Token has not been revoked")
	}

	return result, nil
}

func (db *DB) IsTokenRevoked(token string) bool {
	dbs, err := db.loadDB()
	if err != nil {
		return false
	}
	_, ok := dbs.Revocations[token]
	if !ok {
		return false
	}

	return true
}

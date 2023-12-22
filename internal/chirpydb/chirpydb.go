package chirpydb

import (
	"encoding/json"
	"errors"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

type Chirp struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

type dbUser struct {
	User
	PWH []byte `json:"pwh"`
}

type User struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

type DB struct {
	path string
	mux  *sync.RWMutex
}

type DBStructure struct {
	Chirps map[int]Chirp
	Users  map[int]dbUser
	Emails map[string]int
}

func NewDB(path string) (*DB, error) {
	result := new(DB)
	result.mux = new(sync.RWMutex)
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

func (db *DB) CreateChirp(msg string) (Chirp, error) {
	var result Chirp

	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	if len(dbs.Chirps) == 0 {
		dbs.Chirps = make(map[int]Chirp)
	}
	result = Chirp{
		ID:   len(dbs.Chirps) + 1,
		Body: msg,
	}
	dbs.Chirps[result.ID] = result

	err = db.writeDB(dbs)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (db *DB) GetChirps() ([]Chirp, error) {
	var chirps []Chirp
	dbs, err := db.loadDB()
	if err != nil {
		return chirps, err
	}

	chirps = make([]Chirp, len(dbs.Chirps))
	for i, _ := range dbs.Chirps {
		chirps[i-1] = dbs.Chirps[i]
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
		User: User{ID: len(dbs.Users) + 1, Email: email},
		PWH:  pwh,
	}
	_, exists := dbs.Emails[email]
	if exists {
		return User{}, errors.New("User already exists for " + email)
	}
	dbs.Emails[email] = result.ID
	dbs.Users[result.ID] = result

	err = db.writeDB(dbs)
	if err != nil {
		return result.User, err
	}

	return result.User, nil
}

func (db *DB) UpdateUser(id int, email, password string) (User, error) {
	dbs, err := db.loadDB()
	if err != nil {
		return User{}, err
	}
	user, ok := dbs.Users[id]
	if !ok {
		return User{}, errors.New("User id does not exist")
	}

	user.PWH, err = bcrypt.GenerateFromPassword([]byte(password), 0)
	if err != nil {
		return User{}, nil
	}

	if email != user.Email {
		delete(dbs.Emails, user.Email)
		dbs.Emails[email] = user.ID
		user.Email = email
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

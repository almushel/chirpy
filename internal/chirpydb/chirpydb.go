package chirpydb

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
)

type Chirp struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
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
	Users  map[int]User
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
		dbs.Chirps = make(map[int]Chirp)
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

func (db *DB) CreateUser(email string) (User, error) {
	var result User

	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	if len(dbs.Users) == 0 {
		dbs.Users = make(map[int]User)
	}
	result = User{
		ID:    len(dbs.Users) + 1,
		Email: email,
	}
	dbs.Users[result.ID] = result

	err = db.writeDB(dbs)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (db *DB) GetUsers() ([]User, error) {
	var result []User
	dbs, err := db.loadDB()
	if err != nil {
		return result, err
	}

	for _, u := range dbs.Users {
		result = append(result, u)
	}

	return result, nil
}

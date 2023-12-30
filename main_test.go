package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/almushel/chirpy/internal/chirpapi"
)

type chirpStruct struct {
	ID       int    `json:"id"`
	AuthorID int    `json:"author_id"`
	Body     string `json:"body"`
}

const (
	dbPath     = "test_database.json"
	serverAddr = "localhost:8080"
	apiAddr    = "http://" + serverAddr + "/api"

	testEmail1 = "user@email.com"
	testEmail2 = "user2@email.com"
	testPW1    = "12345"
)

var running bool
var accessToken, refreshToken string

func init() {
	os.Remove(dbPath)

	var cfg *ApiConfig
	var jwt, pk []byte

	jwt = make([]byte, 64)
	_, err := rand.Read(jwt)
	if err == nil {
		pk = make([]byte, 16)
		_, err = rand.Read(pk)
		if err == nil {
			cfg, err = NewChirpAPI(dbPath, string(jwt), string(pk))
		}
	}

	if err != nil {
		println(err)
		os.Exit(1)
	}

	server, err := InitServer(cfg, serverAddr)
	go func() {
		running = true
		println(server.ListenAndServe().Error())
	}()
}

func TestMain(m *testing.M) {
	// Wait for test server to start
	for !running {
		time.Sleep(1 * time.Millisecond)
	}
	result := m.Run()
	os.Exit(result)
}

func TestAppGet(t *testing.T) {
	fbuf, err := os.ReadFile("serve/index.html")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get("http://" + serverAddr + "/app/serve")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if strings.Compare(string(fbuf), string(respBody)) != 0 {
		t.Fatal("Unexpected response body (index.html)")
	}
}

func TestPostUser(t *testing.T) {
	requestBody := []byte(fmt.Sprintf(`{"password":"%s", "email":"%s"}`, testPW1, testEmail1))
	response, err := http.Post(apiAddr+"/users", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	rBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Compare(string(rBody), fmt.Sprintf(`{"id":1,"email":"%s","is_chirpy_red":false}`, testEmail1)) != 0 {
		t.Fatalf("Unexpected response: %s", string(rBody))
	}
}

func TestPostLogin(t *testing.T) {
	requestBody := []byte(fmt.Sprintf(`{"password":"%s", "email":"%s"}`, testPW1, testEmail1))
	response, err := http.Post(apiAddr+"/login", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	rBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != 200 {
		println(response.StatusCode)
		t.Fatal("Request failed", string(rBody))
	}

	type loginAuth struct {
		ID           int    `json:"id"`
		Email        string `json:"email"`
		IsChirpyRed  bool   `json:"is_chirpy_red"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	var auth loginAuth
	err = json.Unmarshal(rBody, &auth)
	if err != nil {
		t.Fatal(err)
	}

	accessToken = auth.Token
	refreshToken = auth.RefreshToken
}

func TestPutUser(t *testing.T) {
	requestBody := []byte(fmt.Sprintf(`{"password":"%s", "email":"%s"}`, testPW1, testEmail2))
	request, _ := http.NewRequest("PUT", apiAddr+"/users", bytes.NewBuffer(requestBody))
	request.Header.Add("Authorization", "Bearer "+accessToken)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	rBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Compare(string(rBody), fmt.Sprintf(`{"id":1,"email":"%s","is_chirpy_red":false}`, testEmail2)) != 0 {
		t.Fatalf("Unexpected response: %s", string(rBody))
	}
}

func TestPostChirp(t *testing.T) {
	chirps := [][]byte{
		[]byte(`{"body":"This is a test chirp!"}`),
		[]byte(`{"body":"This is another test chirp!"}`),
		[]byte(`{"body":"This is third test chirp!"}`),
		[]byte(`{"body":"This is fourth test chirp!"}`),
	}

	for i, chirp := range chirps {
		request, err := http.NewRequest("POST", apiAddr+"/chirps", bytes.NewBuffer(chirp))
		if err != nil {
			t.Fatal(err)
		}

		request.Header.Add("Authorization", "Bearer "+accessToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()

		rBody, err := io.ReadAll(response.Body)
		if err != err {
			t.Fatal(err)
		}
		expected := fmt.Sprintf(`{"id":%d,"author_id":1,%s}`, i+1, chirp[1:len(chirp)-1])
		if strings.Compare(string(rBody), expected) != 0 {
			t.Fatalf("\nExpected; %s\n Recieved: %s", expected, string(rBody))
		}

		if response.StatusCode != 201 {
			println(response.StatusCode)
			t.Fatal("Post Chirp failed")
		}
	}
}

func getChirps() ([]chirpStruct, error) {
	var chirpList []chirpStruct

	response, err := http.Get(apiAddr + "/chirps")
	if err != nil {
		return chirpList, err
	}

	responseBody, err := io.ReadAll(response.Body)
	defer response.Body.Close()

	if response.StatusCode != 200 {
		println(response.StatusCode)
		return chirpList, err
	}

	err = json.Unmarshal(responseBody, &chirpList)
	if err != nil {
		return chirpList, err
	}

	return chirpList, nil
}
func TestGetChirps(t *testing.T) {
	chirpList, err := getChirps()
	if err != nil {
		t.Fatal(err)
	}

	chirpID := chirpList[len(chirpList)-1].ID
	response, err := http.Get(apiAddr + "/chirps/" + fmt.Sprint(chirpID))
	if err != nil {
		t.Fatal(err)
	}
	responseBody, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != 200 {
		println(response.StatusCode)
		t.Fatal(string(responseBody))
	}

	//println(string(responseBody))
}

func TestDeleteChirp(t *testing.T) {
	chirpList, err := getChirps()
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println(chirpList)
	chirpCount := len(chirpList)
	deleteID := chirpCount / 2

	request, err := http.NewRequest("DELETE", apiAddr+"/chirps/"+fmt.Sprint(deleteID), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("Authorization", "Bearer "+accessToken)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != 200 {
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal(string(responseBody))
	}

	chirpList, err = getChirps()
	if err != nil {
		t.Fatal(err)
	}

	if len(chirpList) != chirpCount-1 {
		fmt.Println(chirpList)
		t.Fatalf("Before: %d, After: %d\n", chirpCount, len(chirpList))
	}
}

// TO-DO: Tests for refresh, revoke

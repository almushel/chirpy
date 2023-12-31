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

func testRequest(t *testing.T, request *http.Request, codeExpected int, failureMSG string) *http.Response {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	} else if response.StatusCode != codeExpected {
		t.Fatal(failureMSG)
	}

	return response
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
	request, err := http.NewRequest("POST", apiAddr+"/login", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	response := testRequest(t, request, 200, "Login POST request failed")

	defer response.Body.Close()
	rBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	var auth struct {
		ID           int    `json:"id"`
		Email        string `json:"email"`
		IsChirpyRed  bool   `json:"is_chirpy_red"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	err = json.Unmarshal(rBody, &auth)
	if err != nil {
		t.Fatal(err)
	}

	accessToken = auth.Token
	refreshToken = auth.RefreshToken
}

func TestPutUser(t *testing.T) {
	requestBody := []byte(fmt.Sprintf(`{"password":"%s", "email":"%s"}`, testPW1, testEmail2))

	request, err := http.NewRequest("PUT", apiAddr+"/users", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}

	request.Header.Add("Authorization", "Bearer "+accessToken)
	response := testRequest(t, request, 200, "Failed to update user info")

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
		//testRequest(t, request, 401, "Successfully posted chirp without authorization header")
		request.Header.Add("Authorization", "Bearer "+accessToken)
		response := testRequest(t, request, 201, "Failed to post chirp")

		defer response.Body.Close()
		rBody, err := io.ReadAll(response.Body)
		if err != err {
			t.Fatal(err)
		}

		expected := fmt.Sprintf(`{"id":%d,"author_id":1,%s}`, i+1, chirp[1:len(chirp)-1])
		if strings.Compare(string(rBody), expected) != 0 {
			t.Fatalf("\nExpected; %s\n Recieved: %s", expected, string(rBody))
		}
	}
}
func TestGetChirps(t *testing.T) {
	chirpList, err := getChirps()
	if err != nil {
		t.Fatal(err)
	}

	chirpID := chirpList[len(chirpList)-1].ID
	request, err := http.NewRequest("GET", apiAddr+"/chirps/"+fmt.Sprint(chirpID), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := testRequest(t, request, 200, "Failed to get chirp #"+fmt.Sprint(chirpID))
	responseBody, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if strings.Compare(string(responseBody), `{"id":4,"author_id":1,"body":"This is fourth test chirp!"}`) != 0 {
		t.Fatal("Unexpected GET chirp response body")
	}
}

func TestDeleteChirp(t *testing.T) {
	deleteID := 3
	request, err := http.NewRequest("DELETE", apiAddr+"/chirps/"+fmt.Sprint(deleteID), nil)
	if err != nil {
		t.Fatal(err)
	}
	testRequest(t, request, 401, "Successfully deleted chirp without authorization")

	request.Header.Add("Authorization", "Bearer "+accessToken)
	testRequest(t, request, 200, "Failed to delete chirp with authorization")

	request, _ = http.NewRequest("GET", apiAddr+"/chirps/"+fmt.Sprint(deleteID), nil)
	testRequest(t, request, 404, "Successfully GOT deleted chirp")
}

func TestRefresh(t *testing.T) {
	request, err := http.NewRequest("POST", apiAddr+"/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	testRequest(t, request, 401, "Refreshed access token without authorization header")

	request.Header.Add("Authorization", "Bearer "+refreshToken)
	response := testRequest(t, request, 200, "Refresh failed with valid authorization")
	responseBody, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		t.Fatal(err)
	} else if response.StatusCode != 200 {
		t.Fatal(response.Status)
	}

	var refresh struct {
		Token string `json:"token"`
	}
	err = json.Unmarshal(responseBody, &refresh)
	if err != nil {
		t.Fatal(err)
	}
	accessToken = refresh.Token
}

func TestRevoke(t *testing.T) {
	request, err := http.NewRequest("POST", apiAddr+"/revoke", nil)
	if err != nil {
		t.Fatal(err)
	}
	testRequest(t, request, 401, "Revoke succeeded without authorization")

	request.Header.Add("Authorization", "Bearer "+refreshToken)
	testRequest(t, request, 200, "Revoke failed with authorization")

	request, _ = http.NewRequest("POST", apiAddr+"/refresh", nil)
	testRequest(t, request, 401, "Refresh token still valid after revoke")
}

package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"lan-transfer/internal/config"
)

func TestLoginAndFileFlow(t *testing.T) {
	srv, err := New(Options{Config: config.Config{
		Port:       18080,
		StorageDir: t.TempDir(),
		AccessCode: "123456",
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.routes()

	if status := request(t, handler, http.MethodGet, "/api/files", nil, "").Code; status != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", status)
	}

	loginBody := bytes.NewBufferString(`{"code":"123456"}`)
	login := request(t, handler, http.MethodPost, "/api/login", loginBody, "application/json")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	upload := httptest.NewRecorder()
	handler.ServeHTTP(upload, req)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body = %s", upload.Code, upload.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	listReq.AddCookie(cookie)
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", list.Code, list.Body.String())
	}
	var payload struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Files) != 1 || payload.Files[0].Name != "hello.txt" {
		t.Fatalf("unexpected list: %#v", payload.Files)
	}
}

func request(t *testing.T, handler http.Handler, method, path string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body.Bytes())
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

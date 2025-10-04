package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type errorResponse struct {
	Error string `json:"error"`
}

type userResponse struct {
	UserID int `json:"user_id"`
}

type createUserRequest struct {
	Name string `json:"name"`
}

type createUserResponse struct {
	Created string `json:"created"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
	}
}

func errorJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		errorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		errorJSON(w, http.StatusBadRequest, "invalid id")
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid id")
		return
	}

	writeJSON(w, http.StatusOK, userResponse{UserID: id})
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		errorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	raw, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	raw = bytes.TrimSpace(raw)
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	var name string

	if len(raw) > 0 && raw[0] == '{' {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &req); err == nil {
			name = strings.TrimSpace(req.Name)
		} else {
			log.Printf("POST /user: json unmarshal error: %v; raw=%q; ctype=%q",
				err, string(raw), r.Header.Get("Content-Type"))
		}
	}
	if name == "" {
		_ = r.ParseForm()
		if v := r.Form.Get("name"); v != "" {
			name = strings.TrimSpace(v)
		}
		if name == "" {
			if v := r.URL.Query().Get("name"); v != "" {
				name = strings.TrimSpace(v)
			}
		}
	}

	if name == "" {
		errorJSON(w, http.StatusBadRequest, "invalid name")
		return
	}

	writeJSON(w, http.StatusCreated, createUserResponse{Created: name})
}

func authAndLog(next http.Handler) http.Handler {
	const requiredKey = "secret123"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s", r.Method, r.URL.Path)

		if key := r.Header.Get("X-API-Key"); key != requiredKey {
			errorJSON(w, http.StatusUnauthorized, "unauthorized")
			log.Printf("-> %d (%s)", http.StatusUnauthorized, time.Since(start))
			return
		}

		rr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rr, r)
		log.Printf("-> %d (%s)", rr.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetUser(w, r)
		case http.MethodPost:
			handleCreateUser(w, r)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			errorJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	return authAndLog(mux)
}

func main() {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: routes(),
	}

	log.Println("listening on http://localhost:8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

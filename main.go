package main

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	mux := http.NewServeMux()

	// Server init
	notebook := &server{
		nextID: 1,
		notes: map[int]Note{
			1: {ID: 1, Title: "Initial note", Body: "Example initial note"},
		},
	}

	// health route
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	// old health route
	mux.HandleFunc("GET /old-health", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/health", http.StatusMovedPermanently)
	})

	// ping route
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
	})

	// hello route
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/plain") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "hello, Gopher")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "hello"})
	})

	// GET notes route
	mux.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, notebook.notes)
	})

	// POST notes route
	mux.HandleFunc("POST /notes", notebook.createNote)

	// GET note route
	mux.HandleFunc("GET /notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "Page not found")
			return
		}

		notebook.mu.Lock()
		note, exist := notebook.notes[id]
		defer notebook.mu.Unlock()

		if !exist {
			writeError(w, http.StatusNotFound, "Page not found")
			return
		}

		writeJSON(w, http.StatusOK, note)
	})

	// search route
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		term := query.Get("q")
		limit := 10

		if search := query.Get("limit"); search != "" {
			n, err := strconv.Atoi(search)
			if err != nil || n < 1 || n > 100 {
				writeError(w, http.StatusBadRequest, "limit mush be an integer 1-100")
				return
			}
			limit = n
		}
		writeJSON(w, http.StatusOK, map[string]any{"q": term, "limit": limit})
	})

	srv := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("litening on", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}

type Note struct {
	ID    int
	Title string
	Body  string
}

type craeteNoteReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type server struct {
	mu     sync.Mutex
	nextID int
	notes  map[int]Note
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s ua=%q accept=%q (%s)", r.Method, r.URL.RequestURI(),
			r.UserAgent(), r.Header.Get("Accept"), time.Since(start))
	})
}

func (s *server) add(title, body string) Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	n := Note{ID: s.nextID, Title: title, Body: body}
	s.notes[n.ID] = n
	return n
}

func (s *server) createNote(w http.ResponseWriter, r *http.Request) {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var in craeteNoteReq
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Invalid JSON body"+err.Error())
		return
	}

	if in.Title == "" || in.Body == "" {
		writeError(w, http.StatusLengthRequired, "Invalid empty field")
		return
	}

	note := s.add(in.Title, in.Body)
	w.Header().Set("Location", fmt.Sprintf("/notes/%d", note.ID))
	writeJSON(w, http.StatusCreated, note)
}

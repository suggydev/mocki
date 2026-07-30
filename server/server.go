// Package server — HTTP-слой mocki: REST CRUD поверх store.
package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mocki/store"
)

// Options — настройки сервера.
type Options struct {
	Latency time.Duration // искусственная задержка ответа
	CORS    bool          // Access-Control-Allow-*
	Logger  *slog.Logger
}

// Server — REST API поверх хранилища.
type Server struct {
	st   *store.Store
	opts Options
	mux  *http.ServeMux
}

// New создаёт сервер.
func New(st *store.Store, opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{st: st, opts: opts, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /{resource}", s.handleList)
	s.mux.HandleFunc("POST /{resource}", s.handleCreate)
	s.mux.HandleFunc("GET /{resource}/{id}", s.handleGet)
	s.mux.HandleFunc("PUT /{resource}/{id}", s.handleUpdate)
	s.mux.HandleFunc("PATCH /{resource}/{id}", s.handlePatch)
	s.mux.HandleFunc("DELETE /{resource}/{id}", s.handleDelete)
}

// ServeHTTP — middleware: лог, latency, CORS.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if s.opts.Latency > 0 {
		time.Sleep(s.opts.Latency)
	}
	if s.opts.CORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
	s.opts.Logger.Info("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	links := map[string]string{}
	for _, res := range s.st.Resources() {
		links[res] = "/" + res
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": links})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	q := parseQuery(r)
	items, total, err := s.st.List(resource, q)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "resource not found: "+resource)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []store.Item{}
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	item, err := s.st.Get(r.PathValue("resource"), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var item store.Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	created, err := s.st.Create(r.PathValue("resource"), item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var item store.Item
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := s.st.Update(r.PathValue("resource"), r.PathValue("id"), item)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	var patch store.Item
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := s.st.Patch(r.PathValue("resource"), r.PathValue("id"), patch)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	err := s.st.Delete(r.PathValue("resource"), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func parseQuery(r *http.Request) store.Query {
	vals := r.URL.Query()
	q := store.Query{
		Filters: make(map[string]string),
		Q:       vals.Get("q"),
		Sort:    vals.Get("_sort"),
		Order:   strings.ToLower(vals.Get("_order")),
	}
	q.Page, _ = strconv.Atoi(vals.Get("_page"))
	q.Limit, _ = strconv.Atoi(vals.Get("_limit"))
	if q.Page < 0 {
		q.Page = 0
	}
	if q.Limit < 0 {
		q.Limit = 0
	}
	for key := range vals {
		if strings.HasPrefix(key, "_") || key == "q" {
			continue
		}
		q.Filters[key] = vals.Get(key)
	}
	return q
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

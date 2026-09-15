package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

//go:embed index.html
var index string

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,127}$`)

const hash = "web-key-value"

type item struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type page struct {
	Items []item `json:"items"`
	Next  string `json:"next"`
}

func paginate(values map[string]string, after string, limit int) page {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key > after {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := page{Items: []item{}}
	if len(keys) > limit {
		keys = keys[:limit]
		result.Next = keys[len(keys)-1]
	}
	for _, key := range keys {
		result.Items = append(result.Items, item{key, values[key]})
	}
	return result
}

func handler(db *redis.Client, secret string) http.Handler {
	mux := http.NewServeMux()
	fail := func(w http.ResponseWriter, err error) {
		log.Printf("store error: %v", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, index)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()).Err(); err != nil {
			fail(w, err)
			return
		}
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /api/keys", func(w http.ResponseWriter, r *http.Request) {
		limit := 10
		if raw := r.URL.Query().Get("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 100 {
				http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
				return
			}
		}
		values, err := db.HGetAll(r.Context(), hash).Result()
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paginate(values, r.URL.Query().Get("after"), limit))
	})
	mux.HandleFunc("/api/keys/{key}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "PUT" && r.Method != "DELETE" {
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Method != "GET" && (secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "write secret required or incorrect", http.StatusUnauthorized)
			return
		}
		key := r.PathValue("key")
		if !keyPattern.MatchString(key) {
			http.Error(w, "key must be 1–128 letters, digits, dots, underscores or hyphens; no leading dot", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case "GET":
			value, err := db.HGet(r.Context(), hash, key).Result()
			if errors.Is(err, redis.Nil) {
				http.NotFound(w, r)
			} else if err != nil {
				fail(w, err)
			} else {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				io.WriteString(w, value)
			}
		case "PUT":
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
			if err != nil {
				var tooLarge *http.MaxBytesError
				status := http.StatusBadRequest
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
				}
				http.Error(w, "could not read value (maximum 16 KiB)", status)
				return
			}
			if !utf8.Valid(body) {
				http.Error(w, "value must be UTF-8 text", http.StatusBadRequest)
				return
			}
			created, err := db.HSet(r.Context(), hash, key, string(body)).Result()
			if err != nil {
				fail(w, err)
			} else if created == 1 {
				w.Header().Set("Location", r.URL.Path)
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		case "DELETE":
			if err := db.HDel(r.Context(), hash, key).Err(); err != nil {
				fail(w, err)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		recorded := &response{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(recorded, r)
		log.Printf("%s %s status=%d duration=%s", r.Method, r.URL.RequestURI(), recorded.status, time.Since(start))
	})
}

type response struct {
	http.ResponseWriter
	status int
}

func (w *response) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func main() {
	secret := os.Getenv("WRITE_SECRET")
	options, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil || secret == "" {
		log.Fatal("set REDIS_URL to a Redis connection URL and WRITE_SECRET to a nonempty secret")
	}
	options.ContextTimeoutEnabled = true
	db := redis.NewClient(options)
	defer db.Close()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: handler(db, secret), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Printf("Web Key Value listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}

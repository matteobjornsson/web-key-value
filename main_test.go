package main

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestWriteAuthorization(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE"} {
		for _, token := range []string{"", "Bearer wrong", "test-secret", "Bearer "} {
			t.Run(method+"/"+token, func(t *testing.T) {
				r := httptest.NewRequest(method, "/api/keys/example", strings.NewReader("value"))
				r.Header.Set("Authorization", token)
				w := httptest.NewRecorder()
				handler(nil, "test-secret").ServeHTTP(w, r)
				if w.Code != 401 {
					t.Fatalf("status = %d, want 401", w.Code)
				}
			})
		}
	}
	r := httptest.NewRequest("PUT", "/api/keys/example", nil)
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	handler(nil, "").ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("empty configured secret: status = %d, want 401", w.Code)
	}
}

func TestRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/", "", 200},
		{"GET", "/missing", "", 404},
		{"PATCH", "/api/keys/example", "", 405},
		{"GET", "/api/keys?limit=0", "", 400},
		{"GET", "/api/keys?limit=101", "", 400},
		{"GET", "/api/keys?limit=nope", "", 400},
		{"PUT", "/api/keys/bad%20key", "value", 400},
		{"PUT", "/api/keys/example", strings.Repeat("x", 16385), 413},
		{"PUT", "/api/keys/example", "\xff", 400},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer test-secret")
			w := httptest.NewRecorder()
			handler(nil, "test-secret").ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestPagination(t *testing.T) {
	values := map[string]string{"c": "three", "a": "", "b": "two"}
	first := paginate(values, "", 2)
	if want := (page{Items: []item{{"a", ""}, {"b", "two"}}, Next: "b"}); !reflect.DeepEqual(first, want) {
		t.Fatalf("first = %#v, want %#v", first, want)
	}
	delete(values, "b")
	last := paginate(values, first.Next, 2)
	if want := (page{Items: []item{{"c", "three"}}}); !reflect.DeepEqual(last, want) {
		t.Fatalf("last = %#v, want %#v", last, want)
	}
	if empty := paginate(nil, "", 10); empty.Items == nil || len(empty.Items) != 0 || empty.Next != "" {
		t.Fatalf("empty page = %#v", empty)
	}
	if exact := paginate(values, "", 2); exact.Next != "" {
		t.Fatalf("exact-size final page has next = %q", exact.Next)
	}
}

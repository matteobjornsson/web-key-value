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
	first := paginate(values, nil, "", 2)
	if want := (page{Items: []item{{"a", ""}, {"b", "two"}}, Next: "|b"}); !reflect.DeepEqual(first, want) {
		t.Fatalf("first = %#v, want %#v", first, want)
	}
	delete(values, "b")
	last := paginate(values, nil, first.Next, 2)
	if want := (page{Items: []item{{"c", "three"}}}); !reflect.DeepEqual(last, want) {
		t.Fatalf("last = %#v, want %#v", last, want)
	}
	if empty := paginate(nil, nil, "", 10); empty.Items == nil || len(empty.Items) != 0 || empty.Next != "" {
		t.Fatalf("empty page = %#v", empty)
	}
	if exact := paginate(values, nil, "", 2); exact.Next != "" {
		t.Fatalf("exact-size final page has next = %q", exact.Next)
	}
}

func TestNewestFirstPagination(t *testing.T) {
	const older = "2026-09-15T00:00:00.100000000Z"
	const newer = "2026-09-15T00:00:00.110000000Z"
	values := map[string]string{"a": "old", "b": "tie", "c": "new", "d": "legacy"}
	updated := map[string]string{"a": older, "b": newer, "c": newer}
	first := paginate(values, updated, "", 1)
	if want := (page{Items: []item{{"b", "tie"}}, Next: newer + "|b"}); !reflect.DeepEqual(first, want) {
		t.Fatalf("first = %#v, want %#v", first, want)
	}
	delete(values, "b")
	delete(updated, "b")
	second := paginate(values, updated, first.Next, 1)
	if want := (page{Items: []item{{"c", "new"}}, Next: newer + "|c"}); !reflect.DeepEqual(second, want) {
		t.Fatalf("second = %#v, want %#v", second, want)
	}
	updated["c"] = "2026-09-15T00:00:01.000000000Z"
	last := paginate(values, updated, second.Next, 2)
	if want := (page{Items: []item{{"a", "old"}, {"d", "legacy"}}}); !reflect.DeepEqual(last, want) {
		t.Fatalf("last = %#v, want %#v", last, want)
	}
	updated["a"] = "2026-09-15T00:00:02.000000000Z"
	if refreshed := paginate(values, updated, "", 1); refreshed.Items[0].Key != "a" {
		t.Fatalf("edited entry did not move to top: %#v", refreshed)
	}
}

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func sampleStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	writeTemp(t, dir, "users.json", `[
		{"id": 1, "name": "Ada", "role": "admin", "age": 36},
		{"id": 2, "name": "Bob", "role": "user", "age": 25},
		{"id": 3, "name": "Cleo", "role": "user", "age": 41}
	]`)
	s := New()
	if err := s.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	return s
}

func TestLoadDirAndResources(t *testing.T) {
	s := sampleStore(t)
	res := s.Resources()
	if len(res) != 1 || res[0] != "users" {
		t.Fatalf("resources = %v", res)
	}
}

func TestLoadObjectForm(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "db.json", `{"users": [{"id": 1}], "posts": [{"id": 9}]}`)
	s := New()
	if err := s.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	res := s.Resources()
	if len(res) != 2 || res[0] != "posts" || res[1] != "users" {
		t.Fatalf("resources = %v", res)
	}
}

func TestListFilters(t *testing.T) {
	s := sampleStore(t)
	items, total, err := s.List("users", Query{Filters: map[string]string{"role": "user"}})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("filtered = %d/%d, want 2/2", len(items), total)
	}
}

func TestListSearch(t *testing.T) {
	s := sampleStore(t)
	items, _, _ := s.List("users", Query{Q: "ada"})
	if len(items) != 1 || items[0]["name"] != "Ada" {
		t.Fatalf("search = %v", items)
	}
}

func TestListSortAndOrder(t *testing.T) {
	s := sampleStore(t)
	items, _, _ := s.List("users", Query{Sort: "age", Order: "desc"})
	if items[0]["name"] != "Cleo" || items[2]["name"] != "Bob" {
		t.Fatalf("sort desc = %v %v %v", items[0]["name"], items[1]["name"], items[2]["name"])
	}
}

func TestListPagination(t *testing.T) {
	s := sampleStore(t)
	items, total, _ := s.List("users", Query{Sort: "id", Page: 2, Limit: 2})
	if total != 3 || len(items) != 1 || items[0]["name"] != "Cleo" {
		t.Fatalf("page 2 = %v (total %d)", items, total)
	}
}

func TestCRUD(t *testing.T) {
	s := sampleStore(t)

	// Create с автоназначением id
	created, _ := s.Create("users", Item{"name": "Dan"})
	if created["id"] != 4.0 {
		t.Fatalf("auto id = %v", created["id"])
	}

	// Get
	it, err := s.Get("users", "2")
	if err != nil || it["name"] != "Bob" {
		t.Fatalf("get = %v, %v", it, err)
	}

	// Patch
	patched, _ := s.Patch("users", "2", Item{"age": 26.0})
	if patched["age"] != 26.0 || patched["name"] != "Bob" {
		t.Fatalf("patch = %v", patched)
	}

	// Update (PUT)
	replaced, _ := s.Update("users", "2", Item{"name": "Bobby"})
	if replaced["id"] != 2.0 || replaced["name"] != "Bobby" || replaced["age"] != nil {
		t.Fatalf("update = %v", replaced)
	}

	// Delete
	if err := s.Delete("users", "2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get("users", "2"); err != ErrNotFound {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestNotFound(t *testing.T) {
	s := sampleStore(t)
	if _, _, err := s.List("ghosts", Query{}); err != ErrNotFound {
		t.Fatalf("list unknown: %v", err)
	}
	if _, err := s.Get("users", "999"); err != ErrNotFound {
		t.Fatalf("get unknown: %v", err)
	}
}

func TestReloadIfChanged(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "users.json", `[{"id": 1, "name": "A"}]`)
	s := New()
	if err := s.LoadFile(p); err != nil {
		t.Fatal(err)
	}
	if n := s.ReloadIfChanged(); n != 0 {
		t.Fatalf("reload without changes = %d", n)
	}

	// mtime должен уйти вперёд
	time.Sleep(5 * time.Millisecond)
	writeTemp(t, dir, "users.json", `[{"id": 1, "name": "A"}, {"id": 2, "name": "B"}]`)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if n := s.ReloadIfChanged(); n != 1 {
		t.Fatalf("reload after change = %d", n)
	}
	_, total, _ := s.List("users", Query{})
	if total != 2 {
		t.Fatalf("after reload total = %d", total)
	}
}

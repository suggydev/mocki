// Package store — in-memory хранилище ресурсов из JSON-файлов.
//
// Файл users.json → ресурс /users. Файл db.json вида {"users": [...]} →
// ресурсы по ключам верхнего уровня. Хранилище потокобезопасно (RWMutex)
// и умеет перечитывать файлы при изменении (hot reload).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound — ресурс или запись не найдены.
var ErrNotFound = errors.New("not found")

// Item — запись ресурса (произвольный JSON-объект).
type Item = map[string]any

// Store — потокобезопасное хранилище ресурсов.
type Store struct {
	mu    sync.RWMutex
	data  map[string][]Item // ресурс → записи
	files []string          // отслеживаемые файлы
	mtime map[string]time.Time
}

// New пустое хранилище.
func New() *Store {
	return &Store{data: make(map[string][]Item), mtime: make(map[string]time.Time)}
}

// LoadDir загружает все *.json файлы из директории (не рекурсивно).
func (s *Store) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if err := s.LoadFile(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
		loaded++
	}
	if loaded == 0 {
		return fmt.Errorf("no .json files in %s", dir)
	}
	return nil
}

// LoadFile загружает один JSON-файл.
// Массив объектов → ресурс по имени файла; объект → ресурсы по ключам.
func (s *Store) LoadFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	name := strings.TrimSuffix(filepath.Base(path), ".json")

	var arr []Item
	if err := json.Unmarshal(raw, &arr); err == nil && arr != nil {
		s.set(name, arr)
	} else {
		var obj map[string][]Item
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return fmt.Errorf("%s: ожидается массив объектов или объект массивов", path)
		}
		for resource, items := range obj {
			s.set(resource, items)
		}
	}

	if st, err := os.Stat(path); err == nil {
		s.mu.Lock()
		s.files = appendUnique(s.files, path)
		s.mtime[path] = st.ModTime()
		s.mu.Unlock()
	}
	return nil
}

func appendUnique(xs []string, x string) []string {
	for _, e := range xs {
		if e == x {
			return xs
		}
	}
	return append(xs, x)
}

func (s *Store) set(resource string, items []Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[resource] = items
}

// ReloadIfChanged перечитывает файлы, у которых изменился mtime.
// Возвращает число перечитанных файлов.
func (s *Store) ReloadIfChanged() int {
	s.mu.RLock()
	files := append([]string(nil), s.files...)
	s.mu.RUnlock()

	reloaded := 0
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		s.mu.RLock()
		old := s.mtime[f]
		s.mu.RUnlock()
		if st.ModTime().After(old) {
			if err := s.LoadFile(f); err == nil {
				reloaded++
			}
		}
	}
	return reloaded
}

// Watch запускает polling-цикл hot reload (без внешних зависимостей).
func (s *Store) Watch(interval time.Duration, onReload func(n int)) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if n := s.ReloadIfChanged(); n > 0 && onReload != nil {
					onReload(n)
				}
			}
		}
	}()
	return func() { close(done) }
}

// Resources — отсортированный список имён ресурсов.
func (s *Store) Resources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Query — параметры выборки из query string.
type Query struct {
	Filters map[string]string // field=value
	Q       string            // подстрока по всем строковым полям
	Sort    string            // _sort=field
	Order   string            // _order=asc|desc
	Page    int               // _page (1-based, 0 = без пагинации)
	Limit   int               // _limit
}

// List возвращает записи ресурса с учётом Query и общее число до пагинации.
func (s *Store) List(resource string, q Query) ([]Item, int, error) {
	s.mu.RLock()
	items, ok := s.data[resource]
	s.mu.RUnlock()
	if !ok {
		return nil, 0, ErrNotFound
	}

	filtered := make([]Item, 0, len(items))
	for _, it := range items {
		if !matches(it, q) {
			continue
		}
		filtered = append(filtered, it)
	}

	if q.Sort != "" {
		sortItems(filtered, q.Sort, q.Order == "desc")
	}

	total := len(filtered)
	if q.Limit > 0 && q.Page > 0 {
		start := (q.Page - 1) * q.Limit
		if start >= len(filtered) {
			filtered = nil
		} else {
			end := start + q.Limit
			if end > len(filtered) {
				end = len(filtered)
			}
			filtered = filtered[start:end]
		}
	}
	return filtered, total, nil
}

// Get — запись по id.
func (s *Store) Get(resource string, id any) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.data[resource] {
		if idEqual(it["id"], id) {
			return it, nil
		}
	}
	return nil, ErrNotFound
}

// Create добавляет запись. Если id отсутствует — max(id)+1.
func (s *Store) Create(resource string, item Item) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[resource]; !ok {
		s.data[resource] = nil
	}
	if _, has := item["id"]; !has {
		item["id"] = s.nextIDLocked(resource)
	}
	s.data[resource] = append(s.data[resource], item)
	return item, nil
}

// Update заменяет запись по id (PUT: полная замена с сохранением id).
func (s *Store) Update(resource string, id any, item Item) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.data[resource] {
		if idEqual(it["id"], id) {
			item["id"] = it["id"]
			s.data[resource][i] = item
			return item, nil
		}
	}
	return nil, ErrNotFound
}

// Patch частично обновляет запись по id.
func (s *Store) Patch(resource string, id any, patch Item) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.data[resource] {
		if idEqual(it["id"], id) {
			for k, v := range patch {
				it[k] = v
			}
			s.data[resource][i] = it
			return it, nil
		}
	}
	return nil, ErrNotFound
}

// Delete удаляет запись по id.
func (s *Store) Delete(resource string, id any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.data[resource]
	for i, it := range items {
		if idEqual(it["id"], id) {
			s.data[resource] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) nextIDLocked(resource string) float64 {
	max := 0.0
	for _, it := range s.data[resource] {
		if n, ok := it["id"].(float64); ok && n > max {
			max = n
		}
	}
	return max + 1
}

// idEqual сравнивает id, приводя обе стороны к строке числа, если возможно.
func idEqual(a, b any) bool {
	if fmt.Sprint(a) == fmt.Sprint(b) {
		return true
	}
	return numericID(a) == numericID(b) && numericID(a) != ""
}

func numericID(v any) string {
	switch t := v.(type) {
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprint(int64(t))
		}
		return fmt.Sprint(t)
	case string:
		return t
	default:
		return ""
	}
}

// matches — фильтры field=value и q-подстрока по строковым полям.
func matches(it Item, q Query) bool {
	for field, want := range q.Filters {
		got, ok := it[field]
		if !ok || fmt.Sprint(got) != want {
			return false
		}
	}
	if q.Q != "" {
		found := false
		for _, v := range it {
			if str, ok := v.(string); ok && strings.Contains(strings.ToLower(str), strings.ToLower(q.Q)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// sortItems — стабильная сортировка по полю (числа как числа, остальное как строки).
func sortItems(items []Item, field string, desc bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i][field], items[j][field]
		an, aok := a.(float64)
		bn, bok := b.(float64)
		var less bool
		switch {
		case aok && bok:
			less = an < bn
		default:
			less = fmt.Sprint(a) < fmt.Sprint(b)
		}
		if desc {
			return !less
		}
		return less
	})
}

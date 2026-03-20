package store

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddAndGet(t *testing.T) {
	s := tempDB(t)

	doc := &Document{
		Content:   "Test document about DNS configuration",
		Tags:      []string{"type:note", "topic:dns"},
		Workspace: "default",
		Source:    "cli",
	}

	if err := s.Add(doc); err != nil {
		t.Fatalf("add: %v", err)
	}

	if doc.ID == "" {
		t.Fatal("expected ID to be set")
	}

	got, err := s.Get(doc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Content != doc.Content {
		t.Errorf("content = %q, want %q", got.Content, doc.Content)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(got.Tags))
	}
	if got.Workspace != "default" {
		t.Errorf("workspace = %q, want default", got.Workspace)
	}
}

func TestDuplicateContent(t *testing.T) {
	s := tempDB(t)

	doc := &Document{Content: "Same content", Workspace: "default"}
	if err := s.Add(doc); err != nil {
		t.Fatal(err)
	}

	doc2 := &Document{Content: "Same content", Workspace: "default"}
	err := s.Add(doc2)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestDuplicateContentDifferentWorkspace(t *testing.T) {
	s := tempDB(t)

	doc := &Document{Content: "Same content", Workspace: "work"}
	if err := s.Add(doc); err != nil {
		t.Fatal(err)
	}

	doc2 := &Document{Content: "Same content", Workspace: "personal"}
	if err := s.Add(doc2); err != nil {
		t.Fatal("should allow same content in different workspace")
	}
}

func TestList(t *testing.T) {
	s := tempDB(t)

	for i := 0; i < 5; i++ {
		s.Add(&Document{
			Content:   "Document " + string(rune('A'+i)),
			Workspace: "default",
			Tags:      []string{"type:note"},
		})
	}
	s.Add(&Document{
		Content:   "Work document",
		Workspace: "work",
		Tags:      []string{"type:worklog"},
	})

	// List all in default
	docs, err := s.List(ListOptions{Workspace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 5 {
		t.Errorf("got %d docs, want 5", len(docs))
	}

	// List with limit
	docs, err = s.List(ListOptions{Workspace: "default", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Errorf("got %d docs, want 2", len(docs))
	}

	// List by tag
	docs, err = s.List(ListOptions{Tag: "type:worklog"})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Errorf("got %d docs, want 1", len(docs))
	}
}

func TestSearch(t *testing.T) {
	s := tempDB(t)

	s.Add(&Document{Content: "Configure DNS records for mail server", Workspace: "default"})
	s.Add(&Document{Content: "Deploy new version of the web application", Workspace: "default"})
	s.Add(&Document{Content: "Fix SSL certificate renewal for DNS provider", Workspace: "default"})

	results, err := s.Search("DNS", "default", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Errorf("got %d results, want at least 2", len(results))
	}
}

func TestSearchWorkspaceFilter(t *testing.T) {
	s := tempDB(t)

	s.Add(&Document{Content: "DNS config in work", Workspace: "work"})
	s.Add(&Document{Content: "DNS config in personal", Workspace: "personal"})

	results, err := s.Search("DNS", "work", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestUpdateAndDelete(t *testing.T) {
	s := tempDB(t)

	doc := &Document{Content: "Original content", Workspace: "default"}
	s.Add(doc)

	doc.Content = "Updated content"
	if err := s.Update(doc); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(doc.ID)
	if got.Content != "Updated content" {
		t.Errorf("content = %q, want Updated content", got.Content)
	}

	if err := s.Delete(doc.ID); err != nil {
		t.Fatal(err)
	}

	_, err := s.Get(doc.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestPinnedFilter(t *testing.T) {
	s := tempDB(t)

	s.Add(&Document{Content: "Regular doc", Workspace: "default"})
	s.Add(&Document{Content: "Pinned doc", Workspace: "default", Pinned: true})

	pinned := true
	docs, err := s.List(ListOptions{Workspace: "default", Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Errorf("got %d pinned docs, want 1", len(docs))
	}
	if docs[0].Content != "Pinned doc" {
		t.Errorf("wrong doc: %s", docs[0].Content)
	}
}

func TestDBFileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("db file was not created")
	}
}

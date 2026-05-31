package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectBooks(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "Dune.epub"))
	writeFile(t, filepath.Join(root, "notes.txt"))
	writeFile(t, filepath.Join(root, ".hidden.pdf"))
	writeFile(t, filepath.Join(root, "Sci-Fi", "The_Left-Hand_of_Darkness.pdf"))
	writeFile(t, filepath.Join(root, ".private", "Secret.epub"))

	books, err := collectBooks(root)
	if err != nil {
		t.Fatalf("collectBooks() error = %v", err)
	}

	if len(books) != 2 {
		t.Fatalf("collectBooks() returned %d books, want 2: %#v", len(books), books)
	}

	assertBook(t, books[0], "Dune", "/Dune.epub", "", "EPUB")
	assertBook(t, books[1], "The Left - Hand of Darkness", "/Sci-Fi/The_Left-Hand_of_Darkness.pdf", "Sci-Fi", "PDF")
}

func TestCollectBooksWithDotRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Dune.epub"))

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q) error = %v", root, err)
	}

	books, err := collectBooks(".")
	if err != nil {
		t.Fatalf("collectBooks() error = %v", err)
	}

	if len(books) != 1 {
		t.Fatalf("collectBooks() returned %d books, want 1: %#v", len(books), books)
	}
	assertBook(t, books[0], "Dune", "/Dune.epub", "", "EPUB")
}

func TestURLPath(t *testing.T) {
	got := urlPath(filepath.Join("Reference", "C++ Primer.pdf"))
	want := "/Reference/C++%20Primer.pdf"
	if string(got) != want {
		t.Fatalf("urlPath() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertBook(t *testing.T, book Book, name, href, folder, format string) {
	t.Helper()

	if book.Name != name {
		t.Errorf("Name = %q, want %q", book.Name, name)
	}
	if string(book.Href) != href {
		t.Errorf("Href = %q, want %q", book.Href, href)
	}
	if book.Folder != folder {
		t.Errorf("Folder = %q, want %q", book.Folder, folder)
	}
	if book.Format != format {
		t.Errorf("Format = %q, want %q", book.Format, format)
	}
}

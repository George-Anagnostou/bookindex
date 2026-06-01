package main

import (
	"archive/zip"
	"bytes"
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

func TestCollectBooksSkipsGeneratedAssets(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "Dune.epub"))
	writeFile(t, filepath.Join(root, generatedAssetsDir, "covers", "generated.pdf"))

	books, err := collectBooks(root)
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

func TestCollectBooksWithEpubCover(t *testing.T) {
	root := t.TempDir()
	writeEPUB(t, filepath.Join(root, "Dune.epub"), "OPS/images/cover.jpg", []byte("cover image"))

	books, err := collectBooksWithCovers(root, coverOptions{
		Enabled:  true,
		CacheDir: defaultCoverCacheDir(root),
	})
	if err != nil {
		t.Fatalf("collectBooksWithCovers() error = %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("collectBooksWithCovers() returned %d books, want 1", len(books))
	}
	if books[0].Cover == "" {
		t.Fatal("Cover is empty")
	}
	if got := string(books[0].Cover); filepath.Ext(got) != ".jpg" {
		t.Fatalf("Cover = %q, want .jpg path", got)
	}
}

func TestEPUBCoverImageUsesManifestCover(t *testing.T) {
	cover := []byte("cover image")
	zr := testEPUBReader(t, "OPS/images/cover.jpg", cover)

	got, ext, err := epubCoverImage(zr)
	if err != nil {
		t.Fatalf("epubCoverImage() error = %v", err)
	}
	if !bytes.Equal(got, cover) {
		t.Fatalf("epubCoverImage() bytes = %q, want %q", got, cover)
	}
	if ext != ".jpg" {
		t.Fatalf("epubCoverImage() ext = %q, want .jpg", ext)
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

func writeEPUB(t *testing.T, path, coverPath string, cover []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	writeZipFile(t, zw, "META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">
	<rootfiles>
		<rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/>
	</rootfiles>
</container>`))
	writeZipFile(t, zw, "OPS/content.opf", []byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
	<metadata>
		<meta name="cover" content="cover-image"/>
	</metadata>
	<manifest>
		<item id="cover-image" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
	</manifest>
</package>`))
	writeZipFile(t, zw, coverPath, cover)

	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
}

func testEPUBReader(t *testing.T, coverPath string, cover []byte) *zip.Reader {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">
	<rootfiles>
		<rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/>
	</rootfiles>
</container>`))
	writeZipFile(t, zw, "OPS/content.opf", []byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
	<metadata>
		<meta name="cover" content="cover-image"/>
	</metadata>
	<manifest>
		<item id="cover-image" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
	</manifest>
</package>`))
	writeZipFile(t, zw, coverPath, cover)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip NewReader() error = %v", err)
	}
	return zr
}

func writeZipFile(t *testing.T, zw *zip.Writer, name string, content []byte) {
	t.Helper()

	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip Create(%q) error = %v", name, err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip Write(%q) error = %v", name, err)
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

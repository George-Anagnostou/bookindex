package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Book struct {
	Name     string
	Href     template.URL
	Cover    template.URL
	Folder   string
	Format   string
	Size     string
	Modified string
}

type coverOptions struct {
	Enabled  bool
	CacheDir string
}

type PageData struct {
	Title       string
	GeneratedAt string
	Count       int
	Books       []Book
}

var bookExts = map[string]bool{
	".pdf":  true,
	".epub": true,
}

const generatedAssetsDir = "_bookindex"

func main() {
	var root string
	var title string
	var out string

	flag.StringVar(&root, "root", "/srv/books", "book root directory")
	flag.StringVar(&title, "title", "Private Library", "page title")
	flag.StringVar(&out, "out", "", "output HTML path; defaults to <root>/index.html")
	flag.Parse()

	if out == "" {
		out = filepath.Join(root, "index.html")
	}

	books, err := collectBooksWithCovers(root, coverOptions{
		Enabled:  true,
		CacheDir: defaultCoverCacheDir(root),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect books: %v\n", err)
		os.Exit(1)
	}

	data := PageData{
		Title:       title,
		GeneratedAt: time.Now().Format("2006-01-02 15:04"),
		Count:       len(books),
		Books:       books,
	}

	if err := writeIndex(out, data); err != nil {
		fmt.Fprintf(os.Stderr, "write index: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s with %d books\n", out, len(books))
}

func collectBooks(root string) ([]Book, error) {
	return collectBooksWithCovers(root, coverOptions{})
}

func collectBooksWithCovers(root string, covers coverOptions) ([]Book, error) {
	var books []Book

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := d.Name()

		if d.IsDir() {
			if path != root && name == generatedAssetsDir {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if name == "index.html" || strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if !bookExts[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		folder := filepath.Dir(rel)
		if folder == "." {
			folder = ""
		}

		modTime := info.ModTime()
		cover := template.URL("")
		if covers.Enabled {
			cover = coverForBook(root, path, rel, ext, info, covers.CacheDir)
		}

		books = append(books, Book{
			Name:     displayName(name),
			Href:     urlPath(rel),
			Cover:    cover,
			Folder:   folder,
			Format:   strings.TrimPrefix(strings.ToUpper(ext), "."),
			Size:     humanSize(info.Size()),
			Modified: modTime.Format("2006-01-02"),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(books, func(i, j int) bool {
		if strings.ToLower(books[i].Folder) == strings.ToLower(books[j].Folder) {
			return strings.ToLower(books[i].Name) < strings.ToLower(books[j].Name)
		}
		return strings.ToLower(books[i].Folder) < strings.ToLower(books[j].Folder)
	})

	return books, nil
}

func displayName(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " - ")
	base = strings.Join(strings.Fields(base), " ")
	return base
}

func urlPath(rel string) template.URL {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return template.URL("/" + strings.Join(parts, "/"))
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}

	div, exp := int64(unit), 0
	for n >= div*unit && exp < len(units) {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

func coverForBook(root, bookPath, rel, ext string, info fs.FileInfo, cacheDir string) template.URL {
	if cacheDir == "" {
		cacheDir = defaultCoverCacheDir(root)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return ""
	}

	key := coverCacheKey(rel, info)

	var coverPath string
	var err error
	switch ext {
	case ".pdf":
		coverPath, err = pdfCover(bookPath, cacheDir, key)
	case ".epub":
		coverPath, err = epubCover(bookPath, cacheDir, key)
	default:
		return ""
	}
	if err != nil || coverPath == "" {
		return ""
	}

	coverRel, err := filepath.Rel(root, coverPath)
	if err != nil {
		return ""
	}
	return urlPath(coverRel)
}

func defaultCoverCacheDir(root string) string {
	return filepath.Join(root, generatedAssetsDir, "covers")
}

func coverCacheKey(rel string, info fs.FileInfo) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())))
	return hex.EncodeToString(sum[:16])
}

func pdfCover(bookPath, cacheDir, key string) (string, error) {
	outBase := filepath.Join(cacheDir, key)
	outPath := outBase + ".jpg"
	if fileExists(outPath) {
		return outPath, nil
	}

	tmpBase := filepath.Join(cacheDir, key+".tmp")
	tmpPath := tmpBase + ".jpg"
	_ = os.Remove(tmpPath)

	cmd := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-jpeg", "-jpegopt", "quality=82", "-r", "96", bookPath, tmpBase)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return outPath, nil
}

func epubCover(bookPath, cacheDir, key string) (string, error) {
	zr, err := zip.OpenReader(bookPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	image, ext, err := epubCoverImage(&zr.Reader)
	if err != nil {
		return "", err
	}

	outPath := filepath.Join(cacheDir, key+ext)
	if fileExists(outPath) {
		return outPath, nil
	}

	tmpPath := filepath.Join(cacheDir, key+".tmp"+ext)
	if err := os.WriteFile(tmpPath, image, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return outPath, nil
}

func epubCoverImage(zr *zip.Reader) ([]byte, string, error) {
	containerBytes, err := zipReadFile(zr, "META-INF/container.xml")
	if err != nil {
		return nil, "", err
	}

	var container epubContainer
	if err := xml.Unmarshal(containerBytes, &container); err != nil {
		return nil, "", err
	}
	if container.Rootfile.FullPath == "" {
		return nil, "", fmt.Errorf("epub rootfile not found")
	}

	opfBytes, err := zipReadFile(zr, container.Rootfile.FullPath)
	if err != nil {
		return nil, "", err
	}

	var pkg epubPackage
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return nil, "", err
	}

	item := pkg.coverItem()
	if item.Href == "" {
		return nil, "", fmt.Errorf("epub cover image not found")
	}

	imagePath := path.Clean(path.Join(path.Dir(container.Rootfile.FullPath), item.Href))
	imageBytes, err := zipReadFile(zr, imagePath)
	if err != nil {
		return nil, "", err
	}

	ext := imageExtension(item.Href, item.MediaType)
	return imageBytes, ext, nil
}

type epubContainer struct {
	Rootfile struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Metadata struct {
		Metas []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []epubManifestItem `xml:"item"`
	} `xml:"manifest"`
}

type epubManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

func (pkg epubPackage) coverItem() epubManifestItem {
	coverID := ""
	for _, meta := range pkg.Metadata.Metas {
		if strings.EqualFold(meta.Name, "cover") {
			coverID = meta.Content
			break
		}
	}
	if coverID != "" {
		for _, item := range pkg.Manifest.Items {
			if item.ID == coverID {
				return item
			}
		}
	}

	for _, item := range pkg.Manifest.Items {
		if strings.Contains(" "+item.Properties+" ", " cover-image ") {
			return item
		}
	}

	for _, item := range pkg.Manifest.Items {
		if strings.HasPrefix(strings.ToLower(item.MediaType), "image/") {
			return item
		}
	}

	return epubManifestItem{}
}

func zipReadFile(zr *zip.Reader, name string) ([]byte, error) {
	cleanName := path.Clean(name)
	for _, file := range zr.File {
		if path.Clean(file.Name) != cleanName {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("zip file %q not found", name)
}

func imageExtension(href, mediaType string) string {
	ext := strings.ToLower(path.Ext(href))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return ext
	}

	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".img"
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeIndex(out string, data PageData) error {
	tmp := out + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := pageTemplate.Execute(f, data); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, out)
}

var pageTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>{{.Title}}</title>
		<style>
			:root {
				--bg: #fafafa;
				--fg: #111;
				--muted: #555;
				--line: #ddd;
				--card: #fff;
			}

			body {
				margin: 0;
				padding: 0.75rem;
				max-width: 760px;
				font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
				background: var(--bg);
				color: var(--fg);
				line-height: 1.25;
			}

			header {
				margin-bottom: 0.55rem;
				border-bottom: 1px solid var(--line);
			}

			h1 {
				font-size: 1rem;
				margin: 0 0 0.15rem 0;
			}

			.summary {
				color: var(--muted);
				font-size: 0.72rem;
				margin-bottom: 0.45rem;
			}

			.search {
				position: sticky;
				top: 0;
				z-index: 1;
				margin: 0 -0.75rem 0.65rem;
				padding: 0.55rem 0.75rem;
				background: color-mix(in srgb, var(--bg) 92%, transparent);
				backdrop-filter: blur(8px);
				border-bottom: 1px solid var(--line);
			}

			input {
				width: 100%;
				box-sizing: border-box;
				padding: 0.58rem 0.65rem;
				border: 1px solid var(--line);
				border-radius: 0.35rem;
				font-size: 1rem;
				background: white;
			}

			ul {
				list-style: none;
				padding: 0;
				margin: 0;
			}

			li {
				background: var(--card);
				border: 1px solid var(--line);
				border-radius: 0.25rem;
				padding: 0.5rem;
				margin-bottom: 0.45rem;
				display: grid;
				grid-template-columns: 5.5rem minmax(0, 1fr);
				gap: 0.65rem;
				align-items: start;
			}

			a {
				color: var(--fg);
				text-decoration: none;
			}

			.cover-link {
				display: block;
				width: 5.5rem;
				aspect-ratio: 2 / 3;
				border-radius: 0.2rem;
				overflow: hidden;
				background: #eee;
				border: 1px solid var(--line);
			}

			.cover-link:hover {
				text-decoration: none;
			}

			.cover {
				display: block;
				width: 100%;
				height: 100%;
				object-fit: cover;
			}

			.cover-placeholder {
				width: 100%;
				height: 100%;
				display: grid;
				place-items: center;
				color: var(--muted);
				background: linear-gradient(135deg, #f4f4f4, #e6e6e6);
				font-size: 0.72rem;
				font-weight: 700;
				letter-spacing: 0;
			}

			.book-info {
				min-width: 0;
				padding-top: 0.05rem;
			}

			.title {
				font-weight: 600;
				font-size: 0.98rem;
				overflow-wrap: anywhere;
			}

			.title:hover {
				text-decoration: underline;
			}

			.meta {
				margin-top: 0.22rem;
				color: var(--muted);
				font-size: 0.76rem;
				display: flex;
				gap: 0.35rem;
				flex-wrap: wrap;
			}

			.folder {
				font-style: italic;
			}

			@media (min-width: 560px) {
				body {
					padding: 1rem;
				}

				.search {
					margin-left: -1rem;
					margin-right: -1rem;
					padding-left: 1rem;
					padding-right: 1rem;
				}

				li {
					grid-template-columns: 6.25rem minmax(0, 1fr);
				}

				.cover-link {
					width: 6.25rem;
				}
			}
		</style>
	</head>
	<body>
		<header>
			<h1>{{.Title}}</h1>
			<div class="summary">{{.Count}} books generated {{.GeneratedAt}}</div>
		</header>
	
	<div class="search">
		<input id="q" type="search" placeholder="Filter books...">
	</div>

	<ul id="books">
		{{range .Books}}
		<li>
			<a class="cover-link" href="{{.Href}}" aria-label="{{.Name}}">
				{{if .Cover}}<img class="cover" src="{{.Cover}}" alt="" loading="lazy">{{else}}<span class="cover-placeholder">{{.Format}}</span>{{end}}
			</a>
			<div class="book-info">
				<a class="title" href="{{.Href}}">{{.Name}}</a>
				<div class="meta">
					{{if .Folder}}<span class="folder">{{.Folder}}</span><span>-</span>{{end}}
					<span>{{.Format}}</span>
					<span>-</span>
					<span>{{.Size}}</span>
					<span>-</span>
					<span>modified {{.Modified}}</span>
				</div>
			</div>
		</li>
		{{end}}
	</ul>

	<script>
		const q = document.getElementById("q");
		const items = [...document.querySelectorAll("#books li")];

		q.addEventListener("input", () => {
			const needle = q.value.trim().toLowerCase();

			for (const item of items) {
				item.style.display = item.textContent.toLowerCase().includes(needle) ? "" : "none";
			}
		});
	</script>
</body>
</html>
`))

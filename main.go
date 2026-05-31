package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Book struct {
	Name     string
	Href     template.URL
	Folder   string
	Format   string
	Size     string
	Modified string
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

	books, err := collectBooks(root)
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
	var books []Book

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := d.Name()

		if d.IsDir() {
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
		books = append(books, Book{
			Name:     displayName(name),
			Href:     urlPath(rel),
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
				padding: 1rem;
				max-width: 900px;
				font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
				background: var(--bg);
				color: var(--fg);
				line-height: 1.25;
			}

			header {
				margin-bottom: 0.8rem;
				border-bottom: 1px solid var(--line);
			}

			h1 {
				font-size: 1.35rem;
				margin: 0 0 0.25rem 0;
			}

			.summary {
				color: var(--muted);
				font-size: 0.82rem;
				margin-bottom: 0.65rem;
			}

			.search {
				margin: 0 0 0.75rem;
			}

			input {
				width: 100%;
				box-sizing: border-box;
				padding: 0.5rem 0.6rem;
				border: 1px solid var(--line);
				border-radius: 0.35rem;
				font-size: 0.95rem;
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
				padding: 0.42rem 0.55rem;
				margin-bottom: 0.25rem;
			}

			a {
				color: var(--fg);
				text-decoration: none;
				font-weight: 600;
				font-size: 0.95rem;
			}

			a:hover {
				text-decoration: underline;
			}

			.meta {
				margin-top: 0.12rem;
				color: var(--muted);
				font-size: 0.76rem;
				display: flex;
				gap: 0.35rem;
				flex-wrap: wrap;
			}

			.folder {
				font-style: italic;
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
			<a href="{{.Href}}">{{.Name}}</a>
			<div class="meta">
				{{if .Folder}}<span class="folder">{{.Folder}}</span><span>-</span>{{end}}
				<span>{{.Format}}</span>
				<span>-</span>
				<span>{{.Size}}</span>
				<span>-</span>
				<span>modified {{.Modified}}</span>
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

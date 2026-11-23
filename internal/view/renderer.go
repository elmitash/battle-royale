package view

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"battle-royale/internal/model"
)

// Renderer parses and executes HTML templates with common layout and helper functions.
type Renderer struct {
	templates map[string]*template.Template
}

// NewRenderer loads layout.html paired with each page template in dir.
func NewRenderer(dir string) (*Renderer, error) {
	funcMap := template.FuncMap{
		"formatDur":  model.FormatDurability,
		"formatKind": model.FormatItemKind,
		"formatTime": func(ts int64) string {
			if ts <= 0 {
				return "-"
			}
			return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
		},
		"add": func(a, b int) int { return a + b },
	}

	layoutPath := filepath.Join(dir, "layout.html")
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return nil, fmt.Errorf("glob templates: %w", err)
	}

	tmpls := make(map[string]*template.Template)
	for _, p := range pages {
		name := filepath.Base(p)
		if name == "layout.html" {
			continue
		}
		t, err := template.New("layout.html").Funcs(funcMap).ParseFiles(layoutPath, p)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		tmpls[name] = t
	}
	return &Renderer{templates: tmpls}, nil
}

// Render writes the named page template into w.
func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	t, ok := r.templates[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

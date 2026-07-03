// Package seed creates the built-in template projects on startup.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/ladderlogix/typstpad/internal/store"
)

//go:embed templates
var templatesFS embed.FS

type templateMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	MainPath    string `json:"mainPath,omitempty"`
}

// Templates seeds built-in templates, owned by the first admin user. No-op
// until a user exists or when a template of the same name is already present.
func Templates(ctx context.Context, st *store.Store) error {
	var ownerID string
	err := st.Pool.QueryRow(ctx,
		`SELECT id FROM users WHERE is_admin ORDER BY created_at LIMIT 1`).Scan(&ownerID)
	if err != nil {
		return nil // no admin yet; caller retries after first registration
	}

	dirs, err := templatesFS.ReadDir("templates")
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		if err := seedOne(ctx, st, ownerID, dir.Name()); err != nil {
			slog.Error("template seed failed", "template", dir.Name(), "err", err)
		}
	}
	return nil
}

func seedOne(ctx context.Context, st *store.Store, ownerID, slug string) error {
	base := "templates/" + slug
	metaBytes, err := templatesFS.ReadFile(base + "/template.json")
	if err != nil {
		return fmt.Errorf("missing template.json: %w", err)
	}
	var meta templateMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return err
	}

	metaJSON, _ := json.Marshal(map[string]string{"description": meta.Description, "category": meta.Category, "builtin": slug})

	var existingID string
	err = st.Pool.QueryRow(ctx, `
		SELECT id FROM projects WHERE is_template AND name=$1 AND deleted_at IS NULL LIMIT 1`,
		meta.Name).Scan(&existingID)
	if err == nil {
		// Already seeded — just refresh its metadata (e.g. add category).
		_, uerr := st.Pool.Exec(ctx, `UPDATE projects SET template_meta=$2 WHERE id=$1`, existingID, metaJSON)
		return uerr
	}

	p, err := st.CreateProject(ctx, meta.Name, meta.Description, ownerID)
	if err != nil {
		return err
	}
	if meta.MainPath != "" {
		if err := st.UpdateProject(ctx, p.ID, nil, nil, &meta.MainPath); err != nil {
			return err
		}
	}
	err = fs.WalkDir(templatesFS, base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "template.json") {
			return err
		}
		rel := strings.TrimPrefix(path, base+"/")
		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = st.CreateTextFile(ctx, p.ID, rel, string(data))
		return err
	})
	if err != nil {
		return err
	}
	if err := st.SetProjectTemplate(ctx, p.ID, true, metaJSON); err != nil {
		return err
	}
	slog.Info("seeded template", "name", meta.Name)
	return nil
}

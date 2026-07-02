// Package versions implements the version-history snapshotter and restore.
package versions

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"typstpad/internal/blob"
	"typstpad/internal/collab"
	"typstpad/internal/store"
)

const (
	idleWindow  = 2 * time.Minute  // snapshot after this much quiet
	maxWindow   = 15 * time.Minute // ...or at most this long after first change
	tickEvery   = 30 * time.Second
	maxVersions = 500
)

type Snapshotter struct {
	Store   *store.Store
	Blob    *blob.Store
	Collab  *collab.Client
	Publish func(projectID string, eventType string)

	mu    sync.Mutex
	dirty map[string]*dirtyState
}

type dirtyState struct {
	first, last time.Time
}

func NewSnapshotter(st *store.Store, bl *blob.Store, cc *collab.Client, publish func(string, string)) *Snapshotter {
	return &Snapshotter{Store: st, Blob: bl, Collab: cc, Publish: publish, dirty: map[string]*dirtyState{}}
}

// MarkDirty records write activity on a project (called on every sidecar
// persistence callback and on file-tree mutations).
func (sn *Snapshotter) MarkDirty(projectID string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	d := sn.dirty[projectID]
	if d == nil {
		sn.dirty[projectID] = &dirtyState{first: time.Now(), last: time.Now()}
		return
	}
	d.last = time.Now()
}

func (sn *Snapshotter) Run(ctx context.Context) {
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sn.sweep(ctx)
		}
	}
}

func (sn *Snapshotter) sweep(ctx context.Context) {
	now := time.Now()
	var due []string
	sn.mu.Lock()
	for id, d := range sn.dirty {
		if now.Sub(d.last) >= idleWindow || now.Sub(d.first) >= maxWindow {
			due = append(due, id)
			delete(sn.dirty, id)
		}
	}
	sn.mu.Unlock()
	for _, projectID := range due {
		if _, err := sn.Snapshot(ctx, projectID, "auto", nil, nil); err != nil {
			slog.Error("auto-snapshot failed", "project", projectID, "err", err)
		}
	}
}

// Snapshot captures the current content of every project file. Returns
// (nil, nil) when content is unchanged since the latest snapshot.
func (sn *Snapshotter) Snapshot(ctx context.Context, projectID, kind string, name, createdBy *string) (*store.Snapshot, error) {
	files, err := sn.Store.ListFiles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	manifest := make([]store.SnapshotFile, 0, len(files))
	for _, f := range files {
		var hash []byte
		switch f.Kind {
		case "text":
			text, err := sn.currentText(ctx, f.ID)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", f.Path, err)
			}
			hash, err = sn.Blob.Put([]byte(text))
			if err != nil {
				return nil, err
			}
			if err := sn.Store.UpsertBlob(ctx, hash, int64(len(text))); err != nil {
				return nil, err
			}
		case "asset":
			hash = f.BlobHash
			if err := sn.Store.UpsertBlob(ctx, hash, f.Size); err != nil {
				return nil, err
			}
		}
		manifest = append(manifest, store.SnapshotFile{Path: f.Path, Kind: f.Kind, ContentHash: hash})
	}

	projectHash := manifestHash(manifest)
	if prev, err := sn.Store.LatestSnapshotHash(ctx, projectID); err != nil {
		return nil, err
	} else if kind == "auto" && string(prev) == string(projectHash) {
		return nil, nil
	}

	snap, err := sn.Store.CreateSnapshot(ctx, projectID, kind, name, createdBy, projectHash, manifest)
	if err != nil {
		return nil, err
	}
	if sn.Publish != nil {
		sn.Publish(projectID, "versions.changed")
	}
	return snap, nil
}

func manifestHash(files []store.SnapshotFile) []byte {
	sorted := append([]store.SnapshotFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s:%x\n", f.Path, f.ContentHash)
	}
	return h.Sum(nil)
}

func (sn *Snapshotter) currentText(ctx context.Context, fileID string) (string, error) {
	text, err := sn.Collab.Text(ctx, fileID, true)
	if err == nil {
		return text, nil
	}
	return sn.Store.FileContent(ctx, fileID)
}

// Restore brings the project's files back to a snapshot's state. Text files
// are restored through the CRDT so live editors merge instead of diverging.
// A pre_restore snapshot is taken first.
func (sn *Snapshotter) Restore(ctx context.Context, projectID, snapshotID string, restoredBy string) error {
	snap, err := sn.Store.SnapshotByID(ctx, snapshotID)
	if err != nil {
		return err
	}
	if snap.ProjectID != projectID {
		return store.ErrNotFound
	}
	if _, err := sn.Snapshot(ctx, projectID, "pre_restore", nil, &restoredBy); err != nil {
		return fmt.Errorf("pre-restore snapshot: %w", err)
	}

	snapFiles, err := sn.Store.SnapshotFiles(ctx, snapshotID)
	if err != nil {
		return err
	}
	current, err := sn.Store.ListFiles(ctx, projectID)
	if err != nil {
		return err
	}
	currentByPath := map[string]*store.File{}
	for _, f := range current {
		currentByPath[f.Path] = f
	}

	for _, sf := range snapFiles {
		data, err := sn.Blob.Get(sf.ContentHash)
		if err != nil {
			return fmt.Errorf("blob for %s: %w", sf.Path, err)
		}
		cur := currentByPath[sf.Path]
		delete(currentByPath, sf.Path)
		switch {
		case cur == nil && sf.Kind == "text":
			if _, err := sn.Store.CreateTextFile(ctx, projectID, sf.Path, string(data)); err != nil {
				return err
			}
		case cur == nil && sf.Kind == "asset":
			if err := sn.Store.UpsertBlob(ctx, sf.ContentHash, int64(len(data))); err != nil {
				return err
			}
			if _, err := sn.Store.CreateAssetFile(ctx, projectID, sf.Path, sf.ContentHash, "", int64(len(data))); err != nil {
				return err
			}
		case cur.Kind != sf.Kind:
			if err := sn.Store.DeleteFile(ctx, cur.ID); err != nil {
				return err
			}
			if sf.Kind == "text" {
				if _, err := sn.Store.CreateTextFile(ctx, projectID, sf.Path, string(data)); err != nil {
					return err
				}
			} else {
				if _, err := sn.Store.CreateAssetFile(ctx, projectID, sf.Path, sf.ContentHash, cur.Mime, int64(len(data))); err != nil {
					return err
				}
			}
		case sf.Kind == "text":
			if err := sn.Collab.SetContent(ctx, cur.ID, string(data)); err != nil {
				return fmt.Errorf("restore %s: %w", sf.Path, err)
			}
		case sf.Kind == "asset":
			if string(cur.BlobHash) != string(sf.ContentHash) {
				if err := sn.Store.DeleteFile(ctx, cur.ID); err != nil {
					return err
				}
				if _, err := sn.Store.CreateAssetFile(ctx, projectID, sf.Path, sf.ContentHash, cur.Mime, int64(len(data))); err != nil {
					return err
				}
			}
		}
	}
	// Anything left in currentByPath does not exist in the snapshot.
	for _, f := range currentByPath {
		if err := sn.Store.DeleteFile(ctx, f.ID); err != nil {
			return err
		}
	}
	if sn.Publish != nil {
		sn.Publish(projectID, "files.changed")
		sn.Publish(projectID, "versions.changed")
	}
	return nil
}

package planned

import (
	"context"
	"encoding/json"
	"fmt"
	stdpath "path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/fs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

func init() {
	RegisterAction("backup", "Incrementally backup a source folder (with all subfolders and files) into a folder of the same name under the target", actionBackup)
}

type backupParams struct {
	Source string      `json:"source"` // mount path of the source directory
	Target string      `json:"target"` // mount path of the target directory
	Mode   string      `json:"mode"`   // "copy" (default) | "mirror"
	Rules  BackupRules `json:"rules"`
}

// backupFile is one source file discovered during traversal.
type backupFile struct {
	RelPath string    // path relative to the source directory
	Name    string    // base name
	Size    int64     // bytes
	ModTime time.Time
	SrcPath string    // full mount path (source + RelPath)
}

// IsDangerousTask reports whether a planned task's backup action may destroy
// data (mirror mode, delete_source completion, or delete deletion rule), so
// the API layer can require a second confirmation before a real run.
func IsDangerousTask(t *model.PlannedTask) bool {
	if t.Action != "backup" {
		return false
	}
	var p backupParams
	if err := json.Unmarshal([]byte(t.Params), &p); err != nil {
		return false
	}
	return p.Mode == "mirror" || p.Rules.IsDangerous()
}

func actionBackup(t *model.PlannedTask, params map[string]interface{}, dryRun bool) (string, error) {
	var p backupParams
	if err := decodeParams(params, &p); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}
	if p.Source == "" || p.Target == "" {
		return "", fmt.Errorf("source and target are required")
	}
	if p.Mode == "" {
		p.Mode = "copy"
	}
	if p.Mode != "copy" && p.Mode != "mirror" {
		return "", fmt.Errorf("mode %q is not supported (only \"copy\" or \"mirror\")", p.Mode)
	}
	if err := p.Rules.validate(); err != nil {
		return "", err
	}
	filters, err := p.Rules.Filters.compile()
	if err != nil {
		return "", err
	}

	ctx := actionContext(t)
	srcStorage, srcActual, err := op.GetStorageAndActualPath(p.Source)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source %q: %w", p.Source, err)
	}
	dstStorage, dstActual, err := op.GetStorageAndActualPath(p.Target)
	if err != nil {
		return "", fmt.Errorf("failed to resolve target %q: %w", p.Target, err)
	}

	// The source folder itself (its last path segment) is recreated under the
	// target, so the backup keeps the folder's identity: picking source
	// /Movies and target /Backup stores files under /Backup/Movies/... .
	// A source at the root ("/") is copied into the target directly.
	backupRoot := p.Target
	dstRootActual := dstActual
	if w := stdpath.Base(p.Source); w != "" && w != "/" && w != "." {
		backupRoot = stdpath.Join(p.Target, w)
		dstRootActual = stdpath.Join(dstActual, w)
	}

	files, err := collectFiles(ctx, srcStorage, srcActual, p.Source, "")
	if err != nil {
		return "", err
	}

	// build the copy plan: apply filters, then the replacement rule
	var toCopy []backupFile
	var skipped, filteredOut int
	for _, f := range files {
		incl, err := filters.shouldInclude(f.RelPath, f.Name, f.Size)
		if err != nil {
			return "", err
		}
		if !incl {
			filteredOut++
			continue
		}
		existing, _ := op.Get(ctx, dstStorage, stdpath.Join(dstRootActual, f.RelPath), true)
		if existing != nil && !existing.IsDir() {
			if p.Rules.Replacement.Action == ReplacementOverwrite &&
				!(existing.GetSize() == f.Size && existing.ModTime().Equal(f.ModTime)) {
				// different -> overwrite (copy)
			} else {
				// skip (identical, or skip policy)
				skipped++
				continue
			}
		}
		toCopy = append(toCopy, f)
	}

	// diff deletion plan: files on the target that no longer exist in the
	// source (mirror mode or deletion.delete). The diff is computed against
	// the *actual* source files (unfiltered), so excluded files still present
	// in the source are never deleted on the target.
	deleteTargets := p.Mode == "mirror" || p.Rules.Deletion.Action == DeletionDelete
	var toDelete []string
	if deleteTargets {
		dstFiles, err := collectFiles(ctx, dstStorage, dstRootActual, backupRoot, "")
		if err != nil {
			return "", err
		}
		srcSet := make(map[string]bool, len(files))
		for _, f := range files {
			srcSet[f.RelPath] = true
		}
		for _, d := range dstFiles {
			if !srcSet[d.RelPath] {
				toDelete = append(toDelete, d.RelPath)
			}
		}
	}

	deleteSource := p.Rules.Completion.Action == CompletionDeleteSource

	var b strings.Builder
	fmt.Fprintf(&b, "backup %q -> %q: scanned=%d, filtered_out=%d, skipped=%d, to_copy=%d, to_delete=%d",
		p.Source, backupRoot, len(files), filteredOut, skipped, len(toCopy), len(toDelete))

	if dryRun {
		b.WriteString("\ndry-run (nothing copied/deleted):")
		for _, f := range toCopy {
			fmt.Fprintf(&b, "\n  + %s (%d bytes)", f.RelPath, f.Size)
		}
		for _, rel := range toDelete {
			fmt.Fprintf(&b, "\n  - %s (would delete from target)", rel)
		}
		if deleteSource {
			for _, f := range toCopy {
				fmt.Fprintf(&b, "\n  - %s (would delete from source after copy)", f.RelPath)
			}
		}
		return b.String(), nil
	}

	// execute copy; record the files that completed synchronously so that
	// delete_source can safely remove them (async transfers are left alone).
	submitted := 0
	var copied []backupFile
	for _, f := range toCopy {
		dstDir := stdpath.Join(backupRoot, stdpath.Dir(f.RelPath))
		info, err := fs.Copy(ctx, f.SrcPath, dstDir)
		if err != nil {
			return "", fmt.Errorf("failed to copy %q: %w", f.RelPath, err)
		}
		submitted++
		if info == nil {
			copied = append(copied, f)
		}
	}
	b.WriteString(fmt.Sprintf("\ncopied %d file(s)", submitted))

	// execute target-side diff deletion
	deleted := 0
	for _, rel := range toDelete {
		if err := fs.Remove(ctx, stdpath.Join(backupRoot, rel)); err != nil {
			return "", fmt.Errorf("failed to delete target %q: %w", rel, err)
		}
		deleted++
	}
	if deleted > 0 {
		b.WriteString(fmt.Sprintf("\ndeleted %d file(s) from target", deleted))
	}

	// delete_source: remove source files that were successfully copied
	if deleteSource {
		removed := 0
		for _, f := range copied {
			if err := fs.Remove(ctx, f.SrcPath); err != nil {
				return "", fmt.Errorf("failed to delete source %q: %w", f.RelPath, err)
			}
			removed++
		}
		if removed > 0 {
			b.WriteString(fmt.Sprintf("\ndeleted %d source file(s)", removed))
		}
		if removed < submitted {
			b.WriteString(fmt.Sprintf(" (%d copied asynchronously, source kept)", submitted-removed))
		}
	}

	return b.String(), nil
}

// collectFiles recursively lists source files under actualPath, recording
// their mount-relative path and full mount path for later fs.Copy calls.
func collectFiles(ctx context.Context, storage driver.Driver, actualPath, srcMount, rel string) ([]backupFile, error) {
	objs, err := op.List(ctx, storage, actualPath, model.ListArgs{Refresh: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list [%s]: %w", actualPath, err)
	}
	var out []backupFile
	for _, obj := range objs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relPath := stdpath.Join(rel, obj.GetName())
		if obj.IsDir() {
			sub, err := collectFiles(ctx, storage, stdpath.Join(actualPath, obj.GetName()), srcMount, relPath)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		} else {
			out = append(out, backupFile{
				RelPath: relPath,
				Name:    obj.GetName(),
				Size:    obj.GetSize(),
				ModTime: obj.ModTime(),
				SrcPath: stdpath.Join(srcMount, relPath),
			})
		}
	}
	return out, nil
}

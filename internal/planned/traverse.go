package planned

import (
	"fmt"
	stdpath "path"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

func init() {
	RegisterAction("traverse", "Visit every folder under a path so its listings/metadata are loaded (optionally bounded by depth)", actionTraverse)
}

type traverseParams struct {
	Path  string `json:"path"`  // mount path of the root folder to traverse
	Depth int    `json:"depth"` // max folder levels to descend; 0 = unlimited
}

// actionTraverse walks the directory tree under p.Path, calling op.List on each
// directory so the storage's listings/metadata are populated. It performs no
// read/download/mutation beyond listing — a "warm-up"/cache-refresh visit.
func actionTraverse(t *model.PlannedTask, params map[string]interface{}, dryRun bool) (string, error) {
	var p traverseParams
	if err := decodeParams(params, &p); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if p.Depth < 0 {
		return "", fmt.Errorf("depth must be >= 0 (0 = unlimited)")
	}

	ctx := actionContext(t)
	storage, actual, err := op.GetStorageAndActualPath(p.Path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %q: %w", p.Path, err)
	}

	var dirsVisited, filesSeen int
	visitedPaths := []string{}
	const sampleMax = 20

	var walk func(actualPath string, level int) error
	walk = func(actualPath string, level int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		objs, err := op.List(ctx, storage, actualPath, model.ListArgs{Refresh: true})
		if err != nil {
			return fmt.Errorf("failed to list [%s]: %w", actualPath, err)
		}
		dirsVisited++
		if len(visitedPaths) < sampleMax {
			visitedPaths = append(visitedPaths, actualPath)
		}
		for _, obj := range objs {
			if obj.IsDir() {
				if p.Depth > 0 && level >= p.Depth {
					continue
				}
				if err := walk(stdpath.Join(actualPath, obj.GetName()), level+1); err != nil {
					return err
				}
			} else {
				filesSeen++
			}
		}
		return nil
	}

	if dryRun {
		// A dry run still has to list to know what it would touch; report the
		// root and stop to avoid doing the full (potentially huge) walk.
		if _, err := op.List(ctx, storage, actual, model.ListArgs{Refresh: true}); err != nil {
			return "", fmt.Errorf("failed to list [%s]: %w", actual, err)
		}
		scope := "all levels (unlimited)"
		if p.Depth > 0 {
			scope = fmt.Sprintf("up to depth %d", p.Depth)
		}
		return fmt.Sprintf("dry-run: would traverse %q (%s)", p.Path, scope), nil
	}

	if err := walk(actual, 1); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "traversed %q: visited %d director", p.Path, dirsVisited)
	if dirsVisited == 1 {
		b.WriteString("y")
	} else {
		b.WriteString("ies")
	}
	fmt.Fprintf(&b, ", saw %d file(s) total", filesSeen)
	if p.Depth > 0 {
		fmt.Fprintf(&b, " (depth limited to %d)", p.Depth)
	}
	return b.String(), nil
}

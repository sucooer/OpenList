package strm

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	log "github.com/sirupsen/logrus"
)

// Safety guards for sync-mode deletion of the local strm tree.
//
// The hook in hook.go mirrors cloud listings into the local tree. In sync
// mode a listing that comes back short — a failed storage, a flapping
// driver, a partial page — used to delete every local file the listing did
// not mention, with no way back. The guards below make that impossible:
//
//  1. Incomplete listings suspend deletion (markIncomplete /
//     hasRecentIncomplete), and an empty listing never deletes.
//  2. A path must be missing from two listings at least
//     syncConfirmInterval apart (noteMissing).
//  3. Each batch is capped (deletionOverCap).
//  4. Only files the driver manages are touched (isStrmFileName /
//     isManagedSibling); shared files such as poster.jpg are never
//     candidates, and directories are never removed wholesale.
//  5. Candidates are quarantined under .trash, never deleted
//     (quarantineFile / quarantineStaleDir).
//
// All guards are always on; there is no configuration. Insert and update
// modes never reach this code.

const (
	// trashDirName is the quarantine directory inside SaveStrmLocalPath.
	// It is never treated as sync content.
	trashDirName = ".trash"
	// syncDeleteCapMin is the floor of the per-batch deletion cap: small
	// directories can always sync a handful of deletions.
	syncDeleteCapMin = 20
	// syncDeleteCapPercent caps each batch at this percent of the
	// directory's entries (above the floor).
	syncDeleteCapPercent = 10
)

var (
	// syncIncompleteTTL suspends sync deletion for this long after any
	// source of the listing fails.
	syncIncompleteTTL = 5 * time.Minute
	// syncConfirmInterval is the minimum time a path must stay missing
	// before it is quarantined.
	syncConfirmInterval = 10 * time.Minute
	// syncMissingEntryMaxAge bounds the missing-path memory: entries
	// older than this are dropped when the map grows large.
	syncMissingEntryMaxAge = 24 * time.Hour
)

// syncSiblingSuffixes are the extensions a sidecar file may have to be
// quarantined together with its .strm file.
var syncSiblingSuffixes = map[string]struct{}{
	"nfo": {}, "json": {}, "srt": {}, "ass": {}, "vtt": {},
	"sub": {}, "jpg": {}, "jpeg": {}, "png": {}, "tbn": {},
}

// markIncomplete records that a source of root failed to list.
func (d *Strm) markIncomplete(root string) {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()
	if d.incompleteMarks == nil {
		d.incompleteMarks = make(map[string]time.Time)
	}
	d.incompleteMarks[root] = time.Now()
}

// hasRecentIncomplete reports whether sync deletion must be suspended: a
// source of rootKey failed within syncIncompleteTTL. An empty rootKey
// matches any recent mark, failing closed when the root is unknown.
func (d *Strm) hasRecentIncomplete(rootKey string) bool {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()
	now := time.Now()
	if rootKey != "" {
		if t, ok := d.incompleteMarks[rootKey]; ok {
			if now.Sub(t) < syncIncompleteTTL {
				return true
			}
			delete(d.incompleteMarks, rootKey)
		}
		return false
	}
	for root, t := range d.incompleteMarks {
		if now.Sub(t) < syncIncompleteTTL {
			return true
		}
		delete(d.incompleteMarks, root)
	}
	return false
}

// noteMissing records that path was absent from a listing and reports
// whether it has been missing long enough to act on: the first sighting
// only records, a later sighting at least syncConfirmInterval after the
// first confirms.
func (d *Strm) noteMissing(path string) bool {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()
	if d.missingSince == nil {
		d.missingSince = make(map[string]time.Time)
	}
	now := time.Now()
	if len(d.missingSince) > 4096 {
		for p, t := range d.missingSince {
			if now.Sub(t) > syncMissingEntryMaxAge {
				delete(d.missingSince, p)
			}
		}
	}
	if first, ok := d.missingSince[path]; ok {
		return now.Sub(first) >= syncConfirmInterval
	}
	d.missingSince[path] = now
	return false
}

// clearMissing forgets a path, called when it shows up in a listing again
// or after it has been quarantined.
func (d *Strm) clearMissing(path string) {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()
	delete(d.missingSince, path)
}

// rootKeyForSource resolves a trie source path to its pathMap root key.
// It returns "" when the source is not in the map.
func (d *Strm) rootKeyForSource(sourcePath string) string {
	want := utils.FixAndCleanPath(sourcePath)
	for key, dsts := range d.pathMap {
		for _, dst := range dsts {
			if utils.FixAndCleanPath(dst) == want {
				return key
			}
		}
	}
	return ""
}

// isStrmFileName reports whether name is a .strm file (any case).
func isStrmFileName(name string) bool {
	const suffix = ".strm"
	return len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}

// strmStem returns the stem of a .strm file name ("Movie" for
// "Movie.strm").
func strmStem(name string) (string, bool) {
	if !isStrmFileName(name) {
		return "", false
	}
	return name[:len(name)-len(".strm")], true
}

// isManagedSibling reports whether name is a sidecar of one of the given
// .strm stems: it must share the stem (stem + "." or stem + "-") and have
// a whitelisted extension. Folder-shared files such as poster.jpg never
// match a stem and are never managed.
func isManagedSibling(name string, stems map[string]struct{}) bool {
	if len(stems) == 0 {
		return false
	}
	if _, ok := syncSiblingSuffixes[strings.ToLower(utils.SourceExt(name))]; !ok {
		return false
	}
	for stem := range stems {
		if stem == "" {
			continue
		}
		if strings.HasPrefix(name, stem+".") || strings.HasPrefix(name, stem+"-") {
			return true
		}
	}
	return false
}

// deletionOverCap reports whether candidates exceeds the per-batch safety
// cap for a directory with entries entries: at most syncDeleteCapMin, or
// syncDeleteCapPercent of the directory, whichever is larger.
func deletionOverCap(candidates, entries int) bool {
	if candidates <= 0 {
		return false
	}
	cap := syncDeleteCapMin
	if byRatio := (entries*syncDeleteCapPercent + 99) / 100; byRatio > cap {
		cap = byRatio
	}
	return candidates > cap
}

// quarantineDest computes the quarantine destination for fullPath,
// preserving its path relative to the local strm root under a dated
// directory. Paths outside the root are refused.
func quarantineDest(localRoot, fullPath string, now time.Time) (string, error) {
	rel, err := filepath.Rel(localRoot, fullPath)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside strm local root %q", fullPath, localRoot)
	}
	return filepath.Join(localRoot, trashDirName, now.Format("20060102"), rel), nil
}

// quarantineFile moves a managed file into the quarantine directory,
// preserving its relative path. Nothing is ever deleted.
func (d *Strm) quarantineFile(fullPath string) {
	dst, err := quarantineDest(d.SaveStrmLocalPath, fullPath, time.Now())
	if err != nil {
		log.Errorf("strm sync: cannot quarantine %s: %v", fullPath, err)
		return
	}
	if err := createLocalDirectory(filepath.Dir(dst)); err != nil {
		log.Errorf("strm sync: cannot create quarantine directory for %s: %v", fullPath, err)
		return
	}
	if _, err := os.Lstat(dst); err == nil {
		// Never overwrite an earlier quarantined copy.
		dst = dst + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if err := os.Rename(fullPath, dst); err != nil {
		log.Errorf("strm sync: failed to quarantine %s: %v", fullPath, err)
		return
	}
	d.clearMissing(fullPath)
	log.Infof("strm sync: quarantined %s -> %s", fullPath, dst)
}

// quarantineStaleDir quarantines the managed contents of a directory that
// is confirmed absent from the cloud listing, recursing with the same
// rules. Children inherit the directory's confirmation. The directory
// itself is removed only if nothing is left in it; foreign files anchor
// their directories.
func (d *Strm) quarantineStaleDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Errorf("strm sync: cannot read stale directory %s: %v", dir, err)
		return
	}
	stems := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if stem, ok := strmStem(entry.Name()); ok {
			stems[stem] = struct{}{}
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == trashDirName {
			continue
		}
		fullPath := filepath.Join(dir, name)
		if entry.IsDir() {
			d.quarantineStaleDir(fullPath)
			continue
		}
		if isStrmFileName(name) || isManagedSibling(name, stems) {
			d.quarantineFile(fullPath)
		}
	}
	rest, err := os.ReadDir(dir)
	if err != nil || len(rest) > 0 {
		return
	}
	if err := os.Remove(dir); err != nil {
		log.Warnf("strm sync: cannot remove emptied directory %s: %v", dir, err)
		return
	}
	log.Infof("strm sync: removed emptied directory %s", dir)
}

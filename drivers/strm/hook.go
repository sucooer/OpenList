package strm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	stdpath "path"
	"path/filepath"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/tchap/go-patricia/v2/patricia"
)

var strmTrie = patricia.NewTrie()

func UpdateLocalStrm(ctx context.Context, path string, objs []model.Obj) {
	path = utils.FixAndCleanPath(path)
	updateLocal := func(driver *Strm, basePath string, rootKey string, objs []model.Obj) {
		relParent := strings.TrimPrefix(basePath, utils.GetActualMountPath(driver.MountPath))
		localParentPath := stdpath.Join(driver.SaveStrmLocalPath, relParent)
		if err := createLocalDirectory(localParentPath); err != nil {
			log.Warnf("failed to create local strm directory %s: %v", localParentPath, err)
			return
		}
		for _, obj := range objs {
			localPath := stdpath.Join(localParentPath, obj.GetName())
			generateStrm(ctx, driver, obj, localPath)
		}
		deleteExtraFiles(driver, localParentPath, objs, rootKey)
	}

	_ = strmTrie.VisitPrefixes(patricia.Prefix(path), func(needPathPrefix patricia.Prefix, item patricia.Item) error {
		strmDrivers := item.([]*Strm)
		needPath := string(needPathPrefix)
		restPath := strings.TrimPrefix(path, needPath)
		if len(restPath) > 0 && restPath[0] != '/' {
			return nil
		}
		for _, strmDriver := range strmDrivers {
			strmObjs := strmDriver.convert2strmObjs(ctx, path, objs)
			updateLocal(strmDriver, stdpath.Join(stdpath.Base(needPath), restPath), strmDriver.rootKeyForSource(needPath), strmObjs)
		}
		return nil
	})
}

func InsertStrm(dstPath string, d *Strm) error {
	prefix := patricia.Prefix(strings.TrimRight(dstPath, "/"))
	existing := strmTrie.Get(prefix)

	if existing == nil {
		if !strmTrie.Insert(prefix, []*Strm{d}) {
			return errors.New("failed to insert strm")
		}
		return nil
	}
	if lst, ok := existing.([]*Strm); ok {
		strmTrie.Set(prefix, append(lst, d))
	} else {
		return errors.New("invalid trie item type")
	}

	return nil
}

func RemoveStrm(dstPath string, d *Strm) {
	prefix := patricia.Prefix(strings.TrimRight(dstPath, "/"))
	existing := strmTrie.Get(prefix)
	if existing == nil {
		return
	}
	lst, ok := existing.([]*Strm)
	if !ok {
		return
	}
	if len(lst) == 1 && lst[0] == d {
		strmTrie.Delete(prefix)
		return
	}

	for i, di := range lst {
		if di == d {
			newList := append(lst[:i], lst[i+1:]...)
			strmTrie.Set(prefix, newList)
			return
		}
	}
}

func generateStrm(ctx context.Context, driver *Strm, obj model.Obj, localPath string) {
	if obj.IsDir() {
		if err := createLocalDirectory(localPath); err != nil {
			log.Warnf("failed to create local strm directory %s: %v", localPath, err)
		}
		return
	}

	if utils.Exists(localPath) && driver.SaveLocalMode == SaveLocalInsertMode {
		return
	}
	link, err := driver.Link(ctx, obj, model.LinkArgs{})
	if err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to link: %v", localPath, err)
		return
	}
	defer link.Close()
	size := link.ContentLength
	if size <= 0 {
		size = obj.GetSize()
	}
	rrf, err := stream.GetRangeReaderFromLink(size, link)
	if err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to get range reader: %v", localPath, err)
		return
	}
	rc, err := rrf.RangeRead(ctx, http_range.Range{Length: -1})
	if err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to read range: %v", localPath, err)
		return
	}
	defer rc.Close()
	same, err := isSameContent(localPath, size, rc)
	if err != nil {
		log.Warnf("failed to compare content of obj %s: %v", localPath, err)
		return
	}
	if same {
		return
	}
	rc, err = rrf.RangeRead(ctx, http_range.Range{Length: -1})
	if err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to reread range: %v", localPath, err)
		return
	}
	defer rc.Close()
	if err := createLocalDirectory(filepath.Dir(localPath)); err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to create parent directory: %v", localPath, err)
		return
	}
	file, err := os.Create(localPath)
	if err != nil {
		log.Warnf("failed to generate strm of obj %s: failed to create local file: %v", localPath, err)
		return
	}
	defer file.Close()
	if _, err := utils.CopyWithBuffer(file, rc); err != nil {
		log.Warnf("failed to generate strm of obj %s: copy failed: %v", localPath, err)
	}
}

func createLocalDirectory(path string) error {
	return os.MkdirAll(path, 0o777)
}

func isSameContent(localPath string, size int64, rc io.Reader) (bool, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if info.Size() != size {
		return false, nil
	}
	localFile, err := os.Open(localPath)
	if err != nil {
		return false, err
	}
	defer localFile.Close()
	h1 := sha256.New()
	h2 := sha256.New()
	if _, err := io.Copy(h1, localFile); err != nil {
		return false, err
	}
	if _, err := io.Copy(h2, rc); err != nil {
		return false, err
	}
	return bytes.Equal(h1.Sum(nil), h2.Sum(nil)), nil
}

// deleteExtraFiles mirrors cloud deletions into the local strm tree, but
// only in sync mode and only behind the guards in sync_guard.go: it never
// deletes on an empty or recently-incomplete listing, it requires a path
// to be missing from two listings, it caps each batch, it only touches
// files the driver manages, and it quarantines instead of deleting.
func deleteExtraFiles(driver *Strm, localPath string, objs []model.Obj, rootKey string) {
	if driver.SaveLocalMode != SaveLocalSyncMode {
		return
	}
	entries, err := os.ReadDir(localPath)
	if err != nil {
		log.Errorf("Failed to read local files from %s: %v", localPath, err)
		return
	}
	// An empty listing must never wipe a non-empty directory: a driver
	// returning no entries with no error is a failure, not a deletion.
	if len(objs) == 0 && len(entries) > 0 {
		log.Warnf("strm sync: refusing deletion in %s: empty listing for a non-empty directory", localPath)
		return
	}
	if driver.hasRecentIncomplete(rootKey) {
		log.Warnf("strm sync: skipping deletion in %s: an incomplete listing was observed recently", localPath)
		return
	}
	presentFiles := make(map[string]struct{}, len(objs))
	presentDirs := make(map[string]struct{}, len(objs))
	for _, obj := range objs {
		objPath := filepath.Join(localPath, obj.GetName())
		driver.clearMissing(objPath)
		if obj.IsDir() {
			presentDirs[objPath] = struct{}{}
		} else {
			presentFiles[objPath] = struct{}{}
		}
	}
	var missingStrm, missingDirs []string
	for _, entry := range entries {
		name := entry.Name()
		if name == trashDirName {
			continue
		}
		fullPath := filepath.Join(localPath, name)
		if entry.IsDir() {
			if _, ok := presentDirs[fullPath]; !ok {
				missingDirs = append(missingDirs, fullPath)
			}
			continue
		}
		if _, ok := presentFiles[fullPath]; ok {
			continue
		}
		if isStrmFileName(name) {
			missingStrm = append(missingStrm, fullPath)
		}
	}
	var eligibleStrm, eligibleDirs []string
	for _, p := range missingStrm {
		if driver.noteMissing(p) {
			eligibleStrm = append(eligibleStrm, p)
		}
	}
	for _, p := range missingDirs {
		if driver.noteMissing(p) {
			eligibleDirs = append(eligibleDirs, p)
		}
	}
	decisions := len(eligibleStrm) + len(eligibleDirs)
	if decisions == 0 {
		return
	}
	if deletionOverCap(decisions, len(entries)) {
		log.Warnf("strm sync: refusing deletion in %s: %d candidates exceed the safety cap; remove them manually if the cloud deletion was intended", localPath, decisions)
		return
	}
	stems := make(map[string]struct{}, len(eligibleStrm))
	for _, p := range eligibleStrm {
		if stem, ok := strmStem(filepath.Base(p)); ok {
			stems[stem] = struct{}{}
		}
	}
	// Sidecar files ride along with their .strm file only.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(localPath, name)
		if _, ok := presentFiles[fullPath]; ok {
			continue
		}
		if isStrmFileName(name) {
			continue
		}
		if isManagedSibling(name, stems) {
			driver.quarantineFile(fullPath)
		}
	}
	for _, p := range eligibleStrm {
		driver.quarantineFile(p)
	}
	for _, p := range eligibleDirs {
		driver.quarantineStaleDir(p)
	}
}

func init() {
	op.RegisterObjsUpdateHook(UpdateLocalStrm)
}

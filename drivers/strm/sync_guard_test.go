package strm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func TestIsStrmFileName(t *testing.T) {
	cases := map[string]bool{
		"Movie.strm":  true,
		"Movie.STRM":  true,
		"Movie.Strm":  true,
		"Movie.mkv":   false,
		"Movie.strm2": false,
		".strm":       false,
		"strm":        false,
		"":            false,
	}
	for name, want := range cases {
		if got := isStrmFileName(name); got != want {
			t.Errorf("isStrmFileName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestStrmStem(t *testing.T) {
	stem, ok := strmStem("Movie.strm")
	if !ok || stem != "Movie" {
		t.Errorf("strmStem(Movie.strm) = %q, %v, want Movie, true", stem, ok)
	}
	if _, ok := strmStem("Movie.mkv"); ok {
		t.Errorf("strmStem(Movie.mkv) unexpectedly ok")
	}
}

func TestIsManagedSibling(t *testing.T) {
	stems := map[string]struct{}{"Movie": {}}
	cases := map[string]bool{
		"Movie.nfo":            true,
		"Movie-mediainfo.json": true,
		"Movie.zh.srt":         true,
		"Movie.tbn":            true,
		"Movie-poster.jpg":     true,
		"poster.jpg":           false,
		"fanart.jpg":           false,
		"Movie2.nfo":           false,
		"Movie.mkv":            false,
		"Movie":                false,
		"Movie.txt":            false,
	}
	for name, want := range cases {
		if got := isManagedSibling(name, stems); got != want {
			t.Errorf("isManagedSibling(%q) = %v, want %v", name, got, want)
		}
	}
	if isManagedSibling("Movie.nfo", nil) {
		t.Errorf("isManagedSibling with no stems should be false")
	}
}

func TestDeletionOverCap(t *testing.T) {
	cases := []struct {
		candidates, entries int
		want                bool
	}{
		{0, 100, false},
		{1, 5, false},
		{20, 30, false},
		{21, 30, true},
		{100, 1000, false},
		{101, 1000, true},
		{5, 10, false},
	}
	for _, c := range cases {
		if got := deletionOverCap(c.candidates, c.entries); got != c.want {
			t.Errorf("deletionOverCap(%d, %d) = %v, want %v", c.candidates, c.entries, got, c.want)
		}
	}
}

func TestQuarantineDest(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	dst, err := quarantineDest("/media/strm", "/media/strm/Movies/Movie.strm", now)
	if err != nil {
		t.Fatalf("quarantineDest() error = %v", err)
	}
	want := filepath.Join("/media/strm", ".trash", "20261115", "Movies", "Movie.strm")
	if dst != want {
		t.Errorf("quarantineDest() = %q, want %q", dst, want)
	}
	if _, err := quarantineDest("/media/strm", "/etc/passwd", now); err == nil {
		t.Errorf("quarantineDest() outside root should fail")
	}
}

func TestRootKeyForSource(t *testing.T) {
	d := &Strm{pathMap: map[string][]string{
		"139家庭云": {"/移动/139家庭云"},
		"alias":  {"/real/path"},
	}}
	if got := d.rootKeyForSource("/移动/139家庭云"); got != "139家庭云" {
		t.Errorf("rootKeyForSource() = %q, want 139家庭云", got)
	}
	if got := d.rootKeyForSource("/real/path"); got != "alias" {
		t.Errorf("rootKeyForSource() = %q, want alias", got)
	}
	if got := d.rootKeyForSource("/nope"); got != "" {
		t.Errorf("rootKeyForSource() = %q, want empty", got)
	}
}

func TestMarkIncomplete(t *testing.T) {
	d := &Strm{}
	d.markIncomplete("root1")
	if !d.hasRecentIncomplete("root1") {
		t.Errorf("hasRecentIncomplete(root1) should be true right after marking")
	}
	if !d.hasRecentIncomplete("") {
		t.Errorf("hasRecentIncomplete(\"\") should fail closed on any recent mark")
	}
	if d.hasRecentIncomplete("other") {
		t.Errorf("hasRecentIncomplete(other) should be false for an unmarked root")
	}
	orig := syncIncompleteTTL
	syncIncompleteTTL = 0
	defer func() { syncIncompleteTTL = orig }()
	if d.hasRecentIncomplete("root1") {
		t.Errorf("hasRecentIncomplete(root1) should be false once the mark expires")
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDeleteExtraFilesSyncFlow(t *testing.T) {
	orig := syncConfirmInterval
	syncConfirmInterval = 0
	defer func() { syncConfirmInterval = orig }()

	local := filepath.Join(t.TempDir(), "strm")
	writeTestFile(t, filepath.Join(local, "keep.strm"))
	writeTestFile(t, filepath.Join(local, "gone.strm"))
	writeTestFile(t, filepath.Join(local, "gone-mediainfo.json"))
	writeTestFile(t, filepath.Join(local, "gone.nfo"))
	writeTestFile(t, filepath.Join(local, "poster.jpg"))
	writeTestFile(t, filepath.Join(local, "Old", "old.strm"))
	writeTestFile(t, filepath.Join(local, "Old", "old.nfo"))
	writeTestFile(t, filepath.Join(local, "Old", "keep.txt"))
	writeTestFile(t, filepath.Join(local, "GoneDir", "gone2.strm"))
	writeTestFile(t, filepath.Join(local, "Live", "live.strm"))

	d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: SaveLocalSyncMode}}
	objs := []model.Obj{
		&model.Object{Name: "keep.strm"},
		&model.Object{Name: "Live", IsFolder: true},
	}

	// First sighting only records; nothing moves.
	deleteExtraFiles(d, local, objs, "")
	if _, err := os.Stat(filepath.Join(local, "gone.strm")); err != nil {
		t.Fatalf("gone.strm moved on first sighting: %v", err)
	}

	// Second sighting confirms and quarantines.
	deleteExtraFiles(d, local, objs, "")
	trash := filepath.Join(local, ".trash", time.Now().Format("20060102"))
	for _, p := range []string{"gone.strm", "gone-mediainfo.json", "gone.nfo",
		filepath.Join("Old", "old.strm"), filepath.Join("Old", "old.nfo"),
		filepath.Join("GoneDir", "gone2.strm")} {
		if _, err := os.Stat(filepath.Join(trash, p)); err != nil {
			t.Errorf("expected %s quarantined under .trash: %v", p, err)
		}
		if _, err := os.Stat(filepath.Join(local, p)); !os.IsNotExist(err) {
			t.Errorf("expected %s moved away from the tree", p)
		}
	}
	// Foreign and live files stay put.
	for _, p := range []string{"keep.strm", "poster.jpg",
		filepath.Join("Old", "keep.txt"), filepath.Join("Live", "live.strm")} {
		if _, err := os.Stat(filepath.Join(local, p)); err != nil {
			t.Errorf("expected %s to stay: %v", p, err)
		}
	}
	// Old stays because keep.txt anchors it; GoneDir is removed once empty.
	if _, err := os.Stat(filepath.Join(local, "Old")); err != nil {
		t.Errorf("Old dir should stay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "GoneDir")); !os.IsNotExist(err) {
		t.Errorf("GoneDir should be removed once emptied")
	}
}

func TestDeleteExtraFilesReappearingClearsRecord(t *testing.T) {
	orig := syncConfirmInterval
	syncConfirmInterval = 0
	defer func() { syncConfirmInterval = orig }()

	local := filepath.Join(t.TempDir(), "strm")
	writeTestFile(t, filepath.Join(local, "flap.strm"))
	d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: SaveLocalSyncMode}}
	absent := []model.Obj{&model.Object{Name: "other.strm"}}
	present := []model.Obj{&model.Object{Name: "flap.strm"}}

	deleteExtraFiles(d, local, absent, "")
	deleteExtraFiles(d, local, present, "")
	// Absent again: re-recorded, not yet confirmed.
	deleteExtraFiles(d, local, absent, "")
	if _, err := os.Stat(filepath.Join(local, "flap.strm")); err != nil {
		t.Fatalf("flap.strm should survive a reappearance cycle: %v", err)
	}
}

func TestDeleteExtraFilesEmptyListing(t *testing.T) {
	orig := syncConfirmInterval
	syncConfirmInterval = 0
	defer func() { syncConfirmInterval = orig }()

	local := filepath.Join(t.TempDir(), "strm")
	writeTestFile(t, filepath.Join(local, "a.strm"))
	d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: SaveLocalSyncMode}}
	deleteExtraFiles(d, local, nil, "")
	deleteExtraFiles(d, local, nil, "")
	if _, err := os.Stat(filepath.Join(local, "a.strm")); err != nil {
		t.Fatalf("empty listing must never delete: %v", err)
	}
}

func TestDeleteExtraFilesIncompleteSuspends(t *testing.T) {
	orig := syncConfirmInterval
	syncConfirmInterval = 0
	defer func() { syncConfirmInterval = orig }()

	local := filepath.Join(t.TempDir(), "strm")
	writeTestFile(t, filepath.Join(local, "a.strm"))
	d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: SaveLocalSyncMode}}
	d.markIncomplete("root1")
	objs := []model.Obj{&model.Object{Name: "other.strm"}}
	deleteExtraFiles(d, local, objs, "root1")
	deleteExtraFiles(d, local, objs, "root1")
	deleteExtraFiles(d, local, objs, "")
	if _, err := os.Stat(filepath.Join(local, "a.strm")); err != nil {
		t.Fatalf("incomplete listing must suspend deletion: %v", err)
	}
}

func TestDeleteExtraFilesCap(t *testing.T) {
	orig := syncConfirmInterval
	syncConfirmInterval = 0
	defer func() { syncConfirmInterval = orig }()

	local := filepath.Join(t.TempDir(), "strm")
	var objs []model.Obj
	for i := 0; i < 30; i++ {
		writeTestFile(t, filepath.Join(local, fmt.Sprintf("f%d.strm", i)))
	}
	for i := 0; i < 5; i++ {
		objs = append(objs, &model.Object{Name: fmt.Sprintf("f%d.strm", i)})
	}
	d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: SaveLocalSyncMode}}
	// 25 missing candidates exceed the cap of max(20, 10% of 30) = 20.
	deleteExtraFiles(d, local, objs, "")
	deleteExtraFiles(d, local, objs, "")
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(filepath.Join(local, fmt.Sprintf("f%d.strm", i))); err != nil {
			t.Fatalf("cap must refuse the whole batch: %v", err)
		}
	}
}

func TestDeleteExtraFilesNonSyncUnchanged(t *testing.T) {
	for _, mode := range []string{SaveLocalInsertMode, SaveLocalUpdateMode, ""} {
		local := filepath.Join(t.TempDir(), "strm")
		writeTestFile(t, filepath.Join(local, "a.strm"))
		d := &Strm{Addition: Addition{SaveStrmLocalPath: local, SaveLocalMode: mode}}
		deleteExtraFiles(d, local, []model.Obj{&model.Object{Name: "other.strm"}}, "")
		if _, err := os.Stat(filepath.Join(local, "a.strm")); err != nil {
			t.Fatalf("mode %q must not delete: %v", mode, err)
		}
	}
}

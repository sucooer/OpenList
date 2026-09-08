package planned

import (
	"testing"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"0B", 0, false},
		{"512", 512, false},
		{"1KB", 1 << 10, false},
		{"1MB", 1 << 20, false},
		{"1.5MB", int64(1.5 * (1 << 20)), false},
		{"1GB", 1 << 30, false},
		{"2tb", 2 << 40, false},
		{"10 MiB", 10 << 20, false},
		{"", 0, true},
		{"10XB", 0, true},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseSize(%q) expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMatchExtension(t *testing.T) {
	cases := []struct {
		value string
		name  string
		want  bool
	}{
		{"mkv", "movie.mkv", true},
		{"mkv,mp4", "movie.mp4", true},
		{"mkv,mp4", "movie.avi", false},
		{"MKV", "movie.mkv", true},
		{"mkv", "movie", false},
		{"mkv", ".mkv", true},
	}
	for _, c := range cases {
		if got := matchExtension(c.value, c.name); got != c.want {
			t.Errorf("matchExtension(%q, %q) = %v, want %v", c.value, c.name, got, c.want)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
		err     bool
	}{
		{"*.sample.*", "movie.sample.mkv", true, false},
		{"*.sample.*", "movie.mkv", false, false},
		{"Thumbs.db", "Thumbs.db", true, false},
		{"!*.mkv", "movie.mkv", false, false},
		{"!*.mkv", "movie.avi", true, false},
		{"[", "x", false, true}, // invalid glob
	}
	for _, c := range cases {
		got, err := matchGlob(c.pattern, c.name)
		if c.err {
			if err == nil {
				t.Errorf("matchGlob(%q, %q) expected error", c.pattern, c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("matchGlob(%q, %q) unexpected error: %v", c.pattern, c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestShouldIncludePriority(t *testing.T) {
	// include "*.mkv", exclude "*.sample.*": sample mkv is excluded (exclude wins)
	fs := FilterSet{
		Include: []FilterItem{{Type: FilterTypeExtension, Value: "mkv"}},
		Exclude: []FilterItem{{Type: FilterTypeFilename, Value: "*.sample.*"}},
	}
	compiled, err := fs.compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cases := []struct {
		name string
		want bool
	}{
		{"movie.mkv", true},
		{"movie.sample.mkv", false},
		{"movie.mp4", false}, // not included
	}
	for _, c := range cases {
		got, err := compiled.shouldInclude(c.name, c.name, 0)
		if err != nil {
			t.Fatalf("shouldInclude(%q): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("shouldInclude(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldIncludeEmptyIncludeMeansAll(t *testing.T) {
	// empty include list -> all included, except excluded
	fs := FilterSet{
		Exclude: []FilterItem{{Type: FilterTypeFilename, Value: "Thumbs.db"}},
	}
	compiled, err := fs.compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, _ := compiled.shouldInclude("a/Thumbs.db", "Thumbs.db", 0); got {
		t.Errorf("Thumbs.db should be excluded")
	}
	if got, _ := compiled.shouldInclude("a/movie.mkv", "movie.mkv", 0); !got {
		t.Errorf("movie.mkv should be included")
	}
}

func TestRegexFilter(t *testing.T) {
	fs := FilterSet{
		Exclude: []FilterItem{{Type: FilterTypeRegex, Value: `^\..*`}}, // hidden files
	}
	compiled, err := fs.compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, _ := compiled.shouldInclude(".hidden", ".hidden", 0); got {
		t.Errorf("hidden file should be excluded")
	}
	if got, _ := compiled.shouldInclude("visible.txt", "visible.txt", 0); !got {
		t.Errorf("visible file should be included")
	}
}

func TestSizeFilter(t *testing.T) {
	cases := []struct {
		item FilterItem
		size int64
		want bool
	}{
		{FilterItem{Type: FilterTypeSize, Value: "0B"}, 0, true},          // exact: empty file
		{FilterItem{Type: FilterTypeSize, Value: "0B"}, 1, false},         // exact: non-empty
		{FilterItem{Type: FilterTypeSize, Value: "1MB", SizeOp: "min"}, 2 << 20, true},
		{FilterItem{Type: FilterTypeSize, Value: "1MB", SizeOp: "min"}, 512, false},
		{FilterItem{Type: FilterTypeSize, Value: "10GB", SizeOp: "max"}, 5 << 30, true},
		{FilterItem{Type: FilterTypeSize, Value: "10GB", SizeOp: "max"}, 20 << 30, false},
		{FilterItem{Type: FilterTypeSize, Value: "1MB", SizeOp: "between", SizeMax: "5MB"}, 3 << 20, true},
		{FilterItem{Type: FilterTypeSize, Value: "1MB", SizeOp: "between", SizeMax: "5MB"}, 10 << 20, false},
	}
	for _, c := range cases {
		compiled, err := compileFilter(c.item, false)
		if err != nil {
			t.Fatalf("compileFilter(%+v): %v", c.item, err)
		}
		got, err := compiled.match("f", "f", c.size)
		if err != nil {
			t.Fatalf("match: %v", err)
		}
		if got != c.want {
			t.Errorf("item %+v size=%d = %v, want %v", c.item, c.size, got, c.want)
		}
	}
}

func TestBackupRulesValidate(t *testing.T) {
	// safe defaults pass
	if err := (BackupRules{}).validate(); err != nil {
		t.Errorf("default rules should validate, got %v", err)
	}
	// dangerous-but-supported rules now validate (M5)
	if err := (BackupRules{Completion: CompletionRule{Action: CompletionDeleteSource}}).validate(); err != nil {
		t.Errorf("delete_source completion should validate, got %v", err)
	}
	if err := (BackupRules{Deletion: DeletionRule{Action: DeletionDelete}}).validate(); err != nil {
		t.Errorf("delete deletion rule should validate, got %v", err)
	}
	// still-unimplemented rules rejected
	if err := (BackupRules{Completion: CompletionRule{Action: CompletionArchiveSource}}).validate(); err == nil {
		t.Errorf("archive_source completion should be rejected")
	}
	if err := (BackupRules{Replacement: ReplacementRule{Action: ReplacementRename}}).validate(); err == nil {
		t.Errorf("rename replacement should be rejected")
	}
	// overwrite is allowed
	if err := (BackupRules{Replacement: ReplacementRule{Action: ReplacementOverwrite}}).validate(); err != nil {
		t.Errorf("overwrite replacement should validate, got %v", err)
	}
}

func TestBackupRulesIsDangerous(t *testing.T) {
	if (BackupRules{}).IsDangerous() {
		t.Errorf("default rules should not be dangerous")
	}
	if !(BackupRules{Completion: CompletionRule{Action: CompletionDeleteSource}}).IsDangerous() {
		t.Errorf("delete_source completion should be dangerous")
	}
	if !(BackupRules{Deletion: DeletionRule{Action: DeletionDelete}}).IsDangerous() {
		t.Errorf("delete deletion rule should be dangerous")
	}
	if (BackupRules{Completion: CompletionRule{Action: CompletionNone}}).IsDangerous() {
		t.Errorf("none completion should not be dangerous")
	}
}

func TestIsDangerousTask(t *testing.T) {
	task := func(action, params string) *model.PlannedTask {
		return &model.PlannedTask{Action: action, Params: params}
	}
	cases := []struct {
		name string
		task *model.PlannedTask
		want bool
	}{
		{"non-backup action", task("webhook", `{}`), false},
		{"copy no rules", task("backup", `{"mode":"copy"}`), false},
		{"mirror mode", task("backup", `{"mode":"mirror"}`), true},
		{"delete_source completion", task("backup", `{"mode":"copy","rules":{"completion":{"action":"delete_source"}}}`), true},
		{"deletion delete", task("backup", `{"mode":"copy","rules":{"deletion":{"action":"delete"}}}`), true},
		{"safe copy with keep", task("backup", `{"mode":"copy","rules":{"deletion":{"action":"keep"},"completion":{"action":"none"}}}`), false},
		{"invalid json", task("backup", `{not json`), false},
	}
	for _, c := range cases {
		if got := IsDangerousTask(c.task); got != c.want {
			t.Errorf("%s: IsDangerousTask = %v, want %v", c.name, got, c.want)
		}
	}
}

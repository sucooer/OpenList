package planned

import (
	"fmt"
	stdpath "path"
	"regexp"
	"strconv"
	"strings"
)

// Filter types (see docs/planned-task-design.md §12.5).
const (
	FilterTypeExtension = "extension"
	FilterTypeFilename  = "filename"
	FilterTypeRegex     = "regex"
	FilterTypeSize      = "size"
)

// Replacement rule actions (§12.4).
const (
	ReplacementSkip      = "skip"
	ReplacementOverwrite = "overwrite"
	ReplacementRename    = "rename"
	ReplacementRefuse    = "refuse"
)

// Completion rule actions (§12.2).
const (
	CompletionNone          = "none"
	CompletionKeepSource    = "keep_source" // alias of none
	CompletionDeleteSource  = "delete_source"
	CompletionArchiveSource = "archive_source"
)

// Deletion rule actions (§12.3).
const (
	DeletionKeep   = "keep"
	DeletionDelete = "delete"
)

// FilterItem is one include/exclude filter rule.
type FilterItem struct {
	Type    string `json:"type"`     // extension | filename | regex | size
	Op      string `json:"op"`       // include | exclude (informational; the array decides)
	Value   string `json:"value"`    // extension: "mkv,mp4" / filename: "*.sample.*" / regex: "^\\..*" / size: "10MB"
	SizeOp  string `json:"size_op"`  // size only: "" (exact) | min | max | between
	SizeMax string `json:"size_max"` // size only: upper bound for "between"
}

// FilterSet holds the include/exclude rule lists of a backup task.
type FilterSet struct {
	Include []FilterItem `json:"include"`
	Exclude []FilterItem `json:"exclude"`
}

// CompletionRule describes what to do with source files after a successful
// backup (§12.2). "none"/"keep_source" (keep source) and "delete_source"
// (remove successfully-backed-up source files) are implemented.
type CompletionRule struct {
	Action string `json:"action"` // none (default) | keep_source | delete_source
}

// DeletionRule describes how to handle target-side files missing from the
// source (§12.3). "keep" (default) and "delete" (mirror-style diff removal)
// are implemented.
type DeletionRule struct {
	Action          string `json:"action"`           // keep (default) | delete
	KeepLast        int    `json:"keep_last"`        // reserved
	RetentionDays   int    `json:"retention_days"`   // reserved
	DeleteEmptyDirs bool   `json:"delete_empty_dirs"` // reserved
}

// ReplacementRule describes how to handle a target file that already exists
// (§12.4). "skip" (default) and "overwrite" are implemented.
type ReplacementRule struct {
	Action string `json:"action"` // skip (default) | overwrite
}

// BackupRules is the rules object of a backup task (§12.1).
type BackupRules struct {
	Completion  CompletionRule  `json:"completion"`
	Deletion    DeletionRule    `json:"deletion"`
	Replacement ReplacementRule `json:"replacement"`
	Filters     FilterSet       `json:"filters"`
}

// validate rejects rules that are declared in the design but not yet
// implemented (the archive_source completion rule is deferred; mirror
// mode and delete/delete_source are handled by the backup action).
func (r BackupRules) validate() error {
	switch r.Completion.Action {
	case "", CompletionNone, CompletionKeepSource, CompletionDeleteSource:
	default:
		return fmt.Errorf("completion rule %q is not supported yet", r.Completion.Action)
	}
	switch r.Deletion.Action {
	case "", DeletionKeep, DeletionDelete:
	default:
		return fmt.Errorf("deletion rule %q is not supported yet", r.Deletion.Action)
	}
	switch r.Replacement.Action {
	case "", ReplacementSkip, ReplacementOverwrite:
	default:
		return fmt.Errorf("replacement rule %q is not supported yet", r.Replacement.Action)
	}
	return nil
}

// IsDangerous reports whether the rules involve a destructive operation
// (delete_source completion or delete deletion rule) that requires a second
// confirmation before a real (non dry-run) execution.
func (r BackupRules) IsDangerous() bool {
	return r.Completion.Action == CompletionDeleteSource ||
		r.Deletion.Action == DeletionDelete
}

// compiledFilter is a filter item with its regex pre-compiled and its size
// bounds pre-parsed, so the per-file hot path does no repeated work.
type compiledFilter struct {
	include bool
	typ     string
	value   string
	re      *regexp.Regexp
	sizeMin int64
	sizeMax int64
	hasMin  bool
	hasMax  bool
}

// compiledFilterSet is a named slice type so methods can be attached to it.
type compiledFilterSet []compiledFilter

// compile merges the include and exclude lists into a flat, pre-compiled
// slice. The include/exclude flag comes from the array an item lives in.
func (f FilterSet) compile() (compiledFilterSet, error) {
	all := make(compiledFilterSet, 0, len(f.Include)+len(f.Exclude))
	for _, it := range f.Include {
		c, err := compileFilter(it, true)
		if err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	for _, it := range f.Exclude {
		c, err := compileFilter(it, false)
		if err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	return all, nil
}

func compileFilter(it FilterItem, include bool) (compiledFilter, error) {
	c := compiledFilter{include: include, typ: it.Type, value: it.Value, sizeMax: -1}
	switch it.Type {
	case FilterTypeExtension, FilterTypeFilename:
		if it.Value == "" {
			return c, fmt.Errorf("filter value is required for type %q", it.Type)
		}
	case FilterTypeRegex:
		re, err := regexp.Compile(it.Value)
		if err != nil {
			return c, fmt.Errorf("invalid regex %q: %w", it.Value, err)
		}
		c.re = re
	case FilterTypeSize:
		if it.Value == "" {
			return c, fmt.Errorf("filter value is required for type \"size\"")
		}
		n, err := parseSize(it.Value)
		if err != nil {
			return c, err
		}
		switch it.SizeOp {
		case "":
			// exact match (e.g. "0B" selects empty files)
			c.sizeMin, c.sizeMax = n, n
			c.hasMin, c.hasMax = true, true
		case "min":
			c.sizeMin, c.hasMin = n, true
		case "max":
			c.sizeMax, c.hasMax = n, true
		case "between":
			c.sizeMin, c.hasMin = n, true
			if it.SizeMax != "" {
				mx, err := parseSize(it.SizeMax)
				if err != nil {
					return c, err
				}
				c.sizeMax, c.hasMax = mx, true
			}
		default:
			return c, fmt.Errorf("unknown size_op %q", it.SizeOp)
		}
	default:
		return c, fmt.Errorf("unknown filter type %q", it.Type)
	}
	return c, nil
}

// shouldInclude applies the include/exclude semantics (§12.5):
//  1. if the include list is non-empty, a file must match at least one item;
//  2. a file matching any exclude item is always dropped (exclude wins).
func (c compiledFilterSet) shouldInclude(relPath, name string, size int64) (bool, error) {
	hasInclude := false
	includeMatch := false
	for _, f := range c {
		if !f.include {
			continue
		}
		hasInclude = true
		m, err := f.match(relPath, name, size)
		if err != nil {
			return false, err
		}
		if m {
			includeMatch = true
		}
	}
	if hasInclude && !includeMatch {
		return false, nil
	}
	for _, f := range c {
		if f.include {
			continue
		}
		m, err := f.match(relPath, name, size)
		if err != nil {
			return false, err
		}
		if m {
			return false, nil
		}
	}
	return true, nil
}

func (c compiledFilter) match(relPath, name string, size int64) (bool, error) {
	switch c.typ {
	case FilterTypeExtension:
		return matchExtension(c.value, name), nil
	case FilterTypeFilename:
		return matchGlob(c.value, name)
	case FilterTypeRegex:
		return c.re.MatchString(relPath), nil
	case FilterTypeSize:
		if c.hasMin && size < c.sizeMin {
			return false, nil
		}
		if c.hasMax && size > c.sizeMax {
			return false, nil
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown filter type %q", c.typ)
	}
}

// ext returns the lower-case extension of a file name without the dot.
func ext(name string) string {
	return strings.ToLower(strings.TrimPrefix(stdpath.Ext(name), "."))
}

// matchExtension matches a comma-separated list of extensions (lower-case,
// no dot) against the file's extension.
func matchExtension(value, name string) bool {
	e := ext(name)
	if e == "" {
		return false
	}
	for _, v := range strings.Split(value, ",") {
		if strings.ToLower(strings.TrimSpace(v)) == e {
			return true
		}
	}
	return false
}

// matchGlob matches a path.Match pattern against the file name. A leading
// "!" negates the pattern (anti-pattern).
func matchGlob(pattern, name string) (bool, error) {
	negate := false
	p := pattern
	if strings.HasPrefix(p, "!") {
		negate = true
		p = p[1:]
	}
	m, err := stdpath.Match(p, name)
	if err != nil {
		return false, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	if negate {
		return !m, nil
	}
	return m, nil
}

// parseSize parses a human-readable size into bytes. Supports B/KB/MB/GB/TB
// (case-insensitive, 1024-based) and a bare integer (bytes). Examples:
// "0B", "512", "10MB", "1.5GB".
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	i := 0
	for i < len(s) && (s[i] == '.' || s[i] == '+' || s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	numStr := s[:i]
	unit := strings.ToUpper(strings.TrimSpace(s[i:]))
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	var mult float64 = 1
	switch unit {
	case "", "B":
		mult = 1
	case "K", "KB", "KIB":
		mult = 1 << 10
	case "M", "MB", "MIB":
		mult = 1 << 20
	case "G", "GB", "GIB":
		mult = 1 << 30
	case "T", "TB", "TIB":
		mult = 1 << 40
	default:
		return 0, fmt.Errorf("unknown size unit %q in %q", unit, s)
	}
	return int64(num * mult), nil
}

// Package diff compares two parsed .env files key by key.
package diff

import (
	"github.com/yassataiseer/envy/pkg/envparse"
)

// DiffStatus enumerates the comparative state of a key across two files.
type DiffStatus string

const (
	StatusIdentical    DiffStatus = "IDENTICAL"
	StatusValueDiff    DiffStatus = "VALUE_MISMATCH"
	StatusMissingLeft  DiffStatus = "MISSING_IN_LEFT"
	StatusMissingRight DiffStatus = "MISSING_IN_RIGHT"
	StatusTypeMismatch DiffStatus = "TYPE_MISMATCH"
)

// Marker is a one-character symbol for the status: "~" changed,
// "≠" type mismatch, "+" missing in left, "-" missing in right, and a space
// for identical.
func (s DiffStatus) Marker() string {
	switch s {
	case StatusValueDiff:
		return "~"
	case StatusTypeMismatch:
		return "≠"
	case StatusMissingLeft:
		return "+"
	case StatusMissingRight:
		return "-"
	default:
		return " "
	}
}

// DiffResult holds the analytical output of comparing two key-value entries.
// LeftEntry and RightEntry point into the compared files' Entries and become
// stale if those files are edited.
type DiffResult struct {
	Key        string             `json:"key"`
	Status     DiffStatus         `json:"status"`
	LeftEntry  *envparse.EnvEntry `json:"left,omitempty"`
	RightEntry *envparse.EnvEntry `json:"right,omitempty"`
	LeftKind   ValueKind          `json:"left_kind,omitempty"`
	RightKind  ValueKind          `json:"right_kind,omitempty"`
}

// FileDiffSet summarizes the aggregate delta between two files.
type FileDiffSet struct {
	SourcePath string       `json:"source_path"`
	TargetPath string       `json:"target_path"`
	Results    []DiffResult `json:"results"`
	HasDrift   bool         `json:"has_drift"`
}

// Options controls how values are compared.
type Options struct {
	// IgnoreValues compares key presence only, which suits comparing a
	// real .env against a template whose values are placeholders.
	IgnoreValues bool
}

// Compare diffs left (the source) against right (the target). Each key
// appears once, using its effective (last) definition in each file.
//
// Results follow the left file's key order. Keys that exist only on the
// right are placed directly after the nearest preceding key they follow in
// the right file, so related keys stay together in side-by-side views.
func Compare(left, right *envparse.EnvFile, opts Options) *FileDiffSet {
	set := &FileDiffSet{SourcePath: left.Path, TargetPath: right.Path}

	leftKeys := left.Keys()
	rightOnlyAfter := make(map[string][]string)
	anchor := ""
	for _, k := range right.Keys() {
		if _, inLeft := left.Get(k); inLeft {
			anchor = k
			continue
		}
		rightOnlyAfter[anchor] = append(rightOnlyAfter[anchor], k)
	}

	add := func(k string) {
		r := compareKey(k, left, right, opts)
		if r.Status != StatusIdentical {
			set.HasDrift = true
		}
		set.Results = append(set.Results, r)
	}
	set.Results = make([]DiffResult, 0, len(leftKeys)+len(right.Index))
	for _, k := range rightOnlyAfter[""] {
		add(k)
	}
	for _, k := range leftKeys {
		add(k)
		for _, rk := range rightOnlyAfter[k] {
			add(rk)
		}
	}
	return set
}

func compareKey(key string, left, right *envparse.EnvFile, opts Options) DiffResult {
	l, inLeft := left.Get(key)
	r, inRight := right.Get(key)
	res := DiffResult{Key: key}
	if inLeft {
		res.LeftEntry = l
		res.LeftKind = InferKind(l.Value)
	}
	if inRight {
		res.RightEntry = r
		res.RightKind = InferKind(r.Value)
	}
	switch {
	case !inLeft:
		res.Status = StatusMissingLeft
	case !inRight:
		res.Status = StatusMissingRight
	case opts.IgnoreValues || l.Value == r.Value:
		res.Status = StatusIdentical
	case kindsConflict(res.LeftKind, res.RightKind):
		res.Status = StatusTypeMismatch
	default:
		res.Status = StatusValueDiff
	}
	return res
}

// Summary counts results by status.
type Summary struct {
	Identical    int `json:"identical"`
	ValueDiff    int `json:"value_mismatch"`
	TypeMismatch int `json:"type_mismatch"`
	MissingLeft  int `json:"missing_in_left"`
	MissingRight int `json:"missing_in_right"`
}

// Differences returns the number of keys that are not identical.
func (s Summary) Differences() int {
	return s.ValueDiff + s.TypeMismatch + s.MissingLeft + s.MissingRight
}

// Summary counts the set's results by status.
func (s *FileDiffSet) Summary() Summary {
	var sum Summary
	for i := range s.Results {
		switch s.Results[i].Status {
		case StatusIdentical:
			sum.Identical++
		case StatusValueDiff:
			sum.ValueDiff++
		case StatusTypeMismatch:
			sum.TypeMismatch++
		case StatusMissingLeft:
			sum.MissingLeft++
		case StatusMissingRight:
			sum.MissingRight++
		}
	}
	return sum
}

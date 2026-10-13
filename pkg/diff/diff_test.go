package diff

import (
	"reflect"
	"testing"

	"github.com/yassataiseer/envy/pkg/envparse"
)

func parse(t *testing.T, path, s string) *envparse.EnvFile {
	t.Helper()
	f, err := envparse.ParseBytes([]byte(s), path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type keyStatus struct {
	Key    string
	Status DiffStatus
}

func statuses(set *FileDiffSet) []keyStatus {
	out := make([]keyStatus, len(set.Results))
	for i, r := range set.Results {
		out[i] = keyStatus{r.Key, r.Status}
	}
	return out
}

func TestCompareStatuses(t *testing.T) {
	left := parse(t, ".env", `PORT=8080
DATABASE_URL=postgres://localhost/app
DEBUG=true
NAME="same"
TIMEOUT=1.5
SECRET=abc
`)
	right := parse(t, ".env.example", `PORT=3000
DATABASE_URL=
DEBUG=maybe
NAME=same
TIMEOUT=2
LOG_LEVEL=debug
`)
	set := Compare(left, right, Options{})
	want := []keyStatus{
		{"PORT", StatusValueDiff},
		{"DATABASE_URL", StatusValueDiff},
		{"DEBUG", StatusTypeMismatch},
		{"NAME", StatusIdentical},
		{"TIMEOUT", StatusValueDiff},
		{"LOG_LEVEL", StatusMissingLeft},
		{"SECRET", StatusMissingRight},
	}
	if got := statuses(set); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	if !set.HasDrift || set.SourcePath != ".env" || set.TargetPath != ".env.example" {
		t.Errorf("set metadata = %+v", set)
	}
	sum := set.Summary()
	if sum != (Summary{Identical: 1, ValueDiff: 3, TypeMismatch: 1, MissingLeft: 1, MissingRight: 1}) || sum.Differences() != 6 {
		t.Errorf("summary = %+v", sum)
	}
}

func TestCompareIdenticalFiles(t *testing.T) {
	a := parse(t, "a", "# c\nA=1\nB='x'\n")
	b := parse(t, "b", "B=x\n\nA=\"1\"")
	set := Compare(a, b, Options{})
	if set.HasDrift {
		t.Errorf("expected no drift, got %v", statuses(set))
	}
}

func TestCompareIgnoreValues(t *testing.T) {
	a := parse(t, "a", "A=1\nB=2\n")
	b := parse(t, "b", "A=\nB=hello\nC=3\n")
	set := Compare(a, b, Options{IgnoreValues: true})
	want := []keyStatus{{"A", StatusIdentical}, {"B", StatusIdentical}, {"C", StatusMissingLeft}}
	if got := statuses(set); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCompareOrdering(t *testing.T) {
	left := parse(t, "l", "A=1\nC=1\nE=1\n")
	right := parse(t, "r", "X=1\nA=1\nB=1\nC=1\nD=1\nD2=1\nZ=1\n")
	var got []string
	for _, r := range Compare(left, right, Options{}).Results {
		got = append(got, r.Key)
	}
	want := []string{"X", "A", "B", "C", "D", "D2", "Z", "E"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestCompareUsesLastDuplicate(t *testing.T) {
	set := Compare(parse(t, "l", "A=1\nA=2"), parse(t, "r", "A=2"), Options{})
	if len(set.Results) != 1 || set.Results[0].Status != StatusIdentical {
		t.Errorf("results = %+v", set.Results)
	}
	if set.Results[0].LeftEntry.LineNumber != 2 {
		t.Errorf("left entry should be the last definition")
	}
}

func TestCompareEmptyFiles(t *testing.T) {
	set := Compare(parse(t, "l", ""), parse(t, "r", "# only comments\n"), Options{})
	if set.HasDrift || len(set.Results) != 0 {
		t.Errorf("set = %+v", set)
	}
}

func TestInferKind(t *testing.T) {
	tests := map[string]ValueKind{
		"":                         KindEmpty,
		"   ":                      KindEmpty,
		"true":                     KindBool,
		"OFF":                      KindBool,
		"0":                        KindInt,
		"-42":                      KindInt,
		"3.14":                     KindFloat,
		"1e9":                      KindFloat,
		"1.2.3":                    KindString,
		"inf":                      KindString,
		"NaN":                      KindString,
		"0x1F":                     KindString,
		"https://example.com/x":    KindURL,
		"postgres://u:p@h:5432/db": KindURL,
		"redis://":                 KindString,
		"hello":                    KindString,
		"sk_test_abc":              KindString,
	}
	for in, want := range tests {
		if got := InferKind(in); got != want {
			t.Errorf("InferKind(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestKindsConflict(t *testing.T) {
	tests := []struct {
		a, b ValueKind
		want bool
	}{
		{KindInt, KindFloat, false},
		{KindInt, KindString, true},
		{KindEmpty, KindString, false},
		{KindURL, KindURL, false},
		{KindURL, KindString, true},
		{KindBool, KindInt, true},
	}
	for _, tt := range tests {
		if got := kindsConflict(tt.a, tt.b); got != tt.want {
			t.Errorf("kindsConflict(%s, %s) = %v", tt.a, tt.b, got)
		}
	}
}

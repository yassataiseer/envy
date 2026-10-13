package runner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yassataiseer/envy/pkg/envparse"
)

func files(t *testing.T, contents ...string) []*envparse.EnvFile {
	t.Helper()
	out := make([]*envparse.EnvFile, len(contents))
	for i, c := range contents {
		f, err := envparse.ParseBytes([]byte(c), "f.env")
		if err != nil {
			t.Fatal(err)
		}
		out[i] = f
	}
	return out
}

func TestBuildEnv(t *testing.T) {
	parent := []string{"PATH=/usr/bin", "HOME=/home/y", "PORT=1", "SHELL_ONLY=s"}
	tests := []struct {
		name  string
		files []string
		opts  Options
		want  []string
	}{
		{
			name:  "file overrides parent and appends new keys",
			files: []string{"PORT=8080\nNEW=x\n"},
			want:  []string{"PATH=/usr/bin", "HOME=/home/y", "PORT=8080", "SHELL_ONLY=s", "NEW=x"},
		},
		{
			name:  "no override keeps parent values",
			files: []string{"PORT=8080\nNEW=x\n"},
			opts:  Options{NoOverride: true},
			want:  []string{"PATH=/usr/bin", "HOME=/home/y", "PORT=1", "SHELL_ONLY=s", "NEW=x"},
		},
		{
			name:  "isolated drops parent",
			files: []string{"PORT=8080\nexport HOME\nexport MISSING\n"},
			opts:  Options{Isolated: true},
			want:  []string{"PORT=8080", "HOME=/home/y"},
		},
		{
			name:  "later files override earlier ones",
			files: []string{"A=1\nB=1\n", "B=2\nC=2\n"},
			opts:  Options{Isolated: true},
			want:  []string{"A=1", "B=2", "C=2"},
		},
		{
			name:  "expansion uses earlier files then parent",
			files: []string{"BASE=http://h\n", "URL=${BASE}/api\nP=$PATH:/opt\nD=${NOPE:-def}\n"},
			opts:  Options{Isolated: true},
			want:  []string{"BASE=http://h", "URL=http://h/api", "P=/usr/bin:/opt", "D=def"},
		},
		{
			name:  "bare export without isolation keeps parent value",
			files: []string{"export HOME\n"},
			want:  parent,
		},
		{
			name:  "duplicate keys: last wins",
			files: []string{"A=1\nA=2\n"},
			opts:  Options{Isolated: true},
			want:  []string{"A=2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildEnv(files(t, tt.files...), parent, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBuildEnvErrors(t *testing.T) {
	if _, err := BuildEnv(files(t, "A=\"x\\\"\"\nB=${A"), nil, Options{}); err == nil {
		t.Error("expansion error not reported")
	}
	if _, err := BuildEnv(files(t, "A=\"a\x00b\""), nil, Options{}); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Errorf("NUL byte error = %v", err)
	}
}

func TestBuildEnvIgnoresMalformedParent(t *testing.T) {
	got, err := BuildEnv(nil, []string{"NOEQUALS", "=hidden", "A=1=2"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"A=1=2"}) {
		t.Errorf("got %q", got)
	}
}

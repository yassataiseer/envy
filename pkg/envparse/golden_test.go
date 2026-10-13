package envparse

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden.json from current parser output")

func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob("testdata/*.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.env files found")
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".env")
		t.Run(name, func(t *testing.T) {
			f, err := ParseFile(in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(f, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			goldenPath := strings.TrimSuffix(in, ".env") + ".golden.json"
			if *update {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("%v (run `go test ./pkg/envparse -run TestGolden -update` to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s; rerun with -update if the change is intended\n got:\n%s", goldenPath, got)
			}
		})
	}
}

func TestGoldenRSAKey(t *testing.T) {
	f, err := ParseFile("testdata/rsa_key.env")
	if err != nil {
		t.Fatal(err)
	}
	priv, ok := f.Get("JWT_PRIVATE_KEY")
	if !ok {
		t.Fatal("JWT_PRIVATE_KEY missing")
	}
	if !priv.IsExported || priv.LineNumber != 3 || priv.EndLine != 11 {
		t.Errorf("JWT_PRIVATE_KEY exported=%v lines=%d-%d, want exported lines 3-11", priv.IsExported, priv.LineNumber, priv.EndLine)
	}
	if !strings.HasPrefix(priv.Value, "-----BEGIN RSA PRIVATE KEY-----\nMIIB") ||
		!strings.HasSuffix(priv.Value, "\n-----END RSA PRIVATE KEY-----") ||
		strings.Count(priv.Value, "\n") != 8 {
		t.Errorf("JWT_PRIVATE_KEY value not preserved:\n%s", priv.Value)
	}
	esc, _ := f.Get("ESCAPED_KEY")
	if esc.Value != "-----BEGIN KEY-----\nabc123\n-----END KEY-----\n" {
		t.Errorf("ESCAPED_KEY = %q", esc.Value)
	}
	if ttl, _ := f.Get("TTL"); ttl.LineNumber != 17 {
		t.Errorf("TTL line = %d, want 17", ttl.LineNumber)
	}
}

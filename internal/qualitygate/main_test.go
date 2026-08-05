package main

import (
	"crypto/sha256"
	"maps"
	"slices"
	"testing"
)

func TestHasModuleDirective(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "exact", content: "module " + modulePath + "\n", want: true},
		{name: "space", content: " module " + modulePath + " \n", want: true},
		{name: "suffix", content: "module " + modulePath + "/old\n"},
		{name: "comment", content: "// module " + modulePath + "\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := hasModuleDirective(test.content, modulePath); got != test.want {
				t.Fatalf("hasModuleDirective() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseTotalCoverage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		report  string
		want    float64
		wantErr bool
	}{
		{name: "valid", report: "file.go:1:\tf\t100.0%\ntotal:\t(statements)\t86.1%\n", want: 86.1},
		{name: "empty", wantErr: true},
		{name: "missing percent", report: "total: statements 86.1", wantErr: true},
		{name: "invalid percent", report: "total: statements nope%", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTotalCoverage(test.report)
			if test.wantErr {
				if err == nil {
					t.Fatal("parseTotalCoverage() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTotalCoverage() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseTotalCoverage() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBatchesPreservesOrder(t *testing.T) {
	t.Parallel()
	values := []string{"a", "b", "c", "d", "e"}
	got := batches(values, 2)
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("batches() = %#v, want %#v", got, want)
	}
}

func TestEqualDigests(t *testing.T) {
	t.Parallel()
	base := map[string][sha256.Size]byte{
		"a": sha256.Sum256([]byte("one")),
		"b": sha256.Sum256([]byte("two")),
	}
	tests := []struct {
		name  string
		right map[string][sha256.Size]byte
		want  bool
	}{
		{name: "same", right: maps.Clone(base), want: true},
		{name: "missing", right: map[string][sha256.Size]byte{"a": base["a"]}},
		{name: "changed", right: map[string][sha256.Size]byte{
			"a": base["a"],
			"b": sha256.Sum256([]byte("changed")),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := equalDigests(base, test.right); got != test.want {
				t.Fatalf("equalDigests() = %v, want %v", got, test.want)
			}
		})
	}
}

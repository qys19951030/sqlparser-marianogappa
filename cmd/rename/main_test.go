package main

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

type runCase struct {
	name     string
	args     []string
	stdin    string
	wantCode int
	wantOut  string
	wantErr  string
}

func TestRun(t *testing.T) {
	cases := []runCase{
		{
			name:     "table rename via stdin",
			args:     []string{"-t", "t1=t2"},
			stdin:    "SELECT a FROM 't1'",
			wantCode: 0,
			wantOut:  "SELECT a FROM 't2'",
		},
		{
			name:     "parse failure writes stderr and nothing to stdout",
			args:     []string{"-t", "t1=t2"},
			stdin:    "NOT A QUERY",
			wantCode: 1,
			wantErr:  "invalid query type",
		},
		{
			name:     "malformed mapping is rejected",
			args:     []string{"-t", "t1"},
			stdin:    "SELECT a FROM 't1'",
			wantCode: 2,
			wantErr:  "invalid mapping",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run(tc.args, strings.NewReader(tc.stdin), &out, &errOut)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d", code, tc.wantCode)
			}
			if out.String() != tc.wantOut {
				t.Fatalf("stdout = %q, want %q", out.String(), tc.wantOut)
			}
			if tc.wantErr != "" && !strings.Contains(errOut.String(), tc.wantErr) {
				t.Fatalf("stderr = %q, want substring %q", errOut.String(), tc.wantErr)
			}
		})
	}
}

func TestRunFileInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.sql")
	if err := ioutil.WriteFile(path, []byte("DELETE FROM 't1' WHERE a = '2'"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := run([]string{"-t", "t1=t2", "-f", path}, bytes.NewReader(nil), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if out.String() != "DELETE FROM 't2' WHERE a = '2'" {
		t.Fatalf("stdout = %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestRunMissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"-f", filepath.Join(t.TempDir(), "missing.sql")}, bytes.NewReader(nil), &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "no such file") {
		t.Fatalf("stderr = %q, want a file error", errOut.String())
	}
}

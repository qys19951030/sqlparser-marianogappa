package main

import "testing"

type runCase struct {
	name    string
	args    []string
	stdin   string
	wantCode int
	wantOut string
	wantErr string
}

func TestRun(t *testing.T) {
	cases := []runCase{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Assertions are added alongside each implementation stage.
		})
	}
}

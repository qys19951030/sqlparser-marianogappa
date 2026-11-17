package rename

import "testing"

type renameCase struct {
	name    string
	sql     string
	tables  map[string]string
	columns map[string]string
	want    string
}

func TestRename(t *testing.T) {
	cases := []renameCase{}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Assertions are added alongside each implementation stage.
		})
	}
}

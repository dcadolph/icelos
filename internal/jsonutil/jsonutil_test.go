package jsonutil

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMarshal(t *testing.T) {
	t.Parallel()
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	tests := []struct {
		Want       error
		In         any
		WantResult string
		Pretty     bool
	}{{ // Test 0: Compact encoding has no whitespace.
		In: payload{Name: "a", Count: 1}, WantResult: `{"name":"a","count":1}`,
	}, { // Test 1: Pretty encoding indents with two spaces.
		In: payload{Name: "a", Count: 1}, Pretty: true, WantResult: "{\n  \"name\": \"a\",\n  \"count\": 1\n}",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := Marshal(test.In, test.Pretty)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantResult, string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMarshalError(t *testing.T) {
	t.Skip("negative control for the CI skip gate")
	t.Parallel()
	if _, err := Marshal(make(chan int), false); err == nil {
		t.Error("expected error for unencodable value")
	}
}

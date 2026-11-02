package progress

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        string
		WantCount []Update
	}{{ // Test 0: One continuing and one final report.
		In: "fps=30.0\nout_time_us=2000000\nspeed=1.5x\nprogress=continue\n" +
			"fps=31.0\nout_time_us=4000000\nspeed=2.0x\nprogress=end\n",
		WantCount: []Update{
			{Elapsed: 2 * time.Second, Speed: 1.5, FPS: 30},
			{Elapsed: 4 * time.Second, Speed: 2, FPS: 31, Done: true},
		},
	}, { // Test 1: Empty input yields nothing.
		In: "",
	}, { // Test 2: Garbage lines and N/A speed are ignored.
		In:        "junk\nspeed=N/A\nout_time_us=1000000\nprogress=continue\n",
		WantCount: []Update{{Elapsed: time.Second}},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var got []Update
			if err := Read(strings.NewReader(test.In), func(u Update) { got = append(got, u) }); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(test.WantCount, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

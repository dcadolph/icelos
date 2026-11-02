package engine

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseEncoders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In      string
		WantSet EncoderSet
	}{{ // Test 0: Legend lines before the divider are skipped.
		In: "Encoders:\n V..... = Video\n A..... = Audio\n ------\n" +
			" V....D libx264              libx264 H.264 (codec h264)\n" +
			" V....D h264_videotoolbox    VideoToolbox H.264 Encoder (codec h264)\n" +
			" A..... aac_at               aac (AudioToolbox) (codec aac)\n",
		WantSet: EncoderSet{"libx264": true, "h264_videotoolbox": true, "aac_at": true},
	}, { // Test 1: No divider means no encoders.
		In: " V....D libx264 x\n",
	}, { // Test 2: Short or malformed lines are ignored.
		In: " ------\nx\n V....D\n VV libx265 x\n A....D aac AAC\n", WantSet: EncoderSet{"aac": true},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := ParseEncoders([]byte(test.In))
			if diff := cmp.Diff(test.WantSet, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

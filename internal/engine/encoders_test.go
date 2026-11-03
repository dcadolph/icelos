package engine

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/icelos/internal/format"
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

func TestUsable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantNames []string
		In        EncoderSet
	}{{ // Test 0: Only catalog encoders with a builder count.
		In:        EncoderSet{"libx264": true, "png": true, "aac_at": true, "libtheora": true},
		WantNames: []string{"aac_at", "libtheora", "libx264"},
	}, { // Test 1: Empty set.
		In: EncoderSet{},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantNames, Usable(test.In), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Codec      string
		In         EncoderSet
		WantResult bool
	}{{ // Test 0: Software encoder present.
		Codec: "hevc", In: EncoderSet{"libx265": true}, WantResult: true,
	}, { // Test 1: Hardware encoder alone is enough.
		Codec: "hevc", In: EncoderSet{"hevc_videotoolbox": true}, WantResult: true,
	}, { // Test 2: No encoder for the codec.
		Codec: "hevc", In: EncoderSet{"libx264": true},
	}, { // Test 3: Theora through the bundled ffmpeg's libtheora.
		Codec: "theora", In: EncoderSet{"libtheora": true}, WantResult: true,
	}, { // Test 4: An encoder outside the catalog makes nothing available.
		Codec: "theora", In: EncoderSet{"png": true},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			codec, err := format.CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantResult, Available(codec, test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

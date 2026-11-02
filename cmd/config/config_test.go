package config

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestLoadPriority checks flag over environment over default. It cannot run in parallel
// because flags and the environment are process-wide.
func TestLoadPriority(t *testing.T) { //nolint:paralleltest // Mutates process-wide state.
	tests := []struct {
		Flag        string
		Env         string
		WantPreset  string
		WantWorkers int
		WantHW      bool
	}{{ // Test 0: Defaults.
		WantPreset: DefaultPreset, WantWorkers: DefaultWorkers, WantHW: DefaultHardware,
	}, { // Test 1: Environment beats default.
		Env: "web", WantPreset: "web", WantWorkers: DefaultWorkers, WantHW: DefaultHardware,
	}, { // Test 2: Flag beats environment.
		Flag: "tiny", Env: "web", WantPreset: "tiny", WantWorkers: DefaultWorkers, WantHW: DefaultHardware,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)
			if test.Env != "" {
				t.Setenv("ICELOS_PRESET", test.Env)
			}
			if test.Flag != "" {
				if err := FlagPreset.Value.Set(test.Flag); err != nil {
					t.Fatal(err)
				}
				FlagPreset.Changed = true
			}
			got := []any{Preset(), Workers(), Hardware()}
			if diff := cmp.Diff([]any{test.WantPreset, test.WantWorkers, test.WantHW}, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBadEnvFallsBack(t *testing.T) {
	t.Setenv("ICELOS_WORKERS", "many")
	t.Setenv("ICELOS_HARDWARE", "maybe")
	Reset()
	if Workers() != DefaultWorkers || Hardware() != DefaultHardware {
		t.Errorf("bad env not ignored: workers=%d hardware=%v", Workers(), Hardware())
	}
}

func TestFlagValueSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Type    string
		In      string
		WantErr bool
	}{{ // Test 0: Valid bool.
		Type: "bool", In: "false",
	}, { // Test 1: Invalid bool.
		Type: "bool", In: "nah", WantErr: true,
	}, { // Test 2: Valid int.
		Type: "int", In: "4",
	}, { // Test 3: Invalid int.
		Type: "int", In: "four", WantErr: true,
	}, { // Test 4: Any string.
		Type: "string", In: "anything",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			v := &FlagValue{ValType: test.Type}
			if err := v.Set(test.In); (err != nil) != test.WantErr {
				t.Fatalf("Set(%q) error = %v, want error %v", test.In, err, test.WantErr)
			}
			if !test.WantErr && (v.String() != test.In || v.Type() != test.Type) {
				t.Errorf("got %q %q", v.String(), v.Type())
			}
		})
	}
}

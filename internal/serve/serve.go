// Package serve speaks the JSON-lines protocol the app uses to drive the converter. Each
// request is one JSON object per line on the input, and every reply or event is one JSON
// object per line on the output. Closing the input cancels running jobs and ends the session,
// so the converter never outlives the app that started it.
package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
	"github.com/dcadolph/icelos/internal/queue"
)

// maxLine bounds one request line.
const maxLine = 1 << 20

// Request operations.
const (
	// OpCapabilities asks for the version and the encoders ffmpeg offers.
	OpCapabilities = "capabilities"
	// OpPresets asks for the built-in presets.
	OpPresets = "presets"
	// OpProbe asks for the metadata of Request.Path.
	OpProbe = "probe"
	// OpAdd queues a conversion.
	OpAdd = "add"
	// OpCancel cancels Request.Job.
	OpCancel = "cancel"
	// OpList asks for every job.
	OpList = "list"
	// OpClear drops finished jobs.
	OpClear = "clear"
)

// Message types.
const (
	// TypeReady is sent once when the session starts.
	TypeReady = "ready"
	// TypeError reports a failed request.
	TypeError = "error"
	// TypeOK acknowledges a request with no other payload.
	TypeOK = "ok"
	// TypeJob carries a job status change.
	TypeJob = "job"
)

// Request is one line of input.
type Request struct {
	// ID is an optional client token echoed in the reply.
	ID string `json:"id,omitempty"`
	// Op is the operation name.
	Op string `json:"op"`
	// Path is the file to probe.
	Path string `json:"path,omitempty"`
	// Input is the file to convert.
	Input string `json:"input,omitempty"`
	// Preset is the preset ID for add.
	Preset string `json:"preset,omitempty"`
	// Size is the target size for the target-size preset, e.g. "25MB".
	Size string `json:"size,omitempty"`
	// Hardware prefers VideoToolbox. Absent means true.
	Hardware *bool `json:"hardware,omitempty"`
	// OutDir is the folder for the output. Empty means next to the input.
	OutDir string `json:"outDir,omitempty"`
	// Output is an exact output path.
	Output string `json:"output,omitempty"`
	// Overwrite replaces an existing output.
	Overwrite bool `json:"overwrite,omitempty"`
	// Job is the job ID to cancel.
	Job string `json:"job,omitempty"`
}

// Message is one line of output.
type Message struct {
	// Type is the message kind, either a Type constant or the request op it answers.
	Type string `json:"type"`
	// Reply is the ID of the request this answers.
	Reply string `json:"reply,omitempty"`
	// Error is the failure message for error replies.
	Error string `json:"error,omitempty"`
	// Version is the converter version.
	Version string `json:"version,omitempty"`
	// Encoders lists the ffmpeg encoders icelos can use.
	Encoders []string `json:"encoders,omitempty"`
	// Presets lists the built-in presets.
	Presets []preset.Preset `json:"presets,omitempty"`
	// Info is the probe result.
	Info *probe.Info `json:"info,omitempty"`
	// Status is one job snapshot.
	Status *queue.Status `json:"status,omitempty"`
	// Jobs lists every job.
	Jobs []queue.Status `json:"jobs,omitempty"`
}

// EncodersFunc reports the encoders ffmpeg offers.
type EncodersFunc func(ctx context.Context) (engine.EncoderSet, error)

// Config holds what a Server needs.
type Config struct {
	// Encoder runs jobs.
	Encoder engine.Encoder
	// Prober reads metadata for probe requests.
	Prober engine.Prober
	// Encoders lists the available encoders.
	Encoders EncodersFunc
	// Version is reported in the ready and capabilities messages.
	Version string
	// Workers is how many jobs run at once. Zero means one.
	Workers int
}

// Server runs one protocol session.
type Server struct {
	// cfg is the configuration.
	cfg Config
	// q holds the jobs.
	q *queue.Queue
	// mu serializes writes to out.
	mu sync.Mutex
	// out encodes messages to the output.
	out *json.Encoder
	// writeErr is the first write failure.
	writeErr error
}

// New returns a Server that writes messages to w.
func New(w io.Writer, cfg Config) *Server {
	if w == nil {
		panic("serve.New: writer required")
	}
	if cfg.Encoder == nil || cfg.Prober == nil || cfg.Encoders == nil {
		panic("serve.New: Encoder, Prober, and Encoders required")
	}
	s := &Server{cfg: cfg, out: json.NewEncoder(w)}
	s.q = queue.WithWorkers(cfg.Encoder, max(1, cfg.Workers), func(st queue.Status) {
		s.send(Message{Type: TypeJob, Status: &st})
	})
	return s
}

// Run reads requests from r until it closes or ctx ends, then cancels running jobs and waits
// for them to stop.
func (s *Server) Run(ctx context.Context, r io.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.q.Serve(ctx)
	}()
	s.send(Message{Type: TypeReady, Version: s.cfg.Version})

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.fail("", fmt.Errorf("%w: %w", ErrRequest, err))
			continue
		}
		s.handle(ctx, req, &wg)
	}
	cancel()
	wg.Wait()
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}

// handle answers one request. Slow requests run on their own goroutine tracked by wg.
func (s *Server) handle(ctx context.Context, req Request, wg *sync.WaitGroup) {
	switch req.Op {
	case OpCapabilities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			set, err := s.cfg.Encoders(ctx)
			if err != nil {
				s.fail(req.ID, err)
				return
			}
			s.send(Message{Type: req.Op, Reply: req.ID, Version: s.cfg.Version, Encoders: usable(set)})
		}()
	case OpPresets:
		s.send(Message{Type: req.Op, Reply: req.ID, Presets: preset.All()})
	case OpProbe:
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := s.cfg.Prober.Probe(ctx, req.Path)
			if err != nil {
				s.fail(req.ID, err)
				return
			}
			s.send(Message{Type: req.Op, Reply: req.ID, Info: &info})
		}()
	case OpAdd:
		job, err := jobFor(req)
		if err != nil {
			s.fail(req.ID, err)
			return
		}
		st, err := s.q.Add(job, req.OutDir)
		if err != nil {
			s.fail(req.ID, err)
			return
		}
		s.send(Message{Type: req.Op, Reply: req.ID, Status: &st})
	case OpCancel:
		if err := s.q.Cancel(req.Job); err != nil {
			s.fail(req.ID, err)
			return
		}
		s.send(Message{Type: TypeOK, Reply: req.ID})
	case OpList:
		s.send(Message{Type: req.Op, Reply: req.ID, Jobs: s.q.List()})
	case OpClear:
		s.q.Clear()
		s.send(Message{Type: TypeOK, Reply: req.ID})
	default:
		s.fail(req.ID, fmt.Errorf("%w: %q", ErrUnknownOp, req.Op))
	}
}

// jobFor builds the engine job an add request describes.
func jobFor(req Request) (engine.Job, error) {
	if req.Input == "" {
		return engine.Job{}, fmt.Errorf("%w: input required", ErrRequest)
	}
	p, err := preset.Lookup(req.Preset)
	if err != nil {
		return engine.Job{}, err
	}
	if req.Size != "" {
		n, err := preset.ParseSize(req.Size)
		if err != nil {
			return engine.Job{}, err
		}
		p = p.WithTargetSize(n)
	}
	if p.ID == preset.TargetSizeID && p.TargetBytes == 0 {
		return engine.Job{}, fmt.Errorf("%w: %s needs a size", ErrRequest, p.ID)
	}
	hw := req.Hardware == nil || *req.Hardware
	return engine.Job{Input: req.Input, Output: req.Output, Preset: p, Hardware: hw,
		Overwrite: req.Overwrite}, nil
}

// usable returns the sorted encoder names from set that icelos knows how to drive.
func usable(set engine.EncoderSet) []string {
	known := []string{"libx264", "libx265", "h264_videotoolbox", "hevc_videotoolbox", "aac",
		"aac_at", "libmp3lame"}
	var out []string
	for _, name := range known {
		if set.Has(name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// fail sends an error reply.
func (s *Server) fail(id string, err error) {
	s.send(Message{Type: TypeError, Reply: id, Error: strings.TrimSpace(err.Error())})
}

// send writes one message, keeping the first write error for Run to return.
func (s *Server) send(m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return
	}
	if err := s.out.Encode(m); err != nil {
		s.writeErr = fmt.Errorf("%w: %w", ErrWrite, err)
	}
}

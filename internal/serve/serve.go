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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
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
	// OpCatalog asks for the containers, the codecs this ffmpeg can encode, and the option lists.
	OpCatalog = "catalog"
	// OpPresets asks for the built-in and custom presets and the recently used IDs.
	OpPresets = "presets"
	// OpPresetSave saves Request.PresetBody as a custom preset and answers like presets.
	OpPresetSave = "preset.save"
	// OpPresetDelete deletes the custom preset Request.Preset and answers like presets.
	OpPresetDelete = "preset.delete"
	// OpProbe asks for the metadata of Request.Path.
	OpProbe = "probe"
	// OpAdd queues a conversion.
	OpAdd = "add"
	// OpPlan answers what an add request with the same fields would run, without queueing it.
	OpPlan = "plan"
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
	// Preset is the preset ID for add and preset.delete.
	Preset string `json:"preset,omitempty"`
	// PresetBody is a whole preset. For add it is used instead of Preset, so a file can carry
	// settings that no saved preset holds. For preset.save it is the preset to save.
	PresetBody *preset.Preset `json:"presetBody,omitempty"`
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
	// Streams chooses the tracks, subtitles, chapters, metadata, and threads for add and plan.
	Streams engine.Streams `json:"streams,omitzero"`
	// SplitChapters makes add queue one job per chapter of the input.
	SplitChapters bool `json:"splitChapters,omitempty"`
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
	// Presets lists the built-in presets followed by the custom ones.
	Presets []preset.Preset `json:"presets,omitempty"`
	// Recent lists the IDs of the presets used most recently, newest first.
	Recent []string `json:"recent,omitempty"`
	// Warning reports a problem that did not stop the request, such as an unreadable custom
	// preset file.
	Warning string `json:"warning,omitempty"`
	// Catalog is the format catalog with codec availability.
	Catalog *Catalog `json:"catalog,omitempty"`
	// Info is the probe result.
	Info *probe.Info `json:"info,omitempty"`
	// Plan is the answer to a plan request.
	Plan *Plan `json:"plan,omitempty"`
	// Status is one job snapshot. For add it is the first job the request became.
	Status *queue.Status `json:"status,omitempty"`
	// Jobs lists every job for list, and for add every job a split request became.
	Jobs []queue.Status `json:"jobs,omitempty"`
}

// Catalog is the format catalog as this ffmpeg build sees it.
type Catalog struct {
	// Containers lists every output container in catalog order.
	Containers []format.Container `json:"containers"`
	// Codecs lists every codec in catalog order with its availability.
	Codecs []CatalogCodec `json:"codecs"`
	// Domains are the value lists the settings dialog offers.
	Domains format.Domains `json:"domains"`
}

// Plan is what an add request would run.
type Plan struct {
	// Copy holds the passthrough verdict for each kind of output stream.
	Copy engine.Copy `json:"copy"`
	// Lossless is true when every output stream can pass through untouched.
	Lossless bool `json:"lossless"`
	// Output is the path the conversion would write.
	Output string `json:"output"`
	// Passes holds the ffmpeg arguments of each pass in run order, without the program name.
	Passes [][]string `json:"passes"`
	// Encoder is the video encoder, empty when video is copied or absent.
	Encoder string `json:"encoder,omitempty"`
	// Hardware is true when Encoder runs on the media engine.
	Hardware bool `json:"hardware"`
	// Filters lists the filters the plan needs that some ffmpeg builds lack.
	Filters []string `json:"filters,omitempty"`
}

// planPassLog is the two-pass statistics path shown in plans. Real encodes use a fresh temporary
// folder instead.
var planPassLog = filepath.Join(os.TempDir(), "icelos-pass") //nolint:gochecknoglobals // Read-only.

// CatalogCodec is one catalog codec and whether it can be encoded here.
type CatalogCodec struct {
	format.Codec
	// Available is true when ffmpeg offers an encoder for the codec that icelos can drive.
	Available bool `json:"available"`
}

// NewCatalog returns the format catalog with each codec marked available or not against set.
func NewCatalog(set engine.EncoderSet) Catalog {
	codecs := format.Codecs()
	out := Catalog{Containers: format.Containers(), Codecs: make([]CatalogCodec, len(codecs)),
		Domains: format.Options()}
	for i, c := range codecs {
		out.Codecs[i] = CatalogCodec{Codec: c, Available: engine.Available(c, set)}
	}
	return out
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
	// Presets holds the custom presets and the recent list.
	Presets *preset.Store
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
	if cfg.Encoder == nil || cfg.Prober == nil || cfg.Encoders == nil || cfg.Presets == nil {
		panic("serve.New: Encoder, Prober, Encoders, and Presets required")
	}
	s := &Server{cfg: cfg, out: json.NewEncoder(w)}
	s.q = queue.WithWorkers(cfg.Encoder, cfg.Prober, max(1, cfg.Workers), func(st queue.Status) {
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
			s.send(Message{Type: req.Op, Reply: req.ID, Version: s.cfg.Version,
				Encoders: engine.Usable(set)})
		}()
	case OpCatalog:
		wg.Add(1)
		go func() {
			defer wg.Done()
			set, err := s.cfg.Encoders(ctx)
			if err != nil {
				s.fail(req.ID, err)
				return
			}
			c := NewCatalog(set)
			s.send(Message{Type: req.Op, Reply: req.ID, Catalog: &c})
		}()
	case OpPresets:
		s.send(s.presets(req))
	case OpPresetSave:
		if req.PresetBody == nil {
			s.fail(req.ID, fmt.Errorf("%w: presetBody required", ErrRequest))
			return
		}
		if err := s.cfg.Presets.Save(*req.PresetBody); err != nil {
			s.fail(req.ID, err)
			return
		}
		s.send(s.presets(req))
	case OpPresetDelete:
		if err := s.cfg.Presets.Delete(req.Preset); err != nil {
			s.fail(req.ID, err)
			return
		}
		s.send(s.presets(req))
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
		job, err := s.jobFor(req)
		if err != nil {
			s.fail(req.ID, err)
			return
		}
		group, err := s.q.Add(ctx, job, req.OutDir)
		if err != nil {
			s.fail(req.ID, err)
			return
		}
		m := Message{Type: req.Op, Reply: req.ID, Status: &group[0]}
		if len(group) > 1 {
			m.Jobs = group
		}
		if req.PresetBody == nil {
			if err := s.cfg.Presets.Use(job.Preset.ID); err != nil {
				m.Warning = err.Error()
			}
		}
		s.send(m)
	case OpPlan:
		job, err := s.jobFor(req)
		if err != nil {
			s.fail(req.ID, err)
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := s.plan(ctx, job, req.OutDir)
			if err != nil {
				s.fail(req.ID, err)
				return
			}
			s.send(Message{Type: req.Op, Reply: req.ID, Plan: &p})
		}()
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

// presets answers req with the built-in and custom presets and the recent list. A custom preset
// file that cannot be read becomes a warning, so the rest still reach the app.
func (s *Server) presets(req Request) Message {
	m := Message{Type: req.Op, Reply: req.ID, Presets: preset.All()}
	custom, listErr := s.cfg.Presets.List()
	m.Presets = append(m.Presets, custom...)
	recent, recentErr := s.cfg.Presets.Recent()
	m.Recent = recent
	if err := errors.Join(listErr, recentErr); err != nil {
		m.Warning = strings.ReplaceAll(err.Error(), "\n", "; ")
	}
	return m
}

// jobFor builds the engine job an add request describes.
func (s *Server) jobFor(req Request) (engine.Job, error) {
	if req.Input == "" {
		return engine.Job{}, fmt.Errorf("%w: input required", ErrRequest)
	}
	p, err := s.presetFor(req)
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
	if err := req.Streams.Validate(); err != nil {
		return engine.Job{}, err
	}
	hw := req.Hardware == nil || *req.Hardware
	return engine.Job{Input: req.Input, Output: req.Output, Preset: p, Hardware: hw,
		Overwrite: req.Overwrite, Streams: req.Streams, SplitChapters: req.SplitChapters}, nil
}

// plan probes job's input and external audio and returns what converting it would run. The
// output path is the one add would pick now, with outDir as the folder when job has none.
func (s *Server) plan(ctx context.Context, job engine.Job, outDir string) (Plan, error) {
	in, err := filepath.Abs(job.Input)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	job.Input = in
	info, err := s.cfg.Prober.Probe(ctx, job.Input)
	if err != nil {
		return Plan{}, err
	}
	extra := make([]probe.Info, 0, len(job.Streams.ExternalAudio))
	for _, path := range job.Streams.ExternalAudio {
		ext, err := s.cfg.Prober.Probe(ctx, path)
		if err != nil {
			return Plan{}, err
		}
		extra = append(extra, ext)
	}
	set, err := s.cfg.Encoders(ctx)
	if err != nil {
		return Plan{}, err
	}
	dest := job.Output
	if dest == "" {
		if dest, err = engine.OutputPath(job.Input, job.Preset.Ext(), outDir, nil); err != nil {
			return Plan{}, err
		}
	}
	c, err := engine.Copyable(job, info, set, extra...)
	if err != nil {
		return Plan{}, err
	}
	built, err := engine.Build(job, info, set, dest, planPassLog, extra...)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Copy: c, Lossless: c.Lossless(), Output: dest, Passes: built.Passes,
		Encoder: built.Encoder, Hardware: built.Hardware, Filters: built.Filters}, nil
}

// presetFor returns the inline preset of an add request after validating it, or else the named
// built-in or custom one.
func (s *Server) presetFor(req Request) (preset.Preset, error) {
	if req.PresetBody == nil {
		return s.cfg.Presets.Lookup(req.Preset)
	}
	if err := req.PresetBody.Validate(); err != nil {
		return preset.Preset{}, err
	}
	return *req.PresetBody, nil
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

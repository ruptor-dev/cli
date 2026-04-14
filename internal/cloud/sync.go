package cloud

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog"
)

// FileResult describes the outcome of one pending-report upload.
type FileResult struct {
	Name  string
	OK    bool
	Error string
}

// SyncResult is the aggregate outcome the cmd layer renders.
type SyncResult struct {
	// Skipped is true when CloudReportingEnabled was false. The cmd
	// layer prints the waitlist message in that case and exits 0.
	Skipped bool
	// Empty is true when the pending dir does not exist or held no
	// .json files. The cmd layer prints "Nothing to sync." and exits 0.
	Empty bool
	// Files is one entry per upload attempt, in the lexicographic
	// order the dir was scanned. Successful entries are deleted from
	// disk before the next upload runs.
	Files []FileResult
}

// Uploaded reports the count of successful uploads.
func (r SyncResult) Uploaded() int {
	n := 0
	for _, f := range r.Files {
		if f.OK {
			n++
		}
	}
	return n
}

// Failed reports the count of files left in pending/ after the run.
func (r SyncResult) Failed() int {
	n := 0
	for _, f := range r.Files {
		if !f.OK {
			n++
		}
	}
	return n
}

// SyncOptions wires the deps the cmd layer supplies. The Enabled
// field carries cloud.CloudReportingEnabled — passing it explicitly
// keeps the gate testable without flipping a const.
type SyncOptions struct {
	Enabled    bool
	PendingDir string
	Client     *Client
	Logger     zerolog.Logger
}

// Sync uploads every *.json file in PendingDir. Successful uploads
// are deleted; failures are left in place so the next `ruptor sync`
// retries them. The returned error is non-nil only on filesystem
// problems unrelated to a single file (e.g. unreadable dir) — upload
// errors are reported per-file in Result.Files so the cmd layer can
// render them all at once.
func Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	if !opts.Enabled {
		return SyncResult{Skipped: true}, nil
	}
	if opts.PendingDir == "" {
		return SyncResult{}, errors.New("cloud: PendingDir is required")
	}
	if opts.Client == nil {
		return SyncResult{}, errors.New("cloud: Client is required")
	}

	files, err := listPending(opts.PendingDir)
	if err != nil {
		return SyncResult{}, err
	}
	if len(files) == 0 {
		return SyncResult{Empty: true}, nil
	}

	res := SyncResult{Files: make([]FileResult, 0, len(files))}
	for _, name := range files {
		path := filepath.Join(opts.PendingDir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			res.Files = append(res.Files, FileResult{Name: name, OK: false, Error: err.Error()})
			continue
		}
		if err := opts.Client.UploadReport(ctx, body); err != nil {
			opts.Logger.Warn().Str("file", name).Err(err).Msg("cloud: sync upload failed")
			res.Files = append(res.Files, FileResult{Name: name, OK: false, Error: err.Error()})
			continue
		}
		// Best-effort delete: a successful upload that fails to
		// unlink will resurface on the next run. We surface the
		// upload as OK because the platform already has the report.
		if err := os.Remove(path); err != nil {
			opts.Logger.Warn().Str("file", name).Err(err).Msg("cloud: sync uploaded but local cleanup failed")
		}
		res.Files = append(res.Files, FileResult{Name: name, OK: true})
	}
	return res, nil
}

// listPending returns the *.json filenames in dir, sorted. Hidden
// files (.foo.json from atomic writes) are skipped so an in-flight
// run report does not get partially uploaded.
func listPending(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cloud: reading %s: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

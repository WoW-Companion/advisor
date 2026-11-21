package store

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// HTTPRangeReader is an io.ReaderAt over one URL, using HTTP Range requests.
type HTTPRangeReader struct {
	URL    string
	Client *http.Client
	size   int64
	reqs   atomic.Int64
}

// NewHTTPRangeReader finds the object's size with a HEAD request.
func NewHTTPRangeReader(client *http.Client, url string) (*HTTPRangeReader, error) {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Head(url)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("HEAD %s: %w", url, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HEAD %s: %s", url, resp.Status)
	}
	if resp.ContentLength < 0 {
		return nil, fmt.Errorf("HEAD %s: no Content-Length", url)
	}
	return &HTTPRangeReader{URL: url, Client: client, size: resp.ContentLength}, nil
}

func (r *HTTPRangeReader) Size() int64 { return r.size }

// Requests is the number of range requests made so far.
func (r *HTTPRangeReader) Requests() int64 { return r.reqs.Load() }

func (r *HTTPRangeReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.size {
		return 0, io.EOF
	}
	want := int64(len(p))
	if off+want > r.size {
		want = r.size - off
	}
	if want == 0 {
		return 0, nil
	}
	req, err := http.NewRequest(http.MethodGet, r.URL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+want-1))
	r.reqs.Add(1)
	resp, err := r.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("GET %s: %s (the server must honour Range)", r.URL, resp.Status)
	}
	n, err := io.ReadFull(resp.Body, p[:want])
	if err == nil && want < int64(len(p)) {
		err = io.EOF
	}
	return n, err
}

// HTTPSource serves `<base>/<table>.parquet` over HTTP range requests.
type HTTPSource struct {
	Base   string
	Client *http.Client
}

func (h HTTPSource) Open(table string) (io.ReaderAt, int64, error) {
	if !identRE.MatchString(table) {
		return nil, 0, fmt.Errorf("invalid table name %q", table)
	}
	r, err := NewHTTPRangeReader(h.Client, strings.TrimRight(h.Base, "/")+"/"+table+".parquet")
	if err != nil {
		return nil, 0, err
	}
	return r, r.Size(), nil
}

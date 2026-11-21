package dataset

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unpack writes the archive's `<build>/...` files into dir, refusing anything but regular files
// the manifest lists (plus manifest.json, which must equal the separately downloaded manifest).
func unpack(r io.Reader, build string, want Manifest, manifest []byte, dir string) error {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()
	listed := map[string]bool{}
	for _, t := range want.Tables {
		listed[t.Path] = true
	}
	seen := map[string]bool{}
	var total int64
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		name := path.Clean(h.Name)
		if h.Typeflag == tar.TypeDir {
			continue
		}
		rel, ok := strings.CutPrefix(name, build+"/")
		if !ok || h.Typeflag != tar.TypeReg || !safePath(rel) {
			return fmt.Errorf("archive entry %q is not allowed", h.Name)
		}
		if rel != "manifest.json" && !listed[rel] {
			return fmt.Errorf("archive file %q is not in the manifest", rel)
		}
		if seen[rel] {
			return fmt.Errorf("archive lists %q twice", rel)
		}
		seen[rel] = true
		total += h.Size
		if total > maxUnpacked {
			return errors.New("archive is too large")
		}
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), tr, h.Size); err != nil {
			return err
		}
	}
	for p := range listed {
		if !seen[p] {
			return fmt.Errorf("archive lacks %q", p)
		}
	}
	if !seen["manifest.json"] {
		return errors.New("archive lacks manifest.json")
	}
	got, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	if string(got) != string(manifest) {
		return errors.New("manifest.json differs from the release's manifest")
	}
	return nil
}

func writeFile(p string, r io.Reader, size int64) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, size))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != size {
		err = io.ErrUnexpectedEOF
	}
	return err
}

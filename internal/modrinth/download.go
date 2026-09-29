package modrinth

import (
	"archive/zip"
	"context"
	"crypto"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Progress func(downloaded, total int64)

// Stage downloads to the destination directory but leaves publication to the caller holding the instance lock.
func (c *Client) Stage(ctx context.Context, file File, dir string, progress Progress) (string, error) {
	return c.stage(ctx, file, dir, progress, "JAR", ".tmp-*.jar", false)
}

// StagePack downloads a Modrinth ZIP into a private staging file, verifies its
// hashes and ZIP structure, and leaves publication to the caller.
func (c *Client) StagePack(ctx context.Context, file File, dir string, progress Progress) (string, error) {
	if file.Filename == "" || strings.ContainsAny(file.Filename, `/\`) || !strings.EqualFold(filepath.Ext(file.Filename), ".zip") {
		return "", fmt.Errorf("invalid pack ZIP filename")
	}
	return c.stage(ctx, file, dir, progress, "ZIP", ".tmp-*.zip", true)
}

func (c *Client) stage(ctx context.Context, file File, dir string, progress Progress, kind, pattern string, verifyZIP bool) (name string, err error) {
	expected, e := hex.DecodeString(file.Hashes.SHA512)
	if e != nil || len(expected) != sha512.Size || strings.ToLower(file.Hashes.SHA512) != file.Hashes.SHA512 {
		return "", fmt.Errorf("invalid SHA-512")
	}
	if file.Size > maxJAR || file.Size < 0 {
		return "", fmt.Errorf("%s size limit exceeded", kind)
	}
	resp, e := c.request(ctx, "GET", file.URL, nil, true, 5*time.Minute)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxJAR || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "", fmt.Errorf("unsafe %s response", kind)
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp(dir, pattern)
	if e != nil {
		return "", e
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	sha := sha512.New() // SHA-1 is supplementary Modrinth metadata; SHA-512 gates publication.
	shaOne := crypto.SHA1.New()
	var hashes hash.Hash = sha
	reader := io.TeeReader(io.LimitReader(resp.Body, maxJAR+1), io.MultiWriter(hashes, shaOne))
	buf := make([]byte, 32<<10)
	var n int64
	for {
		k, readErr := reader.Read(buf)
		if k > 0 {
			w, wErr := tmp.Write(buf[:k])
			n += int64(w)
			if progress != nil {
				progress(n, file.Size)
			}
			if wErr != nil || w != k {
				err = errors.Join(wErr, io.ErrShortWrite)
				break
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				err = readErr
			}
			break
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
	}
	if err == nil && n > maxJAR {
		err = fmt.Errorf("%s size limit exceeded", kind)
	}
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("truncated %s response", kind)
	}
	if err == nil && file.Size > 0 && n != file.Size {
		err = fmt.Errorf("%s size mismatch", kind)
	}
	if err == nil && !strings.EqualFold(hex.EncodeToString(sha.Sum(nil)), file.Hashes.SHA512) {
		err = fmt.Errorf("%s SHA-512 mismatch", kind)
	}
	if err == nil && file.Hashes.SHA1 != "" && !strings.EqualFold(hex.EncodeToString(shaOne.Sum(nil)), file.Hashes.SHA1) {
		err = fmt.Errorf("%s SHA-1 mismatch", kind)
	}
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return "", err
	}
	if verifyZIP {
		var archive *zip.ReadCloser
		archive, err = zip.OpenReader(tmp.Name())
		if err != nil {
			return "", fmt.Errorf("invalid pack ZIP: %w", err)
		}
		if err = archive.Close(); err != nil {
			return "", err
		}
	}
	return filepath.Clean(tmp.Name()), nil
}

package modrinth

import (
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
func (c *Client) Stage(ctx context.Context, file File, dir string, progress Progress) (name string, err error) {
	expected, e := hex.DecodeString(file.Hashes.SHA512)
	if e != nil || len(expected) != sha512.Size || strings.ToLower(file.Hashes.SHA512) != file.Hashes.SHA512 {
		return "", fmt.Errorf("invalid SHA-512")
	}
	if file.Size > maxJAR || file.Size < 0 {
		return "", fmt.Errorf("JAR size limit exceeded")
	}
	resp, e := c.request(ctx, "GET", file.URL, nil, true, 5*time.Minute)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxJAR || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "", fmt.Errorf("unsafe JAR response")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp(dir, ".tmp-*.jar")
	if e != nil {
		return "", e
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	sha := sha512.New() // SHA-1 is only checked as supplementary Modrinth metadata; SHA-512 gates publication.
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
		err = fmt.Errorf("JAR size limit exceeded")
	}
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("truncated JAR response")
	}
	if err == nil && file.Size > 0 && n != file.Size {
		err = fmt.Errorf("JAR size mismatch")
	}
	if err == nil && !strings.EqualFold(hex.EncodeToString(sha.Sum(nil)), file.Hashes.SHA512) {
		err = fmt.Errorf("JAR SHA-512 mismatch")
	}
	if err == nil && file.Hashes.SHA1 != "" && !strings.EqualFold(hex.EncodeToString(shaOne.Sum(nil)), file.Hashes.SHA1) {
		err = fmt.Errorf("JAR SHA-1 mismatch")
	}
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return "", err
	}
	return filepath.Clean(tmp.Name()), nil
}

// SecureDelete overwrites the file with 3 passes then deletes it.
//
// Passes: random bytes → zero bytes → random bytes, each followed by fsync.
// This makes data recovery significantly harder on HDDs. On SSDs, wear
// leveling may retain copies in over-provisioned sectors — this is
// best-effort without kernel/hardware-level assistance.
//
// Returns nil if the file does not exist.
package utils

import (
	"crypto/rand"
	"io"
	"os"
)

func SecureDelete(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	size := info.Size()
	if size == 0 {
		return os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	const chunkSize = 32 * 1024 // 32 KB
	buf := make([]byte, chunkSize)

	for pass := 0; pass < 3; pass++ {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		remaining := size
		for remaining > 0 {
			n := int64(chunkSize)
			if remaining < n {
				n = remaining
			}
			if pass == 1 {
				// zeros pass
				for i := range buf[:n] {
					buf[i] = 0
				}
			} else {
				if _, err := rand.Read(buf[:n]); err != nil {
					f.Close()
					return err
				}
			}
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			remaining -= n
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
	}

	f.Close()
	return os.Remove(path)
}

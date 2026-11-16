package jsonfile

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
)

// Changed reports whether the file's content differs from since, and returns
// the current fingerprint. It is cheap enough to poll: it stats the file and
// reads and hashes it only when the inode, modification time or size moved
// since the last call. A touch that leaves the bytes alone is not a change. A
// missing file has the fingerprint Missing, so a deleted file is a change
// unless since is Missing too. Changed takes no lock and does not decode or
// validate the file.
func (s *Store[T]) Changed(since Fingerprint) (changed bool, now Fingerprint, err error) {
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return since != Missing, Missing, nil
	}
	if err != nil {
		return false, since, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Caveat: if the inode is reused by a new file of the same size within
	// one mtime tick, this reuses a stale fingerprint. That is acceptable for
	// a poller, which catches the next real change.
	if s.statInfo != nil && os.SameFile(s.statInfo, info) &&
		s.statInfo.ModTime().Equal(info.ModTime()) && s.statInfo.Size() == info.Size() {
		return s.statFP != since, s.statFP, nil
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return since != Missing, Missing, nil
	}
	if err != nil {
		return false, since, err
	}
	fp := Fingerprint(sha256.Sum256(data))
	s.statInfo, s.statFP = info, fp
	return fp != since, fp, nil
}

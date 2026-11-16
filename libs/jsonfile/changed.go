package jsonfile

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"time"
)

// racyWindow is how close to the time it was cached a file's mtime may be
// before the cached fingerprint is not trusted (git's "racy stat" rule): a
// write within the same mtime tick as the caching stat leaves the stat
// unchanged. 2 s covers the coarsest common clocks (FAT).
const racyWindow = 2 * time.Second

// Changed reports whether the file's content differs from since, and returns
// the current fingerprint. It is cheap enough to poll: it stats the file and
// reads and hashes it only when the stat no longer matches the last call's
// (file identity, modification time or size moved), or when the cached mtime
// was within racyWindow of the time it was cached, since a write in the same
// mtime tick would not show in the stat. A touch that leaves the bytes alone
// is not a change. A missing file has the fingerprint Missing, so a deleted
// file is a change unless since is Missing too. Changed takes no lock
// and does not decode or validate the file.
//
// The cache can still miss a change, and report the old fingerprint, when
// the stat matches although the bytes differ: a file rewritten with the same
// size whose mtime was set back on purpose (touch -r, a restore tool), or a
// new file of the same size and mtime that got the old one's inode number.
// That is acceptable for a poller: it catches the next real change.
func (s *Store[T]) Changed(since Fingerprint) (changed bool, now Fingerprint, err error) {
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return since != Missing, Missing, nil
	}
	if err != nil {
		return false, since, err
	}
	// On Windows the file ID SameFile compares is loaded lazily, from the path,
	// on first use: comparing a cached FileInfo later would load the ID of
	// whatever file is at the path then, so a replaced file would always match.
	// Comparing info with itself now pins the ID of the file just stat'ed. On
	// other systems the ID comes with the stat and this costs nothing.
	_ = os.SameFile(info, info)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statInfo != nil && os.SameFile(s.statInfo, info) &&
		s.statInfo.ModTime().Equal(info.ModTime()) && s.statInfo.Size() == info.Size() &&
		s.statAt.Sub(info.ModTime()) >= racyWindow {
		return s.statFP != since, s.statFP, nil
	}

	statAt := time.Now()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return since != Missing, Missing, nil
	}
	if err != nil {
		return false, since, err
	}
	fp := Fingerprint(sha256.Sum256(data))
	s.statInfo, s.statFP, s.statAt = info, fp, statAt
	return fp != since, fp, nil
}

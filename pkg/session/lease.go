package session

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ErrStoreOwned reports that another process holds the session store lease for
// this workspace. The remedy is to use the running engine instead
// (excelsior --engine <url>).
var ErrStoreOwned = errors.New("session store owned by another process; connect to the running engine instead (--engine <url>)")

// storeLease is an OS-held exclusive lock on the session store. The OS
// releases it automatically when the owning process exits, so a crash never
// leaves a stale lock behind.
type storeLease struct {
	f    *os.File
	refs int
}

var (
	leaseMu sync.Mutex
	leases  = map[string]*storeLease{}
)

func leaseKey(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	dir = filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	return dir
}

// acquireStoreLease takes the workspace's store lease. Re-acquiring within the
// same process is idempotent; a second process gets ErrStoreOwned.
func acquireStoreLease(dir string) error {
	key := leaseKey(dir)
	leaseMu.Lock()
	defer leaseMu.Unlock()
	if l := leases[key]; l != nil {
		l.refs++
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := openLockFile(filepath.Join(dir, ".owner.lock"))
	if err != nil {
		return err
	}
	if err := lockFileExclusive(f); err != nil {
		f.Close()
		return ErrStoreOwned
	}
	leases[key] = &storeLease{f: f, refs: 1}
	return nil
}

// releaseStoreLease drops one reference; the last reference unlocks.
func releaseStoreLease(dir string) {
	key := leaseKey(dir)
	leaseMu.Lock()
	defer leaseMu.Unlock()
	l := leases[key]
	if l == nil {
		return
	}
	l.refs--
	if l.refs <= 0 {
		delete(leases, key)
		l.f.Close()
	}
}

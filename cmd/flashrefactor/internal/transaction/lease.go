package transaction

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
)

const (
	metadataDirectory = ".flashrefactor/transaction"
	leaseFile         = metadataDirectory + "/lease"
)

// workspaceLease is both a persistent ownership marker and an advisory
// process lock. The marker prevents a later Run from guessing that a stale
// crash state is safe; the advisory lock lets an explicit recovery operation
// distinguish a live holder from a dead one.
type workspaceLease struct {
	path string
	file *os.File
	info fs.FileInfo
}

var activeLeases sync.Map // canonical lease path -> struct{}

func metadataPath(path string) bool {
	return path == ".flashrefactor" || path == ".flashrefactor.transaction.lease" || len(path) > len(".flashrefactor") && path[:len(".flashrefactor")+1] == ".flashrefactor/"
}

func acquireLease(root string) (*workspaceLease, error) {
	metadata, err := prepareMetadataDirectory(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(metadata, "lease")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, fs.ErrExist) {
		return nil, ErrWorkspaceBusy
	}
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(file); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "version=1\npid=%d\n", os.Getpid()); err != nil {
		_ = unlock(file)
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = unlock(file)
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := syncDirectory(metadata); err != nil {
		_ = unlock(file)
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = unlock(file)
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	activeLeases.Store(path, struct{}{})
	return &workspaceLease{path: path, file: file, info: info}, nil
}

// acquireRecoveryLease is deliberately not used by Run. Only an explicit
// recovery operation reaches this path, and it fails when the crash marker is
// still locked by a living owner.
func acquireRecoveryLease(root string) (*workspaceLease, error) {
	path := filepath.Join(root, filepath.FromSlash(leaseFile))
	if _, active := activeLeases.Load(path); active {
		return nil, ErrWorkspaceBusy
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoRecovery
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("transaction recovery lease is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(file); err != nil {
		_ = file.Close()
		if errors.Is(err, errLeaseLocked) {
			return nil, ErrWorkspaceBusy
		}
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = unlock(file)
		_ = file.Close()
		return nil, err
	}
	if !os.SameFile(info, opened) {
		_ = unlock(file)
		_ = file.Close()
		return nil, fmt.Errorf("transaction recovery lease changed while opening")
	}
	return &workspaceLease{path: path, file: file, info: opened}, nil
}

func (t *transaction) releaseLease() error {
	if t.lease == nil {
		return nil
	}
	lease := t.lease
	t.lease = nil
	activeLeases.Delete(lease.path)
	unLockErr := unlock(lease.file)
	closeErr := lease.file.Close()
	current, statErr := os.Lstat(lease.path)
	if statErr == nil && os.SameFile(current, lease.info) {
		removeErr := os.Remove(lease.path)
		if removeErr == nil {
			removeErr = syncDirectory(filepath.Dir(lease.path))
		}
		if removeErr == nil {
			removeErr = removeEmptyMetadata(filepath.Dir(lease.path))
		}
		return errors.Join(unLockErr, closeErr, removeErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		return errors.Join(unLockErr, closeErr)
	}
	if statErr != nil {
		return errors.Join(unLockErr, closeErr, statErr)
	}
	return errors.Join(unLockErr, closeErr, fmt.Errorf("workspace lease path changed during transaction"))
}

// abandon releases the advisory process lock but deliberately keeps the
// persistent marker and journal for explicit inspect/rollback/complete.
func (t *transaction) abandon(cause error) error {
	if t.lease == nil {
		return cause
	}
	lease := t.lease
	t.lease = nil
	activeLeases.Delete(lease.path)
	return errors.Join(cause, unlock(lease.file), lease.file.Close())
}

func prepareMetadataDirectory(root string) (string, error) {
	container := filepath.Join(root, ".flashrefactor")
	containerInfo, err := os.Lstat(container)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(container, 0700); err != nil {
			return "", err
		}
		if err := syncDirectory(root); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if !containerInfo.IsDir() || containerInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("flashrefactor metadata root is not a real directory")
	}
	metadata := filepath.Join(root, metadataDirectory)
	info, err := os.Lstat(metadata)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(metadata, 0700); err != nil {
			return "", err
		}
		if err := syncDirectory(container); err != nil {
			return "", err
		}
		return metadata, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("transaction metadata is not a real directory")
	}
	entries, err := os.ReadDir(metadata)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", ErrWorkspaceBusy
	}
	return metadata, nil
}

func removeEmptyMetadata(metadata string) error {
	entries, err := os.ReadDir(metadata)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	if err := os.Remove(metadata); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(metadata))
}

func rejectHardLink(path string, info fs.FileInfo) error {
	links, known := hardLinkCount(info)
	if !known {
		return fmt.Errorf("cannot determine hard-link count for transaction input %q", path)
	}
	if links > 1 {
		return fmt.Errorf("transaction input %q is hard-linked (%d links)", path, links)
	}
	return nil
}

func hardLinkCount(info fs.FileInfo) (uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName("Nlink")
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() >= 0 {
			return uint64(field.Int()), true
		}
	}
	return 0, false
}

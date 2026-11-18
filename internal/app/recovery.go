// INPUT: 未保存缓冲与 appdir 缓存目录。
// OUTPUT: 崩溃恢复记录的读写。
// POS: 意外退出后找回编辑内容。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"revtrail/locales"
)

type recoveryRecord struct {
	Root, Path, Base, Text string
	BOM                    bool
}

func recoveryPath(root, path string) (string, error) {
	dir, err := draftDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root + "\x00" + path))
	return filepath.Join(dir, fmt.Sprintf("recovery-%x.json", sum)), nil
}

func readRecovery(root, path string) *recoveryRecord {
	name, err := recoveryPath(root, path)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil
	}
	var r recoveryRecord
	if json.Unmarshal(data, &r) != nil || r.Root != root || r.Path != path {
		return nil
	}
	return &r
}

func (a *App) cacheRecovery(d *doc) {
	if !d.buf.Modified() {
		a.removeRecovery(d)
		return
	}
	a.scheduleIdleSave(a.idleSaveDelay)
	if d.cache == nil {
		d.cache = &draftCache{}
	}
	cache := d.cache
	name, err := recoveryPath(a.root, d.path)
	if err != nil {
		a.note(locales.MsgCouldNotCacheUnsavedFile()+err.Error(), true)
		return
	}
	snap := d.buf.Snapshot()
	record := recoveryRecord{Root: a.root, Path: d.path, Base: d.diskText, Text: snap.String(), BOM: d.bom}
	a.writeCachedSnapshot(cache, snap, name, ".recovery-", func(f *os.File) error {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	})
}

func (a *App) removeRecovery(d *doc) {
	name, err := recoveryPath(a.root, d.path)
	if err != nil {
		return
	}
	cache := d.cache
	d.cache = nil
	if cache == nil {
		_ = os.Remove(name)
		return
	}
	// Serialize removal with pending writes, invalidating them before returning.
	cache.mu.Lock()
	cache.removed = true
	_ = os.Remove(name)
	cache.mu.Unlock()
}

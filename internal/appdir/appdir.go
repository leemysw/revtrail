// INPUT: 系统的用户配置 / 缓存目录。
// OUTPUT: ConfigPath、CachePath：Revtrail 目录下的路径。
// POS: 所有本机数据路径的唯一来源。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package appdir

import (
	"os"
	"path/filepath"
)

func under(base string) string { return filepath.Join(base, "Revtrail") }

func joinUnder(baseDir func() (string, error), elem ...string) (string, error) {
	dir, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{under(dir)}, elem...)...), nil
}

// ConfigPath returns a path inside Revtrail's per-user configuration directory.
func ConfigPath(elem ...string) (string, error) {
	return joinUnder(os.UserConfigDir, elem...)
}

// CachePath returns a path inside Revtrail's per-user cache directory.
func CachePath(elem ...string) (string, error) {
	return joinUnder(os.UserCacheDir, elem...)
}

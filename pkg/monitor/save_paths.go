package monitor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func esperCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	if d == "" {
		return "", errors.New("user cache dir unavailable")
	}
	return filepath.Join(d, "esper"), nil
}

func makeTimestampedPath(prefix, ext string, at time.Time) (string, error) {
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}

	dir, err := esperCacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	stamp := at.Format("20060102-150405")
	base := filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, stamp, ext))
	if _, err := os.Stat(base); err != nil && os.IsNotExist(err) {
		return base, nil
	}
	// Avoid collisions (multiple actions in the same second).
	for i := 2; i < 1000; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%s-%s-%d%s", prefix, stamp, i, ext))
		if _, err := os.Stat(p); err != nil && os.IsNotExist(err) {
			return p, nil
		}
	}
	return "", fmt.Errorf("could not allocate unique path under %s", dir)
}

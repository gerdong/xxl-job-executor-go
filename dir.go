package xxl

import (
	"os"
	"time"
)

func exists(dir string) bool {
	_, err := os.Stat(dir)
	return err == nil
}

func findExpireDir(days int, dir string) []string {
	r := make([]string, 0)

	t := time.Now()
	ld := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, days*(-1))

	s, _ := os.ReadDir(dir)
	for _, entry := range s {
		if fdate, err := time.Parse("2006-01-02", entry.Name()); err == nil {
			if fdate.Before(ld) {
				r = append(r, entry.Name())
			} else {
				// ReadDir是排序后的结果
				break
			}
		}
	}
	return r
}

func removeDir(dir string) {
	_ = os.RemoveAll(dir)
}

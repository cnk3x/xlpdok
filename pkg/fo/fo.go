package fo

import (
	"io/fs"
	"os"
	"path/filepath"
)

type Option func(file string) error

func Chown(uid, gid int, recursive ...bool) Option {
	return func(file string) error {
		if len(recursive) == 0 || !recursive[0] {
			return os.Chown(file, uid, gid)
		}
		return filepath.WalkDir(file, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chown(path, uid, gid)
		})
	}
}

func RChown(uid, gid int) Option { return Chown(uid, gid, true) }

func Chmod(mode os.FileMode, recursive ...bool) Option {
	return func(file string) error {
		if len(recursive) == 0 || !recursive[0] {
			return os.Chmod(file, mode)
		}
		return filepath.WalkDir(file, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chmod(path, mode)
		})
	}
}

func RChmod(mode os.FileMode) Option { return Chmod(mode, true) }

func Apply(file string, options ...Option) error {
	for _, option := range options {
		if err := option(file); err != nil {
			return err
		}
	}
	return nil
}

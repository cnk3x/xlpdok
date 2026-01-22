package sys

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"xlpdok/pkg/fo"
)

type Runner func() error

func Exec(runners ...Runner) (err error) {
	for _, runner := range runners {
		if err = runner(); err != nil {
			return
		}
	}
	return
}

func Rmfile(file string) Runner {
	return func() (err error) {
		if err = os.Remove(file); os.IsNotExist(err) {
			err = nil
		}
		return
	}
}

func Mkdir(dir string, options ...fo.Option) Runner {
	return func() error {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return err
		}
		return fo.Apply(dir, options...)
	}
}

func Mkdirs(dirs []string, options ...fo.Option) Runner {
	return func() error {
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0777); err != nil {
				return err
			}
			if err := fo.Apply(dir, options...); err != nil {
				return err
			}
		}
		return nil
	}
}

func Mkfile[T ~[]byte | ~string](name string, data []T, overwrite bool, options ...fo.Option) Runner {
	return func() (err error) {
		if err = os.MkdirAll(filepath.Dir(name), 0777); err != nil {
			return
		}

		var f *os.File
		if overwrite {
			f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		} else {
			f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		}

		if err == nil {
			err = func() (err error) {
				for i, line := range data {
					if i > 0 {
						if _, err = f.Write([]byte("\n")); err != nil {
							return
						}
					}
					if _, err = f.Write([]byte(line)); err != nil {
						return
					}
				}
				return
			}()

			if e := f.Close(); e != nil && err == nil {
				err = e
			}
		}

		if !overwrite && os.IsExist(err) {
			err = nil
		}

		if err == nil {
			err = fo.Apply(name, options...)
		}

		return
	}
}

func Unshare(flags int) Runner { return func() (err error) { return syscall.Unshare(flags) } }

func Mount(source, target, fstype string, flags uintptr, data string) Runner {
	return func() error { return syscall.Mount(source, target, fstype, flags, data) }
}

func Chown(path string, uid, gid int, recursive ...bool) Runner {
	return func() error {
		if len(recursive) == 0 || !recursive[0] {
			return os.Chown(path, uid, gid)
		}
		return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chown(path, uid, gid)
		})
	}
}

func RunAs(uid, gid int, runners ...Runner) func() error {
	return func() (err error) {
		if uid < 1 {
			uid = -1
		}

		if gid < 1 {
			gid = -1
		}

		if uid > 0 || gid > 0 {
			if err = syscall.Setegid(gid); err != nil {
				return
			}
			defer syscall.Setegid(0)

			if err = syscall.Seteuid(uid); err != nil {
				return
			}
			defer syscall.Seteuid(0)
		}

		return Exec(runners...)
	}
}

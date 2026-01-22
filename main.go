package main

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"xlpdok/embed"
	"xlpdok/spk"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	SYNOPKG_DSM_VERSION_MAJOR = "7"              //系统的主版本
	SYNOPKG_DSM_VERSION_MINOR = "2"              //系统的次版本
	SYNOPKG_DSM_VERSION_BUILD = "64570"          //系统的编译版本
	SYNOPKG_PKGNAME           = "pan-xunlei-com" //包名

	DIR_SYNOPKG_PKGROOT = "/var/packages/" + SYNOPKG_PKGNAME //包安装目录
	DIR_SYNOPKG_PKGDEST = DIR_SYNOPKG_PKGROOT + "/target"    //包安装目录
	DIR_SYNOPKG_WORK    = DIR_SYNOPKG_PKGDEST + "/bin"       //

	FILE_PAN_XUNLEI_VER = DIR_SYNOPKG_PKGDEST + "/bin/bin/version"                                   //版本文件
	FILE_PAN_XUNLEI_CLI = DIR_SYNOPKG_PKGDEST + "/bin/bin/xunlei-pan-cli-launcher." + runtime.GOARCH //启动器
	FILE_INDEX_CGI      = DIR_SYNOPKG_PKGDEST + "/ui/index.cgi"                                      //CGI文件路径

	DIR_VAR              = DIR_SYNOPKG_PKGDEST + "/var"                       //SYNOPKG_PKGROOT
	FILE_PID             = DIR_VAR + "/" + SYNOPKG_PKGNAME + ".pid"           //进程文件
	SOCK_LAUNCHER_LISTEN = DIR_VAR + "/" + SYNOPKG_PKGNAME + "-launcher.sock" //启动器监听地址
	SOCK_DRIVE_LISTEN    = DIR_VAR + "/" + SYNOPKG_PKGNAME + ".sock"          //主程序监听地址

	FILE_SYNO_INFO_CONF        = "/etc/synoinfo.conf"                                //synoinfo.conf 文件路径
	FILE_SYNO_AUTHENTICATE_CGI = "/usr/syno/synoman/webman/modules/authenticate.cgi" //syno...authenticate.cgi 文件路径

	SYNO_PLATFORM = "geminilake"                                                                                                            //平台
	SYNO_MODEL    = "DS920+"                                                                                                                //平台
	SYNO_VERSION  = SYNO_PLATFORM + " dsm " + SYNOPKG_DSM_VERSION_MAJOR + "." + SYNOPKG_DSM_VERSION_MINOR + "-" + SYNOPKG_DSM_VERSION_BUILD //系统版本
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := newnsRun(ctx); err != nil {
		slog.ErrorContext(ctx, "app exited!", "err", err)
	} else {
		slog.InfoContext(ctx, "app exited!")
	}

	<-ctx.Done()
}

func newnsRun(ctx context.Context, runner func(ctx context.Context) error) (err error) {
	// 创建新的挂载命名空间 (CLONE_NEWNS)，这是关键步骤，它允许我们在不影响宿主机的情况下修改挂载点
	if err = syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
		err = fmt.Errorf("failed to create new mount namespace: %v", err)
		return
	}

	// 设置挂载传播为私有，防止影响其他命名空间
	if err = syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		err = fmt.Errorf("failed to set mount propagation to private: %v", err)
		return
	}

	// 重新挂载 /proc 以使 procfs 中的信息反映新的命名空间
	// 这会使得 /proc/self/cgroup, /proc/mounts 等看起来像在非容器环境中
	if e := syscall.Mount("none", "/proc", "proc", 0, ""); e != nil {
		// 如果失败，继续执行，因为有时这并不致命，或者已经在正确的命名空间中
		slog.WarnContext(ctx, "failed to remount /proc (this might be okay if running outside initial NS)", "err", e)
	} else {
		slog.DebugContext(ctx, "successfully remounted /proc in new namespace.")
	}

	os.Remove("/.dockerenv")
	os.MkdirAll("/data", 0777)
	os.MkdirAll("/downloads", 0777)
	os.MkdirAll("/var/packages/pan-xunlei-com/target/var", 0777)

	// embed.ExtractEmbed("/")

	spkUrl := "file:///tmp/xl.spk"
	slog.InfoContext(ctx, "download", "spk", spkUrl, "save", DIR_SYNOPKG_PKGDEST)
	if err = spk.Download(ctx, spkUrl, DIR_SYNOPKG_PKGDEST, false); err != nil {
		return
	}

	if err = mockSyno(); err != nil {
		return
	}

	err = runas(ctx, 1000, 1000, launch)
	return
}

func launch(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, FILE_PAN_XUNLEI_CLI, "-launcher_listen", "unix://"+SOCK_LAUNCHER_LISTEN, "-pid", FILE_PID, "-update_url", "null")
	cmd.Dir = DIR_SYNOPKG_WORK
	cmd.Env = mockEnv("/data", "/downloads")
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNS, Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) }
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err = cmd.Start(); err != nil {
		slog.ErrorContext(ctx, "start", "cmdline", strings.Join(cmd.Args, " "), "err", err)
		return
	}
	slog.InfoContext(ctx, "start", "cmdline", strings.Join(cmd.Args, " "))

	go mockWeb(ctx, cmd.Env, cancel)
	return cmd.Wait()
}

func mockSyno() (err error) {
	err = fileWrite(FILE_SYNO_INFO_CONF, 0666,
		fmt.Sprintf(`platform_name=%q`, SYNO_PLATFORM),
		fmt.Sprintf(`synobios=%q`, SYNO_PLATFORM),
		fmt.Sprintf(`unique=synology_%s_%s`, SYNO_PLATFORM, SYNO_MODEL),
	)
	if err != nil {
		return
	}

	err = fileWrite(FILE_SYNO_AUTHENTICATE_CGI, 0777, embed.AuthenticateGgi)
	return
}

func mockWeb(ctx context.Context, env []string, onDone func()) (err error) {
	defer onDone()
	mux := chi.NewMux()
	mux.Use(middleware.Recoverer)

	const CGI_PATH = "/webman/3rdparty/" + SYNOPKG_PKGNAME + "/index.cgi/"
	var cgiRedir = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, CGI_PATH, 308) })
	mux.Get("/", cgiRedir)
	mux.Get("/web", cgiRedir)
	mux.Get("/webman", cgiRedir)

	mux.Mount(CGI_PATH, &cgi.Handler{
		Dir:  DIR_SYNOPKG_WORK,
		Path: FILE_INDEX_CGI,
		Env:  env,
	})

	token := randText(13)
	mux.Handle("/webman/login.cgi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"SynoToken":%q,"result":"success","success":true}`, token)
	}))

	s := &http.Server{Addr: ":2345", Handler: mux, BaseContext: func(l net.Listener) context.Context {
		slog.InfoContext(ctx, "dashboard started", "listen", l.Addr().String())
		return ctx
	}}

	if err = s.ListenAndServe(); err != nil {
		slog.WarnContext(ctx, "dashboard done", "err", err)
	} else {
		slog.InfoContext(ctx, "dashboard done")
	}
	return
}

func mockEnv(dirData, dirDownload string) []string {
	osEnv := os.Environ()
	environ := make([]string, 0, len(osEnv))
	excludes := []string{"container", "KUBERNETES_SERVICE_HOST", "KUBERNETES_PORT", "DOCKER_IMAGE", "DOCKER_TAG"}
	for _, env := range osEnv {
		if slices.ContainsFunc(excludes, func(key string) bool { return strings.HasPrefix(env, key+"=") }) {
			continue
		}
		environ = append(environ, env)
	}

	return append(environ,
		"SYNOPLATFORM="+SYNO_PLATFORM,
		"SYNOPKG_PKGNAME="+SYNOPKG_PKGNAME,
		"SYNOPKG_PKGDEST="+DIR_SYNOPKG_PKGDEST,
		"SYNOPKG_DSM_VERSION_MAJOR="+SYNOPKG_DSM_VERSION_MAJOR,
		"SYNOPKG_DSM_VERSION_MINOR="+SYNOPKG_DSM_VERSION_MINOR,
		"SYNOPKG_DSM_VERSION_BUILD="+SYNOPKG_DSM_VERSION_BUILD,
		"DriveListen=unix://"+SOCK_DRIVE_LISTEN,
		"PLATFORM=群晖",
		"OS_VERSION="+SYNO_VERSION,
		"ConfigPath="+dirData,
		"HOME="+filepath.Join(dirData, ".drive"),
		"DownloadPATH="+dirDownload,
		"GIN_MODE=release",
		// "LD_LIBRARY_PATH=/var/packages/pan-xunlei-com/lib",
	)
}

func randText(n int) (s string) {
	var d = make([]byte, (n+4)/8*5)
	_, _ = rand.Read(d)
	if s = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding).EncodeToString(d); len(s) > n {
		s = s[:n]
	}
	return
}

func runas(ctx context.Context, uid, gid int, runner func(ctx context.Context) error) (err error) {
	if err = syscall.Setegid(gid); err != nil {
		return
	}
	defer syscall.Setegid(0)

	if err = syscall.Seteuid(uid); err != nil {
		return
	}
	defer syscall.Seteuid(0)

	return runner(ctx)
}

func fileWrite[T ~[]byte | ~string](name string, perm fs.FileMode, data ...T) (err error) {
	var f *os.File
	if f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm); err != nil {
		if os.IsExist(err) {
			err = nil
			return
		}
		return
	}

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
	return
}

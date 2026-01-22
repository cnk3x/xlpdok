package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"xlpdok/pkg/arrx"
	"xlpdok/pkg/embed"
	"xlpdok/pkg/fo"
	"xlpdok/pkg/spk"
	"xlpdok/pkg/sys"

	"github.com/cnk3x/flags"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
)

const (
	SYNOPKG_PKGNAME = "pan-xunlei-com" //包名

	DIR_SYNOPKG_PKGROOT = "/var/packages/pan-xunlei-com"                                                          // 包安装目录
	DIR_SYNOPKG_PKGDEST = "/var/packages/pan-xunlei-com/target"                                                   // 包安装目录
	DIR_SYNOPKG_WORK    = "/var/packages/pan-xunlei-com/target/bin"                                               //
	FILE_PAN_XUNLEI_VER = "/var/packages/pan-xunlei-com/target/bin/bin/version"                                   // 版本文件
	FILE_PAN_XUNLEI_CLI = "/var/packages/pan-xunlei-com/target/bin/bin/xunlei-pan-cli-launcher." + runtime.GOARCH // 启动器
	FILE_INDEX_CGI      = "/var/packages/pan-xunlei-com/target/ui/index.cgi"                                      // CGI文件路径
	DIR_VAR             = "/var/packages/pan-xunlei-com/target/var"                                               // SYNOPKG_PKGROOT
	// FILE_PID            = "/var/packages/pan-xunlei-com/target/var/pan-xunlei-com.pid"                            // 进程文件
	// FILE_SOCK_LAUNCHER  = "/var/packages/pan-xunlei-com/target/var/pan-xunlei-com-launcher.sock"                  // 启动器监听地址
	// FILE_SOCK_DRIVE     = "/var/packages/pan-xunlei-com/target/var/pan-xunlei-com.sock"                           // 主程序监听地址

	SYNO_PLATFORM             = "geminilake"               // 平台
	SYNO_MODEL                = "DS920+"                   //
	SYNOPKG_DSM_VERSION_MAJOR = "7"                        // 系统的主版本
	SYNOPKG_DSM_VERSION_MINOR = "2"                        // 系统的次版本
	SYNOPKG_DSM_VERSION_BUILD = "64570"                    // 系统的编译版本
	SYNO_VERSION              = "geminilake dsm 7.2-64570" // 系统版本

	FILE_SYNO_INFO_CONF        = "/etc/synoinfo.conf"                                //synoinfo.conf 文件路径
	FILE_SYNO_AUTHENTICATE_CGI = "/usr/syno/synoman/webman/modules/authenticate.cgi" //syno...authenticate.cgi 文件路径
)

type Config struct {
	Listen        string   `flag:"" short:"l" usage:"面板监听地址" env:"XL_LISTEN" json:"listen,omitempty"`
	DirDownload   []string `flag:"" short:"d" usage:"下载保存路径，多个路径以冒号:隔开" env:"XL_DIR_DOWNLOAD" json:"dir_download,omitempty"`
	DirData       string   `flag:"" short:"c" usage:"账号保存路径" env:"XL_DIR_DATA" json:"dir_data,omitempty"`
	Uid           int      `flag:"" short:"u" usage:"运行spk的UID" env:"XL_UID" json:"uid,omitempty"`
	Gid           int      `flag:"" short:"g" usage:"运行spk的GID" env:"XL_GID" json:"gid,omitempty"`
	PreventUpdate bool     `flag:"" usage:"禁止更新" env:"XL_PREVENT_UPDATE" json:"prevent_update,omitempty"`
	Busybox       bool     `flag:"" usage:"使用内嵌Busybox文件系统" env:"XL_BUSYBOX" json:"busybox,omitempty"`
}

var BuildTime string
var Version = "0.1.0-beta"

func main() {
	var cfg Config
	fSet := flags.NewSet(flags.SetVersion(Version), flags.SetBuildTime(BuildTime), flags.SetDescription("xunlei wrap"))
	fSet.Struct(&cfg)
	fSet.Parse()

	slog.SetDefault(slog.New(tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{Level: slog.LevelDebug})))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := configCheck(&cfg); err != nil {
		slog.ErrorContext(ctx, "app exited!", "err", err)
		return
	}

	if err := Run(ctx, cfg); err != nil {
		slog.ErrorContext(ctx, "app exited!", "err", err)
	} else {
		slog.InfoContext(ctx, "app exited!")
	}

	<-ctx.Done()
}

func configCheck(cfg *Config) (err error) {
	var dirDownload []string
	for _, d := range cfg.DirDownload {
		for p := range strings.SplitSeq(d, ":") {
			if p = strings.TrimSpace(p); p != "" {
				if p, err = filepath.Abs(p); err != nil {
					return
				}
				dirDownload = append(dirDownload, p)
			}
		}
	}
	cfg.DirDownload = dirDownload
	if len(dirDownload) == 0 {
		cfg.DirDownload = append(cfg.DirDownload, "/xunlei/downloads")
	}

	if cfg.DirData, err = filepath.Abs(cmp.Or(strings.TrimSpace(cfg.DirData), "/xunlei/data")); err != nil {
		return
	}

	cfg.Listen = cmp.Or(cfg.Listen, ":2345")
	return
}

func Run(ctx context.Context, cfg Config) (err error) {
	if cfg.Busybox {
		embed.ExtractEmbed("/")
	}

	confContent := arrx.Stoa(`platform_name="`+SYNO_PLATFORM+`"`, `synobios="`+SYNO_PLATFORM+`"`, `unique="synology_`+SYNO_PLATFORM+`_`+SYNO_MODEL+`"`)
	return sys.Exec(
		sys.Mkfile(FILE_SYNO_INFO_CONF, confContent, false),
		sys.Mkfile(FILE_SYNO_AUTHENTICATE_CGI, arrx.Stoa(embed.AuthenticateGgi), false, fo.Chmod(0777)),
		sys.Rmfile("/.dockerenv"),
		sys.Mkdir(cfg.DirData, fo.RChmod(0777), fo.RChown(cfg.Uid, cfg.Gid)),
		sys.Mkdirs(cfg.DirDownload, fo.Chmod(0777)),
		sys.Unshare(syscall.CLONE_NEWNS|syscall.CLONE_NEWPID|syscall.CLONE_NEWUTS),
		// sys.Unshare(syscall.CLONE_NEWNS),
		sys.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""),
		sys.Mkdir("/proc", fo.Chmod(0755)),
		sys.Mount("none", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, ""),
		downloadSpk(ctx, spk.DownloadUrl),
		sys.Chown(DIR_SYNOPKG_PKGDEST, cfg.Uid, cfg.Gid, true),
		sys.Mkdir(DIR_VAR, fo.Chmod(0777, true), fo.Chown(cfg.Uid, cfg.Gid, true)),
		launch(ctx, cfg),
	)
}

func launch(ctx context.Context, cfg Config) func() error {
	return sys.RunAs(cfg.Uid, cfg.Gid, func() error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var w sync.WaitGroup
		cmdEnv := mockEnv(cfg.DirData, strings.Join(cfg.DirDownload, ":"))
		w.Go(func() {
			cmd := exec.CommandContext(ctx, FILE_PAN_XUNLEI_CLI,
				"-launcher_listen", "unix:///var/packages/pan-xunlei-com/target/var/pan-xunlei-com-launcher.sock",
				"-pid", "/var/packages/pan-xunlei-com/target/var/pan-xunlei-com.pid",
			)
			if cfg.PreventUpdate {
				cmd.Args = append(cmd.Args, "-update_url", "null")
			}
			cmd.Dir = DIR_SYNOPKG_WORK
			cmd.Env = cmdEnv
			cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS, Setpgid: true}
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) }
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := cmd.Start(); err != nil {
				slog.ErrorContext(ctx, "start", "cmdline", strings.Join(cmd.Args, " "), "err", err)
				return
			}
			slog.InfoContext(ctx, "start", "cmdline", strings.Join(cmd.Args, " "))

			if err := cmd.Wait(); err != nil && err != context.Canceled {
				slog.ErrorContext(ctx, "cmd exited!", "err", err)
			} else {
				slog.InfoContext(ctx, "cmd exited!")
			}
		})

		w.Go(func() { mockWeb(ctx, cfg, cmdEnv, cancel) })
		w.Wait()
		return nil
	})
}

func mockWeb(ctx context.Context, cfg Config, env []string, onDone func()) (err error) {
	defer onDone()
	mux := chi.NewMux()
	mux.Use(middleware.Recoverer)

	const CGI_PATH = "/webman/3rdparty/pan-xunlei-com/index.cgi/"
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

	s := &http.Server{Addr: cmp.Or(cfg.Listen, ":2345"), Handler: mux, BaseContext: func(l net.Listener) context.Context {
		slog.InfoContext(ctx, "dashboard started", "listen", l.Addr().String())
		return ctx
	}}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
			if e := s.Shutdown(context.Background()); e != nil && e != http.ErrServerClosed {
				slog.Warn("shutdown web server", "err", e)
			}
		}
	}()

	if err = s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.WarnContext(ctx, "dashboard done", "err", err)
	} else {
		slog.InfoContext(ctx, "dashboard done")
	}
	return
}

func mockEnv(dirData, dirDownload string) []string {
	return sys.Environ().
		Del("container", "KUBERNETES_SERVICE_HOST", "KUBERNETES_PORT", "DOCKER_IMAGE", "DOCKER_TAG").
		Sets(
			"SYNOPLATFORM", SYNO_PLATFORM,
			"SYNOPKG_PKGNAME", SYNOPKG_PKGNAME,
			"SYNOPKG_PKGDEST", DIR_SYNOPKG_PKGDEST,
			"SYNOPKG_DSM_VERSION_MAJOR", SYNOPKG_DSM_VERSION_MAJOR,
			"SYNOPKG_DSM_VERSION_MINOR", SYNOPKG_DSM_VERSION_MINOR,
			"SYNOPKG_DSM_VERSION_BUILD", SYNOPKG_DSM_VERSION_BUILD,
			"DriveListen", "unix:///var/packages/pan-xunlei-com/target/var/pan-xunlei-com.sock",
			"PLATFORM", "群晖",
			"OS_VERSION", SYNO_VERSION,
			"ConfigPath", dirData,
			"HOME", filepath.Join(dirData, ".drive"),
			"DownloadPATH", dirDownload,
			"GIN_MODE", "release",
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

func downloadSpk(ctx context.Context, spkUrl string) sys.Runner {
	return func() (err error) {
		return spk.Download(ctx, spkUrl, DIR_SYNOPKG_PKGDEST, false)
	}
}

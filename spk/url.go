package spk

import (
	"cmp"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// 检查并下载, 如果 force，忽略检查直接下载
func Download(ctx context.Context, spkUrl string, dir string, force bool) (err error) {
	if !force && allExists(ctx, dir) {
		slog.InfoContext(ctx, "check spk all spk file exists")
		return
	}

	switch url := strings.ToLower(spkUrl); {
	case strings.HasPrefix(url, "file://"):
		err = download_file(ctx, spkUrl, dir)
	case strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://"):
		err = download_http(ctx, spkUrl, dir)
	default:
		err = fmt.Errorf("spk url is not support: %s", spkUrl)
	}
	return
}

func download_file(ctx context.Context, spkUrl string, dir string) (err error) {
	slog.InfoContext(ctx, "download spk", "url", spkUrl)
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "download spk fail", "url", spkUrl, "err", errcheck(err))
		} else {
			slog.InfoContext(ctx, "download spk done", "url", spkUrl)
		}
	}()

	f, e := os.Open(strings.TrimPrefix(spkUrl, "file://"))
	if err = e; err != nil {
		return
	}
	defer f.Close()

	err = Extract(ctx, f, dir)
	return
}

func download_http(ctx context.Context, spkUrl string, dir string) (err error) {
	slog.InfoContext(ctx, "download spk", "url", spkUrl)
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "download spk fail", "url", spkUrl, "err", errcheck(err))
		} else {
			slog.InfoContext(ctx, "download spk done", "url", spkUrl)
		}
	}()

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, spkUrl, nil); err != nil {
		return
	}
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-encoding", "gzip, deflate, br, zstd")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6,zh-TW;q=0.5")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("dnt", "1")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("priority", "u=0, i")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0")

	cli := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	var resp *http.Response
	if resp, err = cli.Do(req); err != nil {
		return
	}
	defer resp.Body.Close()

	total := resp.ContentLength
	current := int64(0)
	pPrint := func(b []byte) (n int, err error) {
		n = len(b)
		cur := atomic.AddInt64(&current, int64(n))
		fmt.Printf("\r%s %s/%s %.2f      ",
			filepath.Base(spkUrl),
			HumanBytes(cur),
			HumanBytes(total),
			float64(cur)*100/float64(total),
		)
		return
	}

	err = Extract(ctx, io.MultiReader(resp.Body, Reader(pPrint)), dir)
	return
}

func allExists(ctx context.Context, dir string) bool {
	files := []string{
		filepath.Join(dir, "bin/bin/version"),
		filepath.Join(dir, "bin/bin/xunlei-pan-cli-launcher.{arch}"),
		filepath.Join(dir, "bin/bin/xunlei-pan-cli.{version}.{arch}"),
		filepath.Join(dir, "ui/index.cgi"),
	}

	v, _ := os.ReadFile(files[0])
	version := strings.TrimSpace(string(v))
	if version == "" {
		slog.DebugContext(ctx, "check spk fail, version not found")
		return false
	}
	slog.DebugContext(ctx, "check spk", "version", version)

	repl := strings.NewReplacer("{arch}", runtime.GOARCH, "{version}", version)
	for _, f := range files[1:] {
		f = repl.Replace(f)
		stat, err := os.Stat(f)
		switch {
		case err != nil:
		case !stat.Mode().IsRegular():
			err = fmt.Errorf("is not regular: %s", stat.Mode().Type().String())
		case stat.Size() < 1024*1024*10:
			err = fmt.Errorf("file size too small: %s", HumanBytes(stat.Size()))
		default:
			slog.DebugContext(ctx, "check spk", "perm", stat.Mode().Perm().String(), "size", HumanBytes(stat.Size()), "modtime", stat.ModTime(), "file", f)
			continue
		}

		slog.DebugContext(ctx, "check spk fail", "file", f, "err", errcheck(err))
		return false
	}
	return true
}

func HumanBytes[T UintT | IntT](n T, prec ...int) string {
	if f := float64(n); f >= 1024 {
		for i, u := range slices.Backward([]rune("KMGTE")) {
			if base := float64(int64(1) << (10 * (i + 1))); float64(n) >= base {
				return fmt.Sprintf("%s %ciB", strconv.FormatFloat(f/base, 'f', cmp.Or(cmp.Or(prec...), 2), 64), u)
			}
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

func PasswordMask(s string) string {
	if len(s) <= 1 {
		return strings.Repeat("*", len(s))
	}
	return s[:1] + strings.Repeat("*", len(s[1:]))
}

type UintT interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type IntT interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Reader func([]byte) (int, error)

func (r Reader) Read(p []byte) (int, error) { return r(p) }

func errcheck(err error) error {
	if os.IsNotExist(err) {
		err = os.ErrNotExist
	}
	return err
}

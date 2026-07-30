// mocki — мгновенный mock REST API из JSON-файлов. Наследник json-server,
// но единый статический бинарь без зависимостей.
//
//	mocki serve <dir|file.json> [flags]  (флаги в любом порядке)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mocki/server"
	"mocki/store"
)

const usage = `mocki — mock REST API из JSON-файлов

  mocki serve <dir|file.json> [flags]

flags:
  -p, --port N     порт (по умолч. 3000)
      --latency D  искусственная задержка ответа (напр. 200ms)
      --cors       CORS заголовки (по умолч. true), --no-cors — выключить
      --watch      hot reload (по умолч. true), --no-watch — выключить
`

type options struct {
	path    string
	port    int
	latency time.Duration
	cors    bool
	watch   bool
}

// parseArgs — ручной разбор: флаги принимаются в любом порядке
// (flag-пакет Go останавливается на первом позиционном аргументе).
func parseArgs(args []string) (options, error) {
	opts := options{port: 3000, cors: true, watch: true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("флагу %s нужно значение", a)
			}
			i++
			return args[i], nil
		}
		switch {
		case a == "-p" || a == "--port":
			v, err := next()
			if err != nil {
				return opts, err
			}
			if _, err := fmt.Sscanf(v, "%d", &opts.port); err != nil {
				return opts, fmt.Errorf("неверный порт: %s", v)
			}
		case strings.HasPrefix(a, "--port="):
			if _, err := fmt.Sscanf(strings.TrimPrefix(a, "--port="), "%d", &opts.port); err != nil {
				return opts, err
			}
		case a == "--latency":
			v, err := next()
			if err != nil {
				return opts, err
			}
			if opts.latency, err = time.ParseDuration(v); err != nil {
				return opts, fmt.Errorf("неверная latency: %s", v)
			}
		case strings.HasPrefix(a, "--latency="):
			v := strings.TrimPrefix(a, "--latency=")
			var err error
			if opts.latency, err = time.ParseDuration(v); err != nil {
				return opts, err
			}
		case a == "--cors":
			opts.cors = true
		case a == "--no-cors":
			opts.cors = false
		case a == "--watch":
			opts.watch = true
		case a == "--no-watch":
			opts.watch = false
		case a == "serve":
			// подкоманда, пропускаем
		case strings.HasPrefix(a, "-"):
			return opts, fmt.Errorf("неизвестный флаг: %s", a)
		default:
			if opts.path != "" {
				return opts, fmt.Errorf("лишний аргумент: %s", a)
			}
			opts.path = a
		}
	}
	if opts.path == "" {
		return opts, errors.New("укажите директорию или .json файл")
	}
	return opts, nil
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprint(os.Stderr, usage)
		log.Error("args", "err", err)
		os.Exit(2)
	}

	st := store.New()
	if err := load(st, opts.path); err != nil {
		log.Error("load", "err", err)
		os.Exit(1)
	}
	log.Info("loaded", "resources", strings.Join(st.Resources(), ", "))

	if opts.watch {
		stop := st.Watch(500*time.Millisecond, func(n int) {
			log.Info("reloaded", "files", n, "resources", strings.Join(st.Resources(), ", "))
		})
		defer stop()
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.port),
		Handler:           server.New(st, server.Options{Latency: opts.latency, CORS: opts.cors, Logger: log}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("mocki listening", "addr", srv.Addr, "index", "http://localhost"+srv.Addr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// load загружает директорию или один файл.
func load(st *store.Store, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return st.LoadDir(path)
	}
	if !strings.HasSuffix(filepath.Base(path), ".json") {
		return fmt.Errorf("%s: ожидается .json файл или директория", path)
	}
	return st.LoadFile(path)
}

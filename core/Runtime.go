package core

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	hooks     []func()
	hookMutex sync.Mutex
)

func RegisterShutdownHook(fn func()) {
	hookMutex.Lock()
	hooks = append(hooks, fn)
	hookMutex.Unlock()
}

func StartHookWatch() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	)

	go func() {
		sig := <-ch
		LogDebug("[shutdown] received signal: %s", sig.String())

		hookMutex.Lock()
		for _, fn := range hooks {
			safeCall(fn)
		}
		hookMutex.Unlock()

		os.Exit(0)
	}()
}

func safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			LogDebug("[shutdown] recovered panic in cleanup: %v", r)
		}
	}()
	fn()
}

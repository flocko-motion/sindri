// package: hub/harness / launchask
// type:    logic (a human's launch request, held for the machine's launch)
// job:     carry what a human's start request asks of the launch — a bare shell, the debug
// trace, the preview size — and the stream watching it, from the route that records the request
// to the machine's launching state that carries it out. Start is that state's entry point.
// limits:  holding and handing over. Whether and when to launch is the machine's; the launch is
// lifecycle.go's.
package harness

import (
	"context"
	"io"
	"sync"
)

// LaunchOpts are how a human asked for a pod: a bare shell instead of Claude, the liveness probe's
// trace while it comes up, and the preview pane to size it to.
type LaunchOpts struct {
	Shell, Debug bool
	Cols, Lines  int
}

// LaunchAsk is one human's launch request, held until the machine's next launch of that agent takes
// it. The asker reads the launch's output as it runs and its result once it ends.
type LaunchAsk struct {
	opts LaunchOpts
	mu   sync.Mutex
	out  io.Writer // nil once the asker has gone
	ran  bool
	err  error
}

// Write passes launch output on while the asker is still there, and drops it after.
func (l *LaunchAsk) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.out == nil {
		return len(p), nil
	}
	return l.out.Write(p)
}

// Result reports whether a launch took this request, and how it ended.
func (l *LaunchAsk) Result() (ran bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ran, l.err
}

func (l *LaunchAsk) finish(err error) {
	l.mu.Lock()
	l.ran, l.err = true, err
	l.mu.Unlock()
}

// AskLaunch holds a launch request for this agent, replacing any earlier one. The caller Withdraws it
// when it stops watching: output after that goes nowhere, and a request nobody launched is dropped.
func (s *Service) AskLaunch(project, name string, opts LaunchOpts, out io.Writer) *LaunchAsk {
	l := &LaunchAsk{opts: opts, out: out}
	s.askMu.Lock()
	s.asks[lcKey{project, name}] = l
	s.askMu.Unlock()
	return l
}

// Withdraw ends a request's watch; the launch, if one took it, runs on.
func (s *Service) Withdraw(project, name string, l *LaunchAsk) {
	s.askMu.Lock()
	if s.asks[lcKey{project, name}] == l {
		delete(s.asks, lcKey{project, name})
	}
	s.askMu.Unlock()
	l.mu.Lock()
	l.out = nil
	l.mu.Unlock()
}

// Start is the machine's launch: it carries out the request a human is holding for this agent, or a
// plain launch when the machine started it on its own.
func (s *Service) Start(ctx context.Context, project, name string) error {
	s.askMu.Lock()
	l := s.asks[lcKey{project, name}]
	delete(s.asks, lcKey{project, name})
	s.askMu.Unlock()
	if l == nil {
		return s.Launch(ctx, project, name, false, false, 0, 0, io.Discard)
	}
	err := s.Launch(ctx, project, name, l.opts.Shell, l.opts.Debug, l.opts.Cols, l.opts.Lines, l)
	l.finish(err)
	return err
}

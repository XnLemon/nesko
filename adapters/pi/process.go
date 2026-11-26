package pi

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type rpcSession struct {
	command         *exec.Cmd
	stdin           io.WriteCloser
	stdoutLines     chan rpcLine
	readerStopped   chan struct{}
	stderr          io.ReadCloser
	stderrDone      chan struct{}
	stderrTail      *tailBuffer
	shutdownTimeout time.Duration
	processWaited   bool
}

func (e *Executor) start(ctx context.Context, workingDirectory string) (*rpcSession, error) {
	command := exec.CommandContext(ctx, e.binaryPath, e.args...)
	command.Dir = workingDirectory
	command.Env = mergeEnvironment(os.Environ(), e.environment)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}

	session := &rpcSession{
		command:         command,
		stdin:           stdin,
		stdoutLines:     make(chan rpcLine, 64),
		readerStopped:   make(chan struct{}),
		stderr:          stderr,
		stderrDone:      make(chan struct{}),
		stderrTail:      newTailBuffer(defaultStderrBytes),
		shutdownTimeout: e.shutdownTimeout,
	}
	go func() {
		_, _ = io.Copy(session.stderrTail, session.stderr)
		close(session.stderrDone)
	}()
	go readRPC(stdout, session.stdoutLines, session.readerStopped, e.maxRecordBytes)
	return session, nil
}

func (s *rpcSession) close() {
	close(s.readerStopped)
	_ = s.stdin.Close()
	if !s.processWaited {
		_ = s.command.Process.Kill()
		_ = s.command.Wait()
		s.processWaited = true
	}
	select {
	case <-s.stderrDone:
	case <-time.After(s.shutdownTimeout):
		_ = s.stderr.Close()
		<-s.stderrDone
	}
}

func (s *rpcSession) wait(ctx context.Context, timeout time.Duration) error {
	waitResult := make(chan error, 1)
	go func() { waitResult <- s.command.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-waitResult:
		s.processWaited = true
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		_ = s.command.Process.Kill()
		<-waitResult
		s.processWaited = true
		return ctx.Err()
	case <-timer.C:
		_ = s.command.Process.Kill()
		<-waitResult
		s.processWaited = true
		return fmt.Errorf("Pi process did not exit within %s", timeout)
	}
}

package pi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"goated/internal/agent"
	"goated/internal/msglog"
)

const postSendWaitTimeout = 6 * time.Minute

type SessionRuntime struct {
	workspaceDir string
	logDir       string
	redactor     *msglog.Redactor

	mu         sync.Mutex
	proc       *exec.Cmd
	procErr    error
	lastStderr string
	done       chan struct{}
}

func NewSessionRuntime(workspaceDir, logDir string) *SessionRuntime {
	credsDir := filepath.Join(workspaceDir, "creds")
	return &SessionRuntime{
		workspaceDir: workspaceDir,
		logDir:       logDir,
		redactor:     msglog.NewRedactor(credsDir),
	}
}

func (r *SessionRuntime) Descriptor() agent.RuntimeDescriptor {
	return agent.RuntimeDescriptor{
		Provider:    agent.RuntimePi,
		DisplayName: "Pi",
		SessionName: "",
		Capabilities: agent.Capabilities{
			SupportsInteractiveSession: false,
			SupportsContextEstimate:    false,
			SupportsCompaction:         false,
			SupportsReset:              true,
		},
	}
}

func (r *SessionRuntime) sessionDir() string {
	return filepath.Join(r.logDir, "pi_session")
}

func (r *SessionRuntime) runsDir() string {
	return filepath.Join(r.sessionDir(), "runs")
}

func (r *SessionRuntime) piSessionDir() string {
	return filepath.Join(r.sessionDir(), "sessions")
}

func (r *SessionRuntime) baseArgs() []string {
	return []string{
		"--mode", "json",
		"--session-dir", r.piSessionDir(),
	}
}

func (r *SessionRuntime) hasSavedSession() bool {
	entries, err := os.ReadDir(r.piSessionDir())
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func (r *SessionRuntime) EnsureSession(ctx context.Context) error {
	for _, dir := range []string{r.sessionDir(), r.runsDir(), r.piSessionDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func (r *SessionRuntime) SendUserPrompt(ctx context.Context, channel, chatID, userPrompt string, attachments *agent.MessageAttachments, messageID, threadID string) error {
	envelope := agent.BuildPromptEnvelope(channel, chatID, userPrompt, attachments, messageID, threadID)
	return r.sendEnvelope(ctx, envelope)
}

func (r *SessionRuntime) SendBatchPrompt(ctx context.Context, channel, chatID string, messages []agent.PromptMessage) error {
	envelope := agent.BuildBatchEnvelope(channel, chatID, messages)
	return r.sendEnvelope(ctx, envelope)
}

func (r *SessionRuntime) SendSystemNotice(ctx context.Context, channel, chatID, source, message string, metadata map[string]string) error {
	envelope := agent.BuildSystemNoticeEnvelope(channel, chatID, source, message, metadata)
	return r.sendEnvelope(ctx, envelope)
}

func (r *SessionRuntime) sendEnvelope(ctx context.Context, envelope string) error {
	if err := r.EnsureSession(ctx); err != nil {
		return err
	}

	args := []string{"-p", envelope}
	if r.hasSavedSession() {
		args = append(args, "-c")
	}
	args = append(args, r.baseArgs()...)

	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Dir = r.workspaceDir
	if reqID := msglog.RequestIDFromContext(ctx); reqID != "" {
		cmd.Env = append(filterEnv(os.Environ(), "GOAT_REQUEST_ID"), "GOAT_REQUEST_ID="+reqID)
	}

	runFile := filepath.Join(r.runsDir(), fmt.Sprintf("%d.jsonl", time.Now().UnixNano()))
	outFile, err := os.Create(runFile)
	if err != nil {
		return fmt.Errorf("create run log: %w", err)
	}

	var stdoutBuf strings.Builder
	cmd.Stdout = &teeWriter{file: outFile, buf: &stdoutBuf, redactor: r.redactor}

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrWriter{file: outFile, buf: &stderrBuf, redactor: r.redactor}

	r.mu.Lock()
	for r.proc != nil {
		done := r.done
		r.mu.Unlock()

		select {
		case <-done:
		case <-time.After(postSendWaitTimeout):
			outFile.Close()
			return fmt.Errorf("timed out waiting for previous pi process to finish")
		case <-ctx.Done():
			outFile.Close()
			return ctx.Err()
		}

		r.mu.Lock()
	}

	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		outFile.Close()
		return fmt.Errorf("start pi: %w", err)
	}

	r.proc = cmd
	r.procErr = nil
	r.lastStderr = ""
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()

	go func() {
		runErr := cmd.Wait()
		outFile.Close()

		r.mu.Lock()
		r.proc = nil
		r.procErr = runErr
		r.lastStderr = stderrBuf.String()
		r.mu.Unlock()

		close(done)
	}()

	return nil
}

func (r *SessionRuntime) WaitForAwaitingInput(ctx context.Context, timeout time.Duration) (agent.SessionState, error) {
	r.mu.Lock()
	done := r.done
	proc := r.proc
	r.mu.Unlock()

	if proc == nil || done == nil {
		return agent.SessionState{Kind: agent.SessionStateAwaitingInput, Summary: "idle (no process)"}, nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		r.mu.Lock()
		lastErr := r.procErr
		r.mu.Unlock()
		if lastErr != nil {
			return agent.SessionState{Kind: agent.SessionStateAwaitingInput, Summary: fmt.Sprintf("process exited with error: %v", lastErr)}, nil
		}
		return agent.SessionState{Kind: agent.SessionStateAwaitingInput, Summary: "process completed"}, nil
	case <-timer.C:
		return agent.SessionState{Kind: agent.SessionStateGenerating, Summary: "timed out waiting for process"}, fmt.Errorf("timed out waiting for pi process to finish")
	case <-ctx.Done():
		return agent.SessionState{}, ctx.Err()
	}
}

func (r *SessionRuntime) GetSessionState(ctx context.Context) (agent.SessionState, error) {
	r.mu.Lock()
	proc := r.proc
	done := r.done
	lastErr := r.procErr
	lastStderr := r.lastStderr
	r.mu.Unlock()

	if proc != nil && done != nil {
		select {
		case <-done:
		default:
			return agent.SessionState{Kind: agent.SessionStateGenerating, Summary: "processing"}, nil
		}
	}

	for _, pat := range []string{"authentication", "login", "api key", "unauthorized"} {
		if strings.Contains(strings.ToLower(lastStderr), pat) {
			return agent.SessionState{Kind: agent.SessionStateBlockedIntervene, Summary: "Pi requires manual login or API key setup"}, nil
		}
	}

	if lastErr != nil {
		return agent.SessionState{Kind: agent.SessionStateAwaitingInput, Summary: fmt.Sprintf("last process error: %v", lastErr)}, nil
	}

	return agent.SessionState{Kind: agent.SessionStateAwaitingInput, Summary: "idle"}, nil
}

func (r *SessionRuntime) StopSession(ctx context.Context) error {
	r.mu.Lock()
	proc := r.proc
	done := r.done
	r.mu.Unlock()

	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
		if done != nil {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
		}
	}
	return nil
}

func (r *SessionRuntime) RestartSession(ctx context.Context) error {
	if err := r.StopSession(ctx); err != nil {
		return err
	}
	return r.EnsureSession(ctx)
}

func (r *SessionRuntime) ResetConversation(ctx context.Context, chatID string) (agent.ResetResult, error) {
	if err := os.RemoveAll(r.piSessionDir()); err != nil && !os.IsNotExist(err) {
		return agent.ResetResult{}, err
	}
	if err := os.MkdirAll(r.piSessionDir(), 0o755); err != nil {
		return agent.ResetResult{}, err
	}
	return agent.ResetResult{
		Scope:   agent.ResetScopeHard,
		Summary: "Pi session cleared. The next message starts a fresh session.",
	}, nil
}

func (r *SessionRuntime) SendControlCommand(ctx context.Context, text string) error {
	return nil
}

func (r *SessionRuntime) GetContextEstimate(ctx context.Context, chatID string) (agent.ContextEstimate, error) {
	return agent.ContextEstimate{
		State:      agent.ContextEstimateUnknown,
		RawSummary: "pi manages context automatically",
	}, nil
}

func (r *SessionRuntime) GetHealth(ctx context.Context) (agent.HealthStatus, error) {
	if _, err := exec.LookPath("pi"); err != nil {
		return agent.HealthStatus{OK: false, Recoverable: false, Summary: "pi binary not found on PATH"}, nil
	}

	r.mu.Lock()
	lastStderr := r.lastStderr
	r.mu.Unlock()

	for _, pat := range []string{"authentication", "login", "api key", "unauthorized"} {
		if strings.Contains(strings.ToLower(lastStderr), pat) {
			return agent.HealthStatus{OK: false, Recoverable: false, Summary: fmt.Sprintf("auth error: %s", pat)}, nil
		}
	}

	return agent.HealthStatus{OK: true, Recoverable: true, Summary: "ok"}, nil
}

func (r *SessionRuntime) DetectRetryableError(ctx context.Context) string {
	r.mu.Lock()
	lastStderr := strings.ToLower(r.lastStderr)
	r.mu.Unlock()

	for _, pat := range []string{"server_error", "rate_limit", "overloaded", "status 500", "status 502", "status 503"} {
		if strings.Contains(lastStderr, pat) {
			return pat
		}
	}
	return ""
}

func (r *SessionRuntime) Version(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "pi", "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func filterEnv(env []string, remove string) []string {
	prefix := remove + "="
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

type teeWriter struct {
	file     *os.File
	buf      *strings.Builder
	redactor *msglog.Redactor
}

func (w *teeWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.redactor != nil {
		redacted := w.redactor.Redact(string(p))
		_, err := w.file.WriteString(redacted)
		if err != nil {
			return 0, err
		}
	} else {
		if _, err := w.file.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

type stderrWriter struct {
	file     *os.File
	buf      *strings.Builder
	redactor *msglog.Redactor
}

func (w *stderrWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.redactor != nil {
		redacted := w.redactor.Redact(string(p))
		_, err := w.file.WriteString(redacted)
		if err != nil {
			return 0, err
		}
	} else {
		if _, err := w.file.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

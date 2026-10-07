package definitions

import (
	"os/exec"
	"syscall"
	"time"
)

type RunningModel interface {
	ModelName() string
	ProviderName() string
	RuntimeVersion() string
	Uptime() time.Duration
	Logs() []string // recent captured log lines, oldest first

	Wait()        // wait for this to end
	SendSigQuit() // please quit
	SendSigKill() // too slow, die!
}

func RunDetailsFromCmd(cmd *exec.Cmd, modelName string, providerName string, runtimeVersion string, logs *LogBuffer) RunningModel {
	return &cmdRunningModel{
		cmd:            cmd,
		modelName:      modelName,
		providerName:   providerName,
		runtimeVersion: runtimeVersion,
		startedAt:      time.Now(),
		logs:           logs,
	}
}

type cmdRunningModel struct {
	cmd            *exec.Cmd
	modelName      string
	providerName   string
	runtimeVersion string
	startedAt      time.Time
	logs           *LogBuffer
}

// Uptime implements [RunningModel].
func (this *cmdRunningModel) Uptime() time.Duration {
	return time.Since(this.startedAt)
}

// Logs implements [RunningModel]. It returns the recent log lines captured
// from the process's output, oldest first. It is safe to call after the
// process has exited, in which case it returns the last captured lines.
func (this *cmdRunningModel) Logs() []string {
	if this.logs == nil {
		return nil
	}
	return this.logs.Lines()
}

// Wait implements [RunningModel].
func (this *cmdRunningModel) Wait() {
	this.cmd.Wait()
}

// ModelName implements [RunningModel].
func (this *cmdRunningModel) ModelName() string {
	return this.modelName
}

// ProviderName implements [RunningModel].
func (this *cmdRunningModel) ProviderName() string {
	return this.providerName
}

// RuntimeVersion implements [RunningModel].
func (this *cmdRunningModel) RuntimeVersion() string {
	return this.runtimeVersion
}

// SendSigKill implements [RunningModel].
func (c *cmdRunningModel) SendSigKill() {
	c.cmd.Process.Signal(syscall.SIGKILL)
}

// SendSigQuit implements [RunningModel].
func (c *cmdRunningModel) SendSigQuit() {
	c.cmd.Process.Signal(syscall.SIGQUIT)

}

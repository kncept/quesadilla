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

	Wait()        // wait for this to end
	SendSigQuit() // please quit
	SendSigKill() // too slow, die!
}

func RunDetailsFromCmd(cmd *exec.Cmd, modelName string, providerName string, runtimeVersion string) RunningModel {
	return &cmdRunningModel{
		cmd:            cmd,
		modelName:      modelName,
		providerName:   providerName,
		runtimeVersion: runtimeVersion,
		startedAt:      time.Now(),
	}
}

type cmdRunningModel struct {
	cmd            *exec.Cmd
	modelName      string
	providerName   string
	runtimeVersion string
	startedAt      time.Time
}

// Uptime implements [RunningModel].
func (this *cmdRunningModel) Uptime() time.Duration {
	return time.Since(this.startedAt)
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

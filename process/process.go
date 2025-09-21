package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

const (
	maxLogLines = 1000 // maximum number of log lines to keep
)

type Command struct {
	path    string // path to execute the command
	command string // command to execute
}

type Process struct {
	*Command
	pid         int
	port        int32 // TODO: implement
	status      ProcStatus
	memoryUsage uint64
	cpuUsage    float64
	//log         []string
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.RWMutex
}

func (p *Process) Pid() int              { return p.pid }
func (p *Process) Port() int32           { return p.port }
func (p *Process) MemoryUsage() uint64   { return p.memoryUsage }
func (p *Process) CPUUsage() float64     { return p.cpuUsage }
func (p *Process) Status() ProcStatus    { return p.status }
func (p *Process) Stdout() io.ReadCloser { return p.stdout }
func (p *Process) Stderr() io.ReadCloser { return p.stderr }

func NewProcess(path, command string) *Process {
	return &Process{
		Command: &Command{
			path:    path,
			command: command,
		},
	}
}

func (p *Process) Run() (err error) {
	defer func() {
		p.mu.Lock()
		if err != nil {
			p.status = ProcStatusError
		} else {
			p.status = ProcStatusRunning
		}
		p.mu.Unlock()
	}()

	var originalDir string
	originalDir, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err = os.Chdir(p.path); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", p.path, err)
	}

	commandSplit := strings.Split(p.command, " ")
	if len(commandSplit) == 0 {
		return errors.New("command is empty")
	}

	var cmd *exec.Cmd
	if len(commandSplit) == 1 {
		cmd = exec.Command(commandSplit[0])
	} else {
		cmd = exec.Command(commandSplit[0], commandSplit[1:]...)
	}

	// Create pipes for stdout and stderr
	p.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	p.stderr, err = cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// TODO: research why we need to set pgid
	// for killing the whole process group and its children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command %s: %w", p.command, err)
	}

	p.mu.Lock()
	p.pid = cmd.Process.Pid
	p.mu.Unlock()

	return nil
}

/*
func (p *Process) storeLogs() {
	go p.scanLog(p.stdout)
	go p.scanLog(p.stderr)
}

func (p *Process) scanLog(reader io.ReadCloser) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		scanned := scanner.Text()
		p.mu.RLock()
		if len(p.log) >= maxLogLines {
			p.log = append(p.log[len(p.log)-maxLogLines:], scanned+"\n")
		} else {
			p.log = append(p.log, scanned+"\n")
		}
		p.mu.RUnlock()
	}
}
*/

func (p *Process) UpdateStats(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	proc, err := process.NewProcess(int32(p.pid))
	if err != nil {
		return fmt.Errorf("failed to get process %d: %w", p.pid, err)
	}

	if p.status == ProcStatusRunning && p.pid > 0 {
		stat, _ := proc.MemoryInfoWithContext(ctx)
		// TODO: RSSって何？VMSとどう違う？
		// TODO: CPU利用率や飽和率を調べるのに必要な情報を出せるようにしたい
		p.memoryUsage = stat.RSS // in bytes
		if cpu, err := proc.CPUPercentWithContext(ctx); err == nil {
			p.cpuUsage = cpu // in percentage
		}
	}

	return nil
}

func (p *Process) Terminate(ctx context.Context) error {
	// TODO: process groupって何？
	// negative pid means kill the whole process group and its children
	err := syscall.Kill(-p.pid, syscall.SIGINT)
	if err != nil {
		p.mu.Lock()
		p.status = ProcStatusError
		p.mu.Unlock()
		return fmt.Errorf("failed to terminate process %d: %w", p.pid, err)
	}

	p.mu.Lock()
	p.status = ProcStatusStopped
	p.pid = 0
	p.memoryUsage = 0
	p.cpuUsage = 0
	p.mu.Unlock()

	return nil
}

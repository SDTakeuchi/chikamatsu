package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

func main() {
	p := &Process{
		Command: &Command{
			path:    "/Users/takeuchi/work/card/card-proc-mng/prototype/foo",
			command: "go run main.go webserver.go",
		},
		status: statusRunning,
	}
	processes := []*Process{p}
	for _, proc := range processes {
		if err := proc.Run(); err != nil {
			log.Fatal(err)
		}
	}
	time.Sleep(1 * time.Second)

	ctx := context.Background()

	for _, proc := range processes {
		if err := proc.UpdateStats(ctx); err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("process:%+v", processes[0])

	log.Printf("logs: %s", processes[0].logs)
	for _, proc := range processes {
		if err := proc.Terminate(ctx); err != nil {
			log.Fatal(err)
		}
	}
}

const (
	terminateTimeout = 10 * time.Second // after this timeout, the process will be killed
	maxLogLines      = 1000             // maximum number of log lines to keep
)

type status string

func (s status) String() string {
	return string(s)
}

const (
	statusStopped status = "stopped"
	statusRunning status = "running"
	statusError   status = "error"
)

type Command struct {
	path    string // path to execute the command
	command string // command to execute
}

type Process struct {
	*Command
	terminate   func() error // function to terminate the process
	pid         int
	port        int32
	status      status
	memoryUsage uint64
	cpuUsage    float64
	logs        []string
	mu          sync.RWMutex
}

func (p *Process) Run() (err error) {
	defer func() {
		p.mu.Lock()
		if err != nil {
			p.status = statusError
		} else {
			p.status = statusRunning
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
	var stdout, stderr io.ReadCloser
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err = cmd.StderrPipe()
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

	// Start goroutines to read stdout and stderr
	go p.readOutput(stdout, "stdout")
	go p.readOutput(stderr, "stderr")

	log.Printf("sleeping for 10 seconds")
	time.Sleep(10 * time.Second)

	return nil
}

// readOutput reads from the given reader and adds lines to Process.logs
func (p *Process) readOutput(reader io.Reader, stream string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		logEntry := fmt.Sprintf("%s: %s", stream, line)

		p.mu.Lock()
		p.logs = append(p.logs, logEntry)
		// Keep only the last maxLogLines entries
		if len(p.logs) > maxLogLines {
			p.logs = p.logs[len(p.logs)-maxLogLines:]
		}
		p.mu.Unlock()
	}
}

func (p *Process) UpdateStats(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	proc, err := process.NewProcess(int32(p.pid))
	if err != nil {
		return fmt.Errorf("failed to get process %d: %w", p.pid, err)
	}

	if p.status == statusRunning && p.pid > 0 {
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
		p.status = statusError
		p.mu.Unlock()
		return fmt.Errorf("failed to terminate process %d: %w", p.pid, err)
	}

	p.mu.Lock()
	p.status = statusStopped
	p.mu.Unlock()

	return nil
}

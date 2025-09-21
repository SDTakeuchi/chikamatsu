package process

type ProcStatus string

func (s ProcStatus) String() string {
	return string(s)
}

const (
	ProcStatusStopped ProcStatus = "stopped"
	ProcStatusRunning ProcStatus = "running"
	ProcStatusError   ProcStatus = "error"
)

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/SDTakeuchi/chikamatsu/process"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const refreshInterval = 500 * time.Millisecond

var (
	focusedView tview.Primitive // focused view

	processView = tview.NewTable()
	logView     = tview.NewTextView()
)

func init() {
	processView.SetBorder(true).SetTitle("Processes")
	logView.SetText("chikamatsu started...\n").SetBorder(true).SetTitle("Log")
	focusedView = processView
}

type Process interface {
	// Getters

	Pid() int
	Port() int32
	MemoryUsage() uint64 // in bytes
	CPUUsage() float64   // in percentage
	Status() process.ProcStatus
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser

	// Methods

	Run() error                            // start the process
	UpdateStats(ctx context.Context) error // update the stats of the process
	Terminate(ctx context.Context) error   // terminate the process
}

type App struct {
	view *tview.Application
	root tview.Primitive
}

func newApp() *App {
	return &App{view: tview.NewApplication()}
}

func (app *App) updateLog(textView *tview.TextView, procs []Process) {
	for _, p := range procs {
		go app.update(textView, p.Stdout(), p.Pid())
		go app.update(textView, p.Stderr(), p.Pid())
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		textView.ScrollToEnd()
	}
}

func (app *App) update(textView *tview.TextView, reader io.ReadCloser, pid int) {
	if reader == nil {
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		textView.Write([]byte(fmt.Sprintf("[%d] %s\n", pid, scanner.Text())))
	}
}

func (app *App) tableView(table *tview.Table, processes []Process) {
	// sort processes by PID to fix the order of the processes
	slices.SortFunc(processes, func(i, j Process) int { return int(i.Pid() - j.Pid()) })

	table.
		Select(1, 0).
		SetSelectable(true, false).
		/*
			SetSelectedFunc(func(row, col int) {
				if row == 0 || focusedProcess.Pid() == processes[row-1].Pid() {
					// do nothing when row=0 where header is located
					// or when the selected process is the same as the focused process
					return
				}
				logView.Clear()
				focusedProcess = processes[row-1]
			}).
		*/
		SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				row, _ := table.GetSelection()
				if row == 0 {
					// do nothing for header
					return event
				}

				proc := processes[row-1]

				switch event.Key() {
				case tcell.KeyCtrlT:
					// terminate selected process
					proc.Terminate(context.Background())
				case tcell.KeyCtrlR:
					// restart selected process
					if proc.Status() == process.ProcStatusRunning {
						err := proc.Terminate(context.Background())
						if err != nil {
							// do nothing
							return event
						}
					}
					proc.Run()
					go app.update(logView, proc.Stdout(), proc.Pid())
					go app.update(logView, proc.Stderr(), proc.Pid())
				}
				return event
			},
		)

	// header
	table.SetCell(0, 0, tview.NewTableCell("PID"))
	table.SetCell(0, 1, tview.NewTableCell("Status"))
	table.SetCell(0, 2, tview.NewTableCell("CPU%"))
	table.SetCell(0, 3, tview.NewTableCell("Memory(MB)"))

	ticker := time.NewTicker(refreshInterval)
	for range ticker.C {
		app.view.QueueUpdateDraw(func() {
			for i, proc := range processes {
				proc.UpdateStats(context.Background())
				i++ // increment to skip header

				table.SetCell(i, 0, tview.NewTableCell(strconv.Itoa(int(proc.Pid()))))
				table.SetCell(i, 1, tview.NewTableCell(proc.Status().String()))
				table.SetCell(i, 2, tview.NewTableCell(fmt.Sprintf("%.2f", proc.CPUUsage())))
				table.SetCell(i, 3, tview.NewTableCell(strconv.Itoa(int(proc.MemoryUsage())/1024/1024)))
			}
		})
	}
}

func main() {
	app := newApp()

	processes := []Process{
		process.NewProcess("foo", "go run main.go webserver.go"),
		process.NewProcess("test", "go run main.go"),
	}
	for _, proc := range processes {
		if err := proc.Run(); err != nil {
			log.Fatal(err)
		}
	}

	go app.tableView(processView, processes)
	go app.updateLog(logView, processes)

	flexView := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(processView, 0, 1, true).
		AddItem(logView, 0, 2, false)

	app.root = flexView

	app.view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			if focusedView == processView {
				focusedView = logView
			} else {
				focusedView = processView
			}
			app.view.SetFocus(focusedView)
		case tcell.KeyCtrlC:
			for _, proc := range processes {
				proc.Terminate(context.Background())
			}
			app.view.Stop()
			return nil
		}
		return event
	})

	err := app.view.SetRoot(app.root, true).Run()
	if err != nil {
		log.Fatal()
	}
}

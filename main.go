package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/SDTakeuchi/chikamatsu/process"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const refreshInterval = 500 * time.Millisecond

var (
	loggingProcess Process         // process to log the output
	focusedView    tview.Primitive // focused view
)

type Process interface {
	// Getters
	Pid() int
	Port() int32
	MemoryUsage() uint64 // in bytes
	CPUUsage() float64   // in percentage
	Status() process.ProcStatus
	Logs() []string

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

func currentTimeString() string {
	return time.Now().Format("Current time is 15:04:05")
}

func (app *App) updateTime(textView *tview.TextView) {
	logs := []string{}
	for {
		if loggingProcess != nil && loggingProcess.Logs() != nil {
			if slices.Equal(logs, loggingProcess.Logs()) {
				time.Sleep(refreshInterval)
				continue
			}

			app.view.QueueUpdateDraw(func() {
				textView.
					SetText(strings.Join(loggingProcess.Logs(), "\n"))
			})
		} else {
			time.Sleep(refreshInterval)
		}
	}
}

func (app *App) tableView(table *tview.Table, processes []Process) {
	// sort processes by PID to fix the order of the processes
	slices.SortFunc(processes, func(i, j Process) int { return int(i.Pid() - j.Pid()) })

	table.
		Select(0, 0).
		SetSelectable(true, false).
		SetSelectedFunc(func(row, col int) {
			if row == 0 {
				// do nothing for header
				return
			}
			loggingProcess = processes[row-1]
		}).
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

	table := tview.NewTable()
	table.SetBorder(true).SetTitle("Processes")
	go app.tableView(table, processes)

	textView := tview.NewTextView()
	textView.SetText(currentTimeString())
	textView.SetBorder(true).SetTitle("Log")
	go app.updateTime(textView)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, true).
		AddItem(textView, 0, 2, false)

	app.root = flex

	app.view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			if focusedView == table {
				focusedView = textView
			} else {
				focusedView = table
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

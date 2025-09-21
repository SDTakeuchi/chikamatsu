package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/SDTakeuchi/chikamatsu/process"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const refreshInterval = 500 * time.Millisecond

type Process interface {
	Pid() int
	MemoryUsage() uint64 // in bytes
	CPUUsage() float64   // in percentage
	Status() process.ProcStatus
	Logs() []string
	Run() error
	UpdateStats(ctx context.Context) error
	Terminate(ctx context.Context) error
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
	ticker := time.NewTicker(refreshInterval)
	for range ticker.C {
		app.view.QueueUpdateDraw(func() {
			textView.SetText(currentTimeString())
		})
	}
}

func (app *App) tableView(table *tview.Table, processes []Process) {
	slices.SortFunc(processes, func(i, j Process) int { return int(i.Pid() - j.Pid()) })

	table.
		Select(0, 0).
		SetDoneFunc(func(key tcell.Key) {
			switch key {
			case tcell.KeyCtrlC:
				app.view.Stop()
			case tcell.KeyEnter:
				table.SetSelectable(true, false)
			}
		}).
		SetSelectedFunc(func(row, col int) {
			table.GetCell(row, col).SetTextColor(tcell.ColorLime)
			table.SetSelectable(false, false)
		})

	// header
	table.SetCell(0, 0, tview.NewTableCell("PID"))
	table.SetCell(0, 1, tview.NewTableCell("Status"))
	table.SetCell(0, 2, tview.NewTableCell("CPU%"))
	table.SetCell(0, 3, tview.NewTableCell("Memory(MB)"))

	ticker := time.NewTicker(refreshInterval)
	for range ticker.C {
		for _, proc := range processes {
			proc.UpdateStats(context.Background())
		}

		app.view.QueueUpdateDraw(func() {
			for i, proc := range processes {
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

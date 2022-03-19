package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/alexflint/go-arg"
	"github.com/imarsman/goparallel/cmd/command"
	"github.com/imarsman/goparallel/cmd/parse"
	"github.com/imarsman/goparallel/cmd/tasks"
)

var slots int

func init() {
	slots = 8
}

// args CLI args

// readLines reads a whole file into memory
// and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// callArgs command line arguments
var callArgs struct {
	Command   string   `arg:"positional"`
	Arguments []string `arg:"-a,--arguments,separate" help:"lists of arguments"`
	DryRun    bool     `arg:"-d,--dry-run" help:"show command to run but don't run"`
	Slots     int64    `arg:"-s,--slots" default:"8" help:"number of parallel tasks"`
	Shuffle   bool     `arg:"-S,--shuffle" help:"shuffle tasks prior to running"`
	Ordered   bool     `arg:"-o,--ordered" help:"run tasks in their incoming order"`
	KeepOrder bool     `arg:"-k,--keep-order" help:"don't keep output for calls separate"`
}

func main() {
	arg.MustParse(&callArgs)

	if callArgs.Slots == 0 {
		callArgs.Slots = int64(runtime.NumCPU())
	}

	taskListSet := tasks.NewTaskListSet()

	// Make config to hold various parameters
	config := command.Config{
		Slots:       callArgs.Slots,
		DryRun:      callArgs.DryRun,
		Ordered:     callArgs.Ordered,
		KeepOrder:   callArgs.KeepOrder,
		Concurrency: callArgs.Slots,
	}

	// Define command to run
	var c = command.NewCommand(
		callArgs.Command,
		&taskListSet,
		config,
	)

	c.SetConcurrency(callArgs.Slots)
	var wg sync.WaitGroup

	// Use stdin if it is available
	// It will be the first task list if it is available
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		stdinItems := []string{}
		var scanner = bufio.NewScanner(os.Stdin)

		// Tell scanner to scan by lines.
		scanner.Split(bufio.ScanLines)

		var item string
		for scanner.Scan() {
			item = scanner.Text()
			item = strings.TrimSpace(item)
			// fmt.Println(item)
			if len(callArgs.Arguments) == 0 && len(item) > 0 {
				var task = tasks.NewTask(item)
				var taskSet []tasks.Task
				taskSet = append(taskSet, *task)
				c2 := c.Copy()
				err := command.RunCommand(c2, taskSet, wg)
				if err != nil {
					fmt.Println("got error", err)
					os.Exit(1)
				}
				c.SequenceIncr()

			} else {
				stdinItems = append(stdinItems, item)
			}
		}
		if len(stdinItems) > 0 {
			taskList := tasks.NewTaskList()
			taskList.Add(stdinItems...)
			taskListSet.AddTaskList(taskList)
		}
	}

	if len(callArgs.Arguments) == 0 {
		// Wait for all goroutines to complete
		wg.Wait()

		os.Exit(0)
	}

	if len(callArgs.Arguments) > 0 {
		// Add list verbatim
		if len(callArgs.Arguments) > 0 {
			for _, v := range callArgs.Arguments {
				taskList := tasks.NewTaskList()
				parts := strings.Split(v, " ")

				for _, part := range parts {
					part = strings.TrimSpace(part)
					if parse.RERange.MatchString(part) {
						items, err := parse.Range(part)
						if err != nil {
							fmt.Println(err)
							return
						}
						taskList.Add(items...)
					} else {
						matches, err := filepath.Glob(part)
						if err != nil {
							continue
						}
						if len(matches) == 0 {
							taskList.Add(strings.TrimSpace(part))
						} else {
							var files []string
							for _, f := range matches {
								f, _ := os.Stat(f)
								if !f.IsDir() {
									files = append(files, f.Name())
								}
							}
							taskList.Add(files...)
						}
					}
					if callArgs.Shuffle {
						taskList.Shuffle()
					}
				}
				taskListSet.AddTaskList(taskList)
			}
		}
	}

	for i := 0; i < taskListSet.Max(); i++ {
		tasks, err := taskListSet.NextAll()

		c2 := c.Copy()
		err = command.RunCommand(c2, tasks, wg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		c.SequenceIncr()
	}

	// Wait for all goroutines to complete
	wg.Wait()
}

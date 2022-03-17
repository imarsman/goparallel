package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
	Files     []string `arg:"-f,--files,separate" help:"files to read into lines"`
	DryRun    bool     `arg:"-d,--dry-run" help:"show command to run but don't run"`
	Slots     int      `arg:"-s,--slots" help:"number of parallel tasks"`
	Shuffle   bool     `arg:"-S,--shuffle" help:"shuffle tasks prior to running"`
	Ordered   bool     `arg:"-o,--ordered" help:"run tasks in their incoming order"`
	KeepOrder bool     `arg:"-k,--keep-order" help:"don't keep output for calls separate"`
}

func main() {
	// var args = os.Args

	// call := args[1:]
	// fmt.Println(call)
	// re := regexp.MustCompile(`([\:]{3,4})`)
	// parts := re.Split(strings.Join(call, " "), -1)
	// // fmt.Println("parts", parts, len(parts))

	// var command string
	// var keepOrder bool
	// var dryRun bool
	// var sequences []string

	// for i, p := range parts {
	// 	if i == 0 {
	// 		command = p
	// 		continue
	// 	}
	// 	if strings.Contains(p, "-k") {
	// 		keepOrder = true
	// 		p = strings.ReplaceAll(p, "-k", "")
	// 	}
	// 	if strings.Contains(p, "-d") {
	// 		dryRun = true
	// 		p = strings.ReplaceAll(p, "-d", "")
	// 	}

	// 	sequences = append(sequences, p)
	// 	fmt.Println(p)
	// }

	// fmt.Println(command, sequences, keepOrder, dryRun)
	arg.MustParse(&callArgs)

	if callArgs.Slots == 0 {
		callArgs.Slots = runtime.NumCPU()
	}

	taskListSet := tasks.NewTaskListSet()

	// Use stdin if it is available
	// It will be the first task list if it is available
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		stdinItems := []string{}
		var scanner = bufio.NewScanner(os.Stdin)
		// Tell scanner to scan by lines.
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			item := scanner.Text()
			item = strings.TrimSpace(item)
			stdinItems = append(stdinItems, item)
		}
		if len(stdinItems) > 0 {
			taskList := tasks.NewTaskList()
			taskList.Add(stdinItems...)
			taskListSet.AddTaskList(taskList)
		}
	}

	// Add list verbatim
	if len(callArgs.Arguments) > 0 {
		// c:
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
		// for _, t := range taskListSet.TaskLists {
		// 	fmt.Println("task list", t)
		// }
	}

	// Add all lines for all files
	if len(callArgs.Files) > 0 {
		// Read all lines from files
		for _, v := range callArgs.Files {
			taskList := tasks.NewTaskList()
			parts := strings.Split(v, " ")
			for _, part := range parts {
				part = strings.TrimSpace(part)

				matches, err := filepath.Glob(part)
				if err != nil {
					return
				}
				if callArgs.Shuffle {
					taskList.Shuffle()
				}
				for _, file := range matches {
					var lines []string
					lines, err = readLines(file)
					taskList.Add(lines...)
				}
			}
			taskListSet.AddTaskList(taskList)
		}
	}

	// Make config to hold various parameters
	config := command.Config{
		Slots:     callArgs.Slots,
		DryRun:    callArgs.DryRun,
		Ordered:   callArgs.Ordered,
		KeepOrder: callArgs.KeepOrder,
	}

	// Define command to run
	var c = command.NewCommand(
		// strings.Join(callArgs.Command, " "),
		callArgs.Command,
		&taskListSet,
		config,
	)
	// Run command for all items
	err := command.RunCommand(c)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

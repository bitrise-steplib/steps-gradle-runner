package gradle

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

// Task ...
type Task struct {
	name    string
	project Project
	logger  log.Logger
}

// NewTask creates a new task
func NewTask(name string, project Project, logger log.Logger) *Task {
	return &Task{
		name:    name,
		project: project,
		logger:  logger,
	}
}

// GetVariants ...
func (task *Task) GetVariants(customArgs ...string) (Variants, error) {
	args := slices.Clone(customArgs)
	args = slices.DeleteFunc(args, func(arg string) bool {
		return arg == "--debug" || arg == "-d" || arg == "--info" || arg == "-i" || arg == "--quiet" || arg == "-q" || arg == "--warn" || arg == "-w"
	})
	opts := command.Opts{Dir: task.project.location}
	args = append([]string{"tasks", "--all", "--console=plain", "--quiet", "--no-build-cache", "--no-daemon"}, args...)
	cmd := task.project.cmdFactory.Create(filepath.Join(task.project.location, "gradlew"), args, &opts)
	task.logger.Debugf("$ %s", cmd.PrintableCommandArgs())
	tasksOutput, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s, %s", tasksOutput, err)
	}
	task.logger.Debugf("%s", tasksOutput)
	return task.parseVariants(tasksOutput), nil
}

func (task *Task) parseVariants(gradleOutput string) Variants {
	//example gradleOutput:
	//"
	// lintMyflavorokStaging - Runs lint on the MyflavorokStaging build.
	// lintMyflavorRelease - Runs lint on the MyflavorRelease build.
	// lintVitalMyflavorRelease - Runs lint on the MyflavorRelease build.
	// lintMyflavorStaging - Runs lint on the MyflavorStaging build."
	tasks := Variants{}
lines:
	for _, l := range strings.Split(gradleOutput, "\n") {
		// l: " lintMyflavorokStaging - Runs lint on the MyflavorokStaging build."
		l = strings.TrimSpace(l)
		// l: "lintMyflavorokStaging - Runs lint on the MyflavorokStaging build."
		if l == "" {
			continue
		}
		// l: "lintMyflavorokStaging"
		l = strings.Split(l, " ")[0]
		var module string

		split := strings.Split(l, ":")
		size := len(split)
		if size > 1 {
			module = strings.Join(split[:size-1], ":")
			l = split[size-1]
		}
		// module removed if any
		if strings.HasPrefix(l, task.name) {
			// task.name: "lint"
			// strings.HasPrefix will match lint and lintVital prefix also, we won't need lintVital so it is a conflict
			for _, conflict := range conflicts[task.name] {
				if strings.HasPrefix(l, conflict) {
					// if line has conflicting prefix don't do further checks with this line, skip...
					continue lines
				}
			}
			l = strings.TrimPrefix(l, task.name)
			// l: "MyflavorokStaging"
			if l == "" {
				continue
			}

			tasks[module] = append(tasks[module], l)
		}
	}

	for module, variants := range tasks {
		tasks[module] = cleanStringSlice(variants)
	}

	return tasks
}

func cleanModuleName(s string) string {
	if s == "" {
		return s
	}
	return ":" + s + ":"
}

// GetCommand ...
func (task *Task) GetCommand(v Variants, args ...string) command.Command {
	var a []string
	for module, variants := range v {
		for _, variant := range variants {
			a = append(a, cleanModuleName(module)+task.name+variant)
		}
	}
	cmdOpts := command.Opts{
		Dir:    task.project.location,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return task.project.cmdFactory.Create(filepath.Join(task.project.location, "gradlew"), append(a, args...), &cmdOpts)
}

package integration

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/namespaces"
)

func TestExecIn(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootFs()
	if err != nil {
		t.Fatal(err)
	}
	defer remove(rootfs)

	config := newTemplateConfig(rootfs)
	if err := writeConfig(config); err != nil {
		t.Fatalf("failed to write config %s", err)
	}

	// start the container
	containerErr := make(chan error, 1)
	containerCmd := &exec.Cmd{}
	var statePath string
	createCmd := func(container *libcontainer.Config, console, dataPath, init string,
		pipe *os.File, args []string) *exec.Cmd {
		containerCmd = namespaces.DefaultCreateCommand(container, console, dataPath, init, pipe, args)
		statePath = dataPath
		return containerCmd
	}
	var containerStart sync.WaitGroup
	containerStart.Add(1)
	go func() {
		buffers := newStdBuffers()
		_, err := namespaces.Exec(config,
			buffers.Stdin, buffers.Stdout, buffers.Stderr,
			"", config.RootFs, []string{"sleep", "10"},
			createCmd, containerStart.Done)
		containerErr <- err
	}()
	containerStart.Wait()

	defer func() {
		// kill the container
		if containerCmd.Process != nil {
			containerCmd.Process.Kill()
		}
		if err := <-containerErr; err != nil {
			t.Fatal(err)
		}
	}()

	// start the exec process
	state, err := libcontainer.GetState(statePath)
	if err != nil {
		t.Fatalf("failed to get state %s", err)
	}
	buffers := newStdBuffers()
	execErr := make(chan error, 1)
	go func() {
		_, err := namespaces.ExecIn(config, state, []string{"ps"},
			os.Args[0], "exec", buffers.Stdin, buffers.Stdout, buffers.Stderr,
			"", nil)
		execErr <- err
	}()
	if err := <-execErr; err != nil {
		t.Fatalf("exec finished with error %s", err)
	}

	out := buffers.Stdout.String()
	if !strings.Contains(out, "sleep 10") || !strings.Contains(out, "ps") {
		t.Fatalf("unexpected running process, output %q", out)
	}
}

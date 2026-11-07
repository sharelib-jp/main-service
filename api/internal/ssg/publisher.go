package ssg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type Command func(context.Context, string, ...string) error

func caddyCommand(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type Publisher struct {
	Output, Config string
	Run            Command
}

func (p Publisher) Publish(ctx context.Context) (bool, error) {
	guard, err := lock(p.Output)
	if err != nil {
		return false, err
	}
	defer unlock(guard)
	id := readID(p.Output, "requested")
	if id == "" || id == Current(p.Output) {
		return false, nil
	}
	dir, _ := generationDir(p.Output, id)
	if _, err = os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return false, err
	}
	active := filepath.Join(p.Output, "control", "active.caddy")
	before, err := os.ReadFile(active)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	include := []byte("import " + strconv.Quote(filepath.Join(dir, "routes.caddy")) + "\n")
	if err = atomicWrite(active, include, 0644); err != nil {
		return false, err
	}
	restore := func(cause error) (bool, error) {
		if e := atomicWrite(active, before, 0644); e != nil {
			return false, fmt.Errorf("%w; restoring include: %v", cause, e)
		}
		return false, cause
	}
	run := p.Run
	if run == nil {
		run = caddyCommand
	}
	if err = run(ctx, "caddy", "validate", "--config", p.Config, "--adapter", "caddyfile"); err != nil {
		return restore(fmt.Errorf("validate generation %s: %w", id, err))
	}
	if err = run(ctx, "caddy", "reload", "--config", p.Config, "--adapter", "caddyfile"); err != nil {
		return restore(fmt.Errorf("reload generation %s: %w", id, err))
	}
	// The last known-good include is the recovery source after a process/power failure.
	if err = atomicWrite(filepath.Join(p.Output, "control", "gateway-good.caddy"), include, 0644); err != nil {
		return false, err
	}
	if err = publishID(p.Output, id); err != nil {
		return false, err
	}
	prune(p.Output)
	log.Printf("catalog published id=%s", id)
	return true, nil
}
func prepareGateway(output string) error {
	guard, err := lock(output)
	if err != nil {
		return err
	}
	defer unlock(guard)
	good, err := os.ReadFile(filepath.Join(output, "control", "gateway-good.caddy"))
	if os.IsNotExist(err) {
		good = []byte("# No catalog has been published yet. Dynamic fallback is available.\n")
	} else if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(output, "control", "active.caddy"), good, 0644)
}

// RunGateway supervises Caddy and reloads only complete, validated generations.
// The admin listener remains local to this container; no Docker socket is needed.
func RunGateway(ctx context.Context, configPath, output string) error {
	if err := prepareGateway(output); err != nil {
		return err
	}
	cmd := exec.Command("caddy", "run", "--config", configPath, "--adapter", "caddyfile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Signal(syscall.SIGTERM) }()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	publisher := Publisher{Output: output, Config: configPath}
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			return nil
		case <-ticker.C:
			if _, err := publisher.Publish(ctx); err != nil && !errors.Is(err, ErrBusy) {
				log.Printf("catalog publication failed: %v", err)
			}
		}
	}
}

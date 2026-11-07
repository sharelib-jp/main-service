package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sharelib-jp/main-service/config"
	"github.com/sharelib-jp/main-service/internal/api"
	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/site"
	"github.com/sharelib-jp/main-service/internal/ssg"
	"github.com/sharelib-jp/main-service/internal/views"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "generate" || args[0] == "gateway") {
		command = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	path := flags.String("config", "./config.toml", "configuration file")
	output := flags.String("output", "/srv/main", "shared catalog directory (gateway only)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if command == "gateway" {
		return ssg.RunGateway(ctx, *path, *output)
	}
	conf, err := config.LoadConfig(*path)
	if err != nil {
		return err
	}
	name := conf.Metadata.NodeName
	if name == "" {
		name = "Sharelib"
	}
	website := views.Site{Name: name, Origin: conf.PublicOrigin()}
	source := catalog.NewClient(conf.Repository.BaseURL, conf.Repository.Token)
	if err := ssg.Initialize(conf.SSG.OutputDir); err != nil {
		return err
	}
	builder := &ssg.Builder{Source: source, Site: website, Output: conf.SSG.OutputDir, GatewayManaged: conf.SSG.GatewayManaged}
	if command == "generate" {
		_, err := builder.Generate(ctx)
		return err
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if conf.Repository.BaseURL == "" {
			return
		}
		interval, _ := time.ParseDuration(conf.SSG.Interval)
		for {
			_, _ = builder.Generate(ctx)
			if interval == 0 {
				return
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	router := api.NewRouter(conf, api.Dependencies{Site: site.New(source, website, conf.SSG.OutputDir)})
	server := &http.Server{Addr: conf.PORT, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			_ = server.Shutdown(shutdown)
		case <-stopped:
		}
	}()
	log.Printf("Listening on %s (public origin: %s)", conf.PORT, conf.PublicOrigin())
	err = server.ListenAndServe()
	close(stopped)
	cancel()
	<-workerDone
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

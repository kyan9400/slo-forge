package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kyan9400/slo-forge/internal/compiler"
	"github.com/kyan9400/slo-forge/internal/server"
	"github.com/kyan9400/slo-forge/internal/spec"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "validate":
		return validateCommand(args[1:], stdout, stderr)
	case "render":
		return renderCommand(args[1:], stdout, stderr)
	case "explain":
		return explainCommand(args[1:], stdout, stderr)
	case "serve":
		return serveCommand(args[1:], stdout, stderr)
	case "probe":
		return probeCommand(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "--help", "-h":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func validateCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "path to an SLO YAML file")
	flags.StringVar(file, "f", "", "path to an SLO YAML file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	document, err := loadFile(*file)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "valid: %s/%s (%.4g%% over %s)\n", document.Spec.Service, document.Metadata.Name, document.Spec.Target, document.Spec.Window)
	return nil
}

func renderCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "path to an SLO YAML file")
	flags.StringVar(file, "f", "", "path to an SLO YAML file")
	out := flags.String("out", "generated", "output directory")
	flags.StringVar(out, "o", "generated", "output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	document, err := loadFile(*file)
	if err != nil {
		return err
	}
	output, err := compiler.Compile(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	files := map[string][]byte{
		"prometheus-rules.yaml":  output.PrometheusRules,
		"prometheusrule.yaml":    output.PrometheusRule,
		"grafana-dashboard.json": output.GrafanaDashboard,
		"explanation.md":         output.Explanation,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(*out, name), contents, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	fmt.Fprintf(stdout, "rendered 4 artifacts to %s\n", *out)
	return nil
}

func explainCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("explain", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "path to an SLO YAML file")
	flags.StringVar(file, "f", "", "path to an SLO YAML file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	document, err := loadFile(*file)
	if err != nil {
		return err
	}
	output, err := compiler.Compile(document)
	if err != nil {
		return err
	}
	_, err = stdout.Write(output.Explanation)
	return err
}

func serveCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	address := flags.String("addr", ":8080", "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              *address,
		Handler:           server.New(version).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdownSignals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownSignals.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	fmt.Fprintf(stdout, "slo-forge %s listening on %s\n", version, *address)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func probeCommand(args []string, _ io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("url", "http://127.0.0.1:8080/healthz", "health endpoint")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, *endpoint, nil)
	if err != nil {
		return fmt.Errorf("create probe request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe returned %s", response.Status)
	}
	return nil
}

func loadFile(path string) (spec.Document, error) {
	if path == "" {
		return spec.Document{}, errors.New("--file is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return spec.Document{}, fmt.Errorf("read SLO: %w", err)
	}
	return spec.Load(data)
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `SLO Forge turns one reviewable SLO definition into monitoring artifacts.

Usage:
  slo-forge validate -f <slo.yaml>
  slo-forge render -f <slo.yaml> -o <directory>
  slo-forge explain -f <slo.yaml>
  slo-forge serve [--addr :8080]
  slo-forge probe [--url http://127.0.0.1:8080/healthz]
  slo-forge version`)
}

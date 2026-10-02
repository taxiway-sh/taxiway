// model-catalog-update prepares public-source catalog changes for human review.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/taxiway-sh/taxiway/internal/modelupdate"
)

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("model-catalog-update", flag.ContinueOnError)
	flags.SetOutput(out)
	catalog := flags.String("catalog", "infra/gateway/litellm/models.yaml", "catalog YAML path")
	fixtures := flags.String("fixtures", "", "directory containing all six <source>.txt files (offline)")
	write := flags.Bool("write", false, "write proposed catalog; otherwise report only")
	reportPath := flags.String("report", "", "write Markdown review report to this file")
	asOf := flags.String("as-of", time.Now().UTC().Format("2006-01-02"), "evaluate retirement at this UTC date")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	now, err := time.Parse("2006-01-02", *asOf)
	if err != nil {
		return fmt.Errorf("as-of: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sources, err := modelupdate.Load(ctx, &http.Client{Timeout: 20 * time.Second}, *fixtures)
	if err != nil {
		return err
	}
	before, err := os.ReadFile(*catalog)
	if err != nil {
		return err
	}
	after, report, changed, err := modelupdate.Prepare(before, sources, now)
	if err != nil {
		return err
	}
	if *reportPath != "" {
		if err = os.WriteFile(*reportPath, []byte(report), 0600); err != nil {
			return err
		}
	}
	if *write && changed {
		info, err := os.Stat(*catalog)
		if err != nil {
			return err
		}
		temp, err := os.CreateTemp(filepath.Dir(*catalog), ".model-catalog-*")
		if err != nil {
			return err
		}
		path := temp.Name()
		defer os.Remove(path)
		if _, err = temp.Write(after); err != nil {
			_ = temp.Close()
			return err
		}
		if err = temp.Chmod(info.Mode().Perm()); err != nil {
			_ = temp.Close()
			return err
		}
		if err = temp.Close(); err != nil {
			return err
		}
		if err = os.Rename(path, *catalog); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(out, report)
	return err
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "model-catalog-update:", err)
		os.Exit(1)
	}
}

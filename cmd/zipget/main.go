package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"2025-07-30/internal/loader"
	"2025-07-30/internal/model"
)

var validMIMETypes = []string{"application/pdf", "image/jpeg", "image/png"}

var (
	urlsFile   = flag.String("u", "", "Get URLs from file instead of command line, use '-' for stdin.")
	outputFile = flag.String("o", "", "Output file, use '-' for stdout.")
	statusFile = flag.String("s", "", "Save status to file, use '-' for stdout.")
	verbose    = flag.Bool("v", false, "Enable debug mode and output status to stderr.")
	nothing    = flag.Bool("n", false, "Don't download anything, check only with HEAD requests.")
)

func main() {
	flag.Parse()

	urls := flag.Args()
	if *urlsFile != "" {
		var err error
		urls, err = loadURLsFromFile(*urlsFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "URLs required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	setupLogger()

	var (
		files []model.File
		err   error
	)

	if *nothing {
		files, err = checkOnly(urls)
	} else {
		files, err = download(urls)
	}

	if err != nil {
		log.Fatalln(err)
	}

	if *verbose || *statusFile != "" {
		buf, _ := json.MarshalIndent(files, "", "    ")
		if *verbose {
			os.Stderr.Write(buf)
		}
		if *statusFile != "" {
			if err := os.WriteFile(*statusFile, buf, 0666); err != nil {
				log.Fatalf("write status failed: %v", err)
			}
		}
	}
}

func checkOnly(urls []string) ([]model.File, error) {
	ldr := loader.New(http.DefaultClient, validMIMETypes)
	return ldr.Check(context.Background(), urls)
}

func download(urls []string) ([]model.File, error) {
	if *outputFile == "" {
		fmt.Fprintln(os.Stderr, "output file required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	output := os.Stdout
	if *outputFile != "-" {
		var err error
		output, err = os.Create(*outputFile)
		if err != nil {
			log.Fatalf("create file failed: %v", err)
		}
		defer output.Close()
	}

	w := bufio.NewWriter(output)
	defer w.Flush()

	ldr := loader.New(http.DefaultClient, validMIMETypes)
	return ldr.Download(context.Background(), urls, w)
}

func setupLogger() {
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{Level: level},
	)))
	log.Printf("logging level %v", level)
}

func loadURLsFromFile(fileName string) ([]string, error) {
	input := os.Stdin
	if fileName != "-" {
		var err error
		input, err = os.Open(fileName)
		if err != nil {
			return nil, err
		}
		defer input.Close()
	}

	sc := bufio.NewScanner(input)
	var urls []string

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		urls = append(urls, line)
	}

	return urls, sc.Err()
}

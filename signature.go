package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

func getSignatures(outputDir string, filenames []string, numThreads int, randomize bool) {

	// Ensure the output directory exists
	if err := os.MkdirAll(filepath.Join(outputDir, "signatures"), os.ModePerm); err != nil {
		slog.Error(fmt.Sprintf("Error creating base output directory: %v\n", err))
		return
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup
	wg.Add(len(filenames))

	// Create a channel to limit the number of concurrent downloads
	semaphore := make(chan struct{}, numThreads)

	if randomize {
		randomizeStrings(filenames)
	}

	bar := progressbar.NewOptions(len(filenames),
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetDescription("[cyan][1/2][reset] Getting signature files..."),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	// Iterate over the filenames and download the files
	for _, filename := range filenames {
		bar.Add(1)
		// Skip INI files as they never have signatures - reduces requests by half!
		if strings.HasSuffix(filename, ".INI") {
			wg.Done() // Still need to decrement the wg as we used all file names for the wg size
			continue
		}

		// Add an empty struct to the semaphore channel to "take up" one slot/thread
		semaphore <- struct{}{}

		go func(filename string) {
			defer func() {
				// Read one struct from the semaphore channel to "let go" of one slot/thread
				<-semaphore
				wg.Done()
			}()

			url := fmt.Sprintf("%s/SMS_DP_SMSSIG$/%s.tar", urlBase, filename)
			outputPath := filepath.Join(outputDir, "signatures", filename+".tar")

			// Download the file
			err := downloadFileFromURL(url, outputPath)
			if err != nil {
				slog.Debug(fmt.Sprintf("Error downloading signature %s.tar: %v\n", filename, err))
				return
			}

			slog.Debug(fmt.Sprintf("Downloaded %s to %s\n", filename, outputPath))
		}(filename)
	}
	wg.Wait()
	bar.Finish()
}

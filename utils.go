package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

var allFileURLs []string

func downloadFiles(outputDir, filePath string, fileNames, allowExtensions []string, downloadNoExt bool, numThreads int, randomize bool) {
	filenameWithExt := filepath.Base(filePath)
	dirName := strings.TrimSuffix(filenameWithExt, filepath.Ext(filenameWithExt))

	outPath := filepath.Join(outputDir, "inis", dirName)
	// Ensure the output directory exists for inis
	if err := os.MkdirAll(outPath, os.ModePerm); err != nil {
		slog.Error(fmt.Sprintf("Error creating base output directory: %v\n", err))
		return
	}

	outPathFilesBase := filepath.Join(outputDir, "files")
	// Ensure the output directory exists for files
	if err := os.MkdirAll(outPathFilesBase, os.ModePerm); err != nil {
		slog.Error(fmt.Sprintf("Error creating base output directory: %v\n", err))
		return
	}

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup
	wg.Add(len(fileNames))

	// Create a channel to limit the number of concurrent downloads
	semaphore := make(chan struct{}, numThreads)

	if randomize {
		randomizeStrings(fileNames)
	}

	// Iterate over the filenames and download the files
	for _, filename := range fileNames {
		filename = strings.ReplaceAll(filename, "\\", "/")
		if strings.Contains(filename, "/") {
			dir := filepath.Dir(filename)
			if err := os.MkdirAll(filepath.Join(outPath, dir), os.ModePerm); err != nil {
				slog.Error(fmt.Sprintf("Error creating base output directory: %v\n", err))
				return
			}
		}

		wanted, outPathFiles := fileWanted(allowExtensions, downloadNoExt, filename, outputDir)
		if !wanted {
			wg.Done()
			continue
		}

		// Add an empty struct to the semaphore channel to "take up" one slot/thread
		semaphore <- struct{}{}
		go downloadINIAndFile(outPath, outPathFiles, filename, dirName, &wg, semaphore)
	}
	wg.Wait()

}

// Helper function to check if two slices of bytes are equal
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func extractFileName(href string) string {
	// Split the URL by '/'
	parts := strings.Split(href, "/")
	// Get the last part of the URL
	lastPart := parts[len(parts)-1]
	// Remove any leading or trailing spaces
	trimmedPart := strings.TrimSpace(lastPart)
	return trimmedPart
}

func randomizeStrings(strings []string) {
	// Create a new source for random numbers
	source := rand.NewSource(time.Now().UnixNano())
	random := rand.New(source)

	// Randomize the order of the slice using sort.Slice
	sort.Slice(strings, func(i, j int) bool {
		return random.Intn(2) == 0
	})
}

func extractURLs(fileDirectoryURL string) ([]string, []string) {
	html, err := getURL(fileDirectoryURL)
	if err != nil {
		return nil, nil
	}

	var fileURLs []string
	var dirURLs []string

	// Regular expression pattern to match URLs
	urlPattern := `\d+ <a href="(http://[^"]+)">`

	// Regular expression pattern to match directory URLs
	dirPattern := `&lt;dir&gt <a href="(http://[^"]+)">`

	// Find all URLs
	urls := regexp.MustCompile(urlPattern).FindAllStringSubmatch(html, -1)
	for _, url := range urls {
		fileURLs = append(fileURLs, url[1])
	}

	// Find all directory URLs
	doubleDirURLs := regexp.MustCompile(dirPattern).FindAllStringSubmatch(html, -1)
	for _, url := range doubleDirURLs {
		dirURLs = append(dirURLs, url[1])
	}

	return fileURLs, dirURLs
}

func getAllFileURLsFromDirNames(dataLibFiles []string, numThreads int, randomize bool) []string {

	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetDescription("[cyan][1/2][reset] Getting file URLs"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	// Create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup
	wg.Add(len(dataLibFiles))
	// Create a mutex for file appending
	mu := &sync.Mutex{}

	// Create a channel to limit the number of concurrent downloads
	semaphore := make(chan struct{}, numThreads)

	if randomize {
		randomizeStrings(dataLibFiles)
	}

	for _, dataLibFile := range dataLibFiles {
		// Skip INI files
		if strings.HasSuffix(dataLibFile, ".INI") {
			wg.Done()
			continue
		}
		var fileDirectoryURL string
		if !strings.Contains(dataLibFile, "http") {
			fileDirectoryURL = fmt.Sprintf("%s/SMS_DP_SMSPKG$/%s", urlBase, dataLibFile)
		} else {
			fileDirectoryURL = dataLibFile
		}

		// Add an empty struct to the semaphore channel to "take up" one slot/thread
		semaphore <- struct{}{}
		go getFilesFromDirNames(fileDirectoryURL, bar, &wg, semaphore, mu)

	}
	wg.Wait()
	bar.Finish()
	return allFileURLs

}

func getFilesFromDirNames(fileDirectoryURL string, bar *progressbar.ProgressBar, wg *sync.WaitGroup, semaphore chan struct{}, mu *sync.Mutex) {

	fileURLs, dirURLs := extractURLs(fileDirectoryURL)
	bar.Add(len(fileURLs))

	mu.Lock()
	allFileURLs = append(allFileURLs, fileURLs...)
	mu.Unlock()
	// Read one struct from the semaphore channel to "let go" of one slot/thread
	<-semaphore

	if len(dirURLs) > 0 {
		slog.Debug(fmt.Sprintf("Found %d directories in %s", len(dirURLs), fileDirectoryURL))
		for _, dirURL := range dirURLs {
			wg.Add(1)
			// Add an empty struct to the semaphore channel to "take up" one slot/thread
			semaphore <- struct{}{}
			getFilesFromDirNames(dirURL, bar, wg, semaphore, mu)
		}
	}

	// If we "Done()" before the Add it the wg could hit zero and then an Add could run and panic the Wait()
	wg.Done()
}

func fileWanted(allowExtensions []string, downloadNoExt bool, filename string, outputDir string) (bool, string) {
	var outPathFiles string
	fileSuffix := filepath.Ext(filename)
	// Remove the leading dot (.) from the file suffix
	if len(fileSuffix) > 1 {
		fileSuffix = fileSuffix[1:]
		if allowExtensions != nil && !slices.Contains(allowExtensions, "all") && !slices.Contains(allowExtensions, fileSuffix) {
			slog.Debug(fmt.Sprintf("Skipping %s: %s not wanted", filename, fileSuffix))
			return false, ""
		}
		outPathFiles = filepath.Join(outputDir, "files", fileSuffix)
		// Ensure the output directory exists for files
		if err := os.MkdirAll(outPathFiles, os.ModePerm); err != nil {
			slog.Error(fmt.Sprintf("Error creating file type output directory: %v\n", err))
			return false, ""
		}
	} else {
		if downloadNoExt {
			slog.Debug(fmt.Sprintf("File %s has no file extension, downloading it!", filename))
			outPathFiles = filepath.Join(outputDir, "files", "UKN")
			// Ensure the output directory exists for files
			if err := os.MkdirAll(outPathFiles, os.ModePerm); err != nil {
				slog.Error(fmt.Sprintf("Error creating file type output directory: %v\n", err))
				return false, ""
			}
		} else {
			slog.Debug(fmt.Sprintf("File %s has no file extension, and files without extensions are not being kept, skipping", filename))
			return false, ""
		}
	}
	return true, outPathFiles
}

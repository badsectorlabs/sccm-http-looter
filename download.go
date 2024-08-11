package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func getDatalibListing(server, outputDir string) (string, error) {
	// Ensure the base output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		slog.Error(fmt.Sprintf("Error creating base output directory: %v\n", err))
		return "", err
	}

	url := fmt.Sprintf("%s/SMS_DP_SMSPKG$/Datalib", urlBase)
	slog.Info(fmt.Sprintf("Getting Datalib listing from %s...\n", url))

	response, err := customHTTPClient.Get(url)
	if err != nil {
		slog.Error(fmt.Sprintf("Error sending GET request: %v\n", err))
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		slog.Error(fmt.Sprintf("Received non-OK status code: %v\n", response.Status))
		return "", fmt.Errorf("received non-OK status code: %v", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Error(fmt.Sprintf("Error reading response body: %v\n", err))
		slog.Error(fmt.Sprintf(`

Try to download the Datalib manually with curl:
curl -k -A 'sccm-http-looter' %s > datalib.html
then run sccmlooter with '-datalib datalib.html'

`, url))
		return "", errors.New("error reading response body")
	}

	outputFileName := filepath.Join(outputDir, server+"_Datalib.txt")

	err = os.WriteFile(outputFileName, body, 0644)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing to file: %v\n", err))
		return "", err
	}

	slog.Debug(fmt.Sprintf("Data saved to %s\n", outputFileName))
	return string(body), nil
}

func downloadINIAndFile(outPath, outPathFiles, filename, dirName string, wg *sync.WaitGroup, semaphore chan struct{}) {
	defer func() {
		// Read one struct from the semaphore channel to "let go" of one slot/thread
		<-semaphore
		wg.Done()
	}()

	outputPath := filepath.Join(outPath, filename+".INI")
	url := fmt.Sprintf("%s/SMS_DP_SMSPKG$/Datalib/%s/%s.INI", urlBase, dirName, filename)

	err := downloadFileFromURL(url, outputPath)
	if err != nil {
		slog.Debug(fmt.Sprintf("Error downloading %s: %v\n", filename+".INI", err))
		return
	}

	slog.Debug(fmt.Sprintf("Downloaded %s to %s\n", filename+".INI", outputPath))
	hash, err := getHashFromINI(outputPath)
	if err != nil {
		slog.Debug(fmt.Sprintf("Error getting Hash from INI file %s: %v", outputPath, err))
		return
	}

	// Get the actual file by its hash but save it to the correct name
	if strings.Contains(filename, "/") {
		filename = filepath.Base(filename)
	}
	outputPathFile := filepath.Join(outPathFiles, hash[0:4]+"_sig_"+filename)
	fileURL := fmt.Sprintf("%s/SMS_DP_SMSPKG$/FileLib/%s/%s", urlBase, hash[0:4], hash)

	err = downloadFileFromURL(fileURL, outputPathFile)
	if err != nil {
		slog.Debug(fmt.Sprintf("Error downloading %s/%s: %v\n", hash[0:4], hash, err))
		return
	}

	slog.Debug(fmt.Sprintf("Downloaded %s to %s\n", filename, outputPathFile))

}

func downloadFileFromURL(url, outputPath string) error {
	slog.Debug(fmt.Sprintf("Downloading %s", url))
	// Send HTTP GET request to the URL
	response, err := customHTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Check if the response status code is OK
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status code: %d", response.StatusCode)
	}
	// Create or truncate the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the response body to the output file
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func getURL(url string) (string, error) {
	slog.Debug(fmt.Sprintf("Getting %s\n", url))

	response, err := customHTTPClient.Get(url)
	if err != nil {
		slog.Debug(fmt.Sprintf("Error sending GET request: %v\n", err))
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("Received non-OK status code: %v\n", response.Status))
		return "", err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("Error reading response body: %v\n", err))
		return "", err
	}
	return string(body), nil
}

func downloadFileFromURLAsHashName(url, outputDir string, wg *sync.WaitGroup, semaphore chan struct{}) error {
	defer func() {
		// Read one struct from the semaphore channel to "let go" of one slot/thread
		<-semaphore
		wg.Done()
	}()
	var outputPath string
	parts := strings.Split(url, "/")
	if !(len(parts) > 0) {
		slog.Debug(fmt.Sprintf("could not get file name from URL: %s", url))
		return fmt.Errorf("could not get file name from URL: %s", url)
	}

	slog.Debug(fmt.Sprintf("Downloading %s", url))

	// Send HTTP GET request to the URL
	response, err := customHTTPClient.Get(url)
	if err != nil {
		slog.Debug(fmt.Sprintf("%v", err))
		return err
	}
	defer response.Body.Close()

	// Check if the response status code is OK
	if response.StatusCode != http.StatusOK {
		slog.Debug(fmt.Sprintf("HTTP request failed with status code: %d", response.StatusCode))
		return fmt.Errorf("HTTP request failed with status code: %d", response.StatusCode)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Debug(fmt.Sprintf("%v", err))
		return err
	}

	// Hash the file in memory
	hasher := sha256.New()
	hasher.Write(content)
	hash := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))

	// Append the hash to the beginning of the file name
	outputPath = filepath.Join(outputDir, hash[0:4]+"_url_"+parts[len(parts)-1])

	slog.Debug(fmt.Sprintf("Output path: %s", outputPath))

	// Write the response body to the output file
	err = os.WriteFile(outputPath, content, 0644)
	if err != nil {
		slog.Debug(fmt.Sprintf("%v", err))
		return err
	}

	return nil
}

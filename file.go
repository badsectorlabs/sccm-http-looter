package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

func walkDir(signaturesDir string) []string {
	// Initialize a slice to store the file paths
	filePaths := []string{}

	// Define the function to be called for each file or directory found
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Error(fmt.Sprintf("Error walking path %s: %v\n", path, err))
			return nil
		}

		// Check if it's a regular file (not a directory)
		if !info.IsDir() {
			filePaths = append(filePaths, path)
		}

		return nil
	}

	// Recursively walk the directory and collect file paths
	err := filepath.Walk(signaturesDir, walkFn)
	if err != nil {
		slog.Error(fmt.Sprintf("Error walking directory: %v\n", err))
		return nil
	}
	return filePaths
}

func getHashFromINI(filePath string) (string, error) {
	cfg, err := ini.Load(filePath)
	if err != nil {
		return "", err
	}

	section := cfg.Section("File")
	if section == nil {
		return "", fmt.Errorf("section 'File' not found in the INI file")
	}

	hashValue := section.Key("Hash").String()
	return hashValue, nil
}

func getFileNamesFromSignatureFile(filePath string) ([]string, error) {
	// Open the binary file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the entire file into memory
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()
	fileData := make([]byte, fileSize)
	_, err = file.Read(fileData)
	if err != nil {
		return nil, err
	}

	// Define the byte signature to search for
	signature := []byte{0x18, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}

	// Initialize a slice to store the file strings where the signature is found
	strings := []string{}

	// Search for the signature in the file data
	for i := 0; i < (len(fileData) - len(signature)); i++ {
		if bytesEqual(fileData[i:i+len(signature)], signature) {
			// Calculate the start offset for the string (512 bytes before the signature)
			startOffset := i - 512
			if startOffset < 0 {
				startOffset = 0
			}

			// Find the end of the string (up to the first null byte)
			endOffset := startOffset
			for endOffset < len(fileData) && fileData[endOffset] != 0x00 {
				endOffset++
			}

			// Extract the string
			stringBytes := fileData[startOffset:endOffset]

			strings = append(strings, string(stringBytes))
		}
	}

	return strings, nil
}

func writeStringArrayToFile(filePath string, stringArray []string) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	// Create a buffered writer
	writer := bufio.NewWriter(file)

	// Join the strings with a newline character and write to the file
	line := strings.Join(stringArray, "\n")
	_, err = writer.WriteString(line)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	// Flush the buffered writer to ensure all data is written
	err = writer.Flush()
	if err != nil {
		fmt.Println("Error flushing writer:", err)
		return
	}
}

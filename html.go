package main

import (
	"strings"

	"golang.org/x/net/html"
)

func extractFileNames(htmlContent string) []string {
	var fileNames []string

	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))

	for {
		tokenType := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			return fileNames
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data == "a" {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						// Extract the file name from the href attribute
						fileName := extractFileName(attr.Val)
						if fileName != "" {
							fileNames = append(fileNames, fileName)
						}
					}
				}
			}
		}
	}
}

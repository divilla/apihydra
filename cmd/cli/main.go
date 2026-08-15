package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var InvalidPathError = errors.New("invalid path")

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		subdir := os.Args[1]
		path := filepath.Join(dir, subdir)
		info, err := os.Stat(path)
		if err != nil {
			log.Fatal(fmt.Errorf("%w: %w", InvalidPathError, err))
		}
		if !info.IsDir() {
			log.Fatal(fmt.Errorf("%w: %v", InvalidPathError, path))
		}
		dir = path
	}
	fmt.Printf("%s\n", dir)
}

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

var InvalidPathError = errors.New("invalid path")

type YAMLString string

func (s *YAMLString) UnmarshalYAML(node ast.Node) error {
	*s = YAMLString(node.String())
	return nil
}

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

	yml := `---
foo: 1
bar: {"a":"1","b":"2"}
`
	var v struct {
		A int        `yaml:"foo"`
		B YAMLString `yaml:"bar"`
	}

	if err = yaml.Unmarshal([]byte(yml), &v); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", v)
}

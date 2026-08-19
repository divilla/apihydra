package domain

import "testing"

func TestStepProvenance(t *testing.T) {
	directory := &Directory{Stage: 3, Path: "/parent/child"}
	file := &File{Path: "/parent/child/steps.yaml", Directory: directory}
	step := &Step{Definition: &StepsDefinition{File: file}}

	if got := step.DirectoryStage(); got != directory.Stage {
		t.Fatalf("DirectoryStage() = %d, want %d", got, directory.Stage)
	}
	if got := step.DirectoryPath(); got != directory.Path {
		t.Fatalf("DirectoryPath() = %q, want %q", got, directory.Path)
	}
	if got := step.FilePath(); got != file.Path {
		t.Fatalf("FilePath() = %q, want %q", got, file.Path)
	}
}

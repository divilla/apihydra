package execution

import (
	"fmt"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

type (
	dirsValidator struct {
		suite        *domain.Suite
		isChildCount map[*domain.Directory]int
	}
	stagedDirs struct {
		stagedDirs [][]*domain.Directory
	}
)

func newDirsValidator(suite *domain.Suite) *dirsValidator {
	return &dirsValidator{
		suite:        suite,
		isChildCount: make(map[*domain.Directory]int),
	}
}

func (d *dirsValidator) validateRoot() error {
	if d.suite == nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite is nil")
	}
	root := d.suite.Root
	if root == nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root is nil")
	}
	if root.Parent != nil {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root.Parent is not nil")
	}
	if root.Stage != 0 {
		return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, "domain.Suite.Root.Stage is not 0")
	}

	d.isChildCount[root] = 1
	return d.validateChildren(root)
}

func (d *dirsValidator) validateChildren(parent *domain.Directory) error {
	for _, child := range parent.Children {
		if child == nil {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has nil child", parent.Path))
		}
		if child.Parent != parent {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has invalid parent directory", child.Path))
		}
		if child.Stage != parent.Stage+1 {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has invalid stage number", child.Path))
		}

		d.isChildCount[child]++
		if d.isChildCount[child] > 1 {
			return errs.Build(errs.ExitConfiguration, ErrInvalidDirectoryTree, nil, fmt.Sprintf("%s has multiple parents", child.Path))
		}
		if err := d.validateChildren(child); err != nil {
			return err
		}
	}

	return nil
}

func findMaxStage(dir *domain.Directory, maxStage int) int {
	if dir.Stage > maxStage {
		maxStage = dir.Stage
	}
	for _, child := range dir.Children {
		maxStage = findMaxStage(child, maxStage)
	}
	return maxStage
}

func newStagedDirs(maxStage int) *stagedDirs {
	return &stagedDirs{
		stagedDirs: make([][]*domain.Directory, maxStage+1),
	}
}

func (d *stagedDirs) setStages(dir *domain.Directory) {
	d.stagedDirs[dir.Stage] = append(d.stagedDirs[dir.Stage], dir)

	for _, child := range dir.Children {
		d.setStages(child)
	}
}

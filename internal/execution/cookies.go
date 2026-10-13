package execution

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/divilla/apihydra/internal/domain"
	"github.com/divilla/apihydra/pkg/errs"
)

var errCookieJar = errors.New("cookie jar error")

type cookieJars struct {
	mu              sync.Mutex
	mode            int
	tempRunDir      string
	namespace       string
	runJar          string
	directoryJars   map[*domain.Directory]string
	fileJars        map[*domain.Directory][]string
	incomingSources map[*domain.Directory]string
	latestCompleted map[*domain.Directory]string
	rootInheritance string
}

func newCookieJars(config domain.Config) *cookieJars {
	return &cookieJars{
		mode:            config.Parallelism,
		tempRunDir:      config.TempRunDir,
		directoryJars:   make(map[*domain.Directory]string),
		fileJars:        make(map[*domain.Directory][]string),
		incomingSources: make(map[*domain.Directory]string),
		latestCompleted: make(map[*domain.Directory]string),
	}
}

func (c *cookieJars) prepareStage(stage []*domain.Directory) error {
	for _, dir := range stage {
		if err := c.prepareDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

func (c *cookieJars) prepareDirectory(dir *domain.Directory) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureNamespace(); err != nil {
		return err
	}
	switch c.mode {
	case 0:
		if c.runJar == "" {
			jar, err := c.createJar("")
			if err != nil {
				return err
			}
			c.runJar = jar
		}
	case 1:
		if _, ok := c.directoryJars[dir]; ok {
			return nil
		}
		var source string
		if dir.Parent != nil {
			source = c.directoryJars[dir.Parent]
		}
		jar, err := c.createJar(source)
		if err != nil {
			return err
		}
		c.directoryJars[dir] = jar
	case 2:
		if _, ok := c.fileJars[dir]; ok {
			return nil
		}
		var source string
		if dir.Parent != nil {
			source = c.sourceForChildren(dir.Parent)
			c.incomingSources[dir] = source
		}
		jars := make([]string, len(dir.RuntimeSteps))
		for index := range jars {
			jar, err := c.createJar(source)
			if err != nil {
				return err
			}
			jars[index] = jar
		}
		c.fileJars[dir] = jars
		if dir.Parent == nil && len(jars) == 0 {
			jar, err := c.createJar("")
			if err != nil {
				return err
			}
			c.rootInheritance = jar
		}
	}
	return nil
}

func (c *cookieJars) jarFor(dir *domain.Directory, fileIndex int) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mode == 1 {
		return c.directoryJars[dir]
	}
	if c.mode == 2 {
		return c.fileJars[dir][fileIndex]
	}
	return c.runJar
}

func (c *cookieJars) recordCompletion(dir *domain.Directory, fileIndex int) {
	if c.mode != 2 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if jars := c.fileJars[dir]; fileIndex >= 0 && fileIndex < len(jars) {
		c.latestCompleted[dir] = jars[fileIndex]
	}
}

func (c *cookieJars) sourceForChildren(dir *domain.Directory) string {
	if source := c.latestCompleted[dir]; source != "" {
		return source
	}
	if jars := c.fileJars[dir]; len(jars) > 0 {
		return jars[0]
	}
	if source := c.incomingSources[dir]; source != "" {
		return source
	}
	return c.rootInheritance
}

func (c *cookieJars) ensureNamespace() error {
	if c.namespace != "" {
		return nil
	}
	if c.tempRunDir == "" {
		return c.failure(nil, "temporary run directory is empty")
	}
	runDir := filepath.Clean(c.tempRunDir)
	// curl treats any --cookie operand containing '=' as literal Cookie data,
	// so such a run path cannot safely name the required input jar.
	if strings.Contains(runDir, "=") {
		return c.failure(nil, "temporary run directory cannot be used as a curl cookie filename", runDir)
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return c.failure(err, "inspect temporary run directory", runDir)
	}
	if !info.IsDir() {
		return c.failure(nil, "temporary run directory is not a directory", runDir)
	}

	namespace := filepath.Join(runDir, "cookies")
	if err := os.Mkdir(namespace, 0o700); err != nil {
		return c.failure(err, "create cookie namespace", namespace)
	}
	c.namespace = namespace
	return nil
}

func (c *cookieJars) createJar(source string) (string, error) {
	file, err := os.CreateTemp(c.namespace, "jar-*.cookie.jar")
	if err != nil {
		return "", c.failure(err, "create cookie jar", c.namespace)
	}
	path := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if source != "" {
		input, err := os.Open(source)
		if err != nil {
			return "", c.failure(err, "open cookie inheritance source", source)
		}
		_, copyErr := io.Copy(file, input)
		_ = input.Close()
		if copyErr != nil {
			return "", c.failure(copyErr, "copy cookie jar", source, path)
		}
	}
	if err := file.Close(); err != nil {
		return "", c.failure(err, "initialize cookie jar", path)
	}
	keep = true
	return path, nil
}

func (c *cookieJars) failure(cause error, details ...any) error {
	return errs.Build(errs.ExitInternal, errCookieJar, cause, details...)
}

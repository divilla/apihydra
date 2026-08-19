// Package domain owns the shared APIHydra workflow values.
package domain

// DocumentKind classifies an APIHydra definition document.
type DocumentKind string

const (
	// KindRoot identifies a root definition document.
	KindRoot DocumentKind = "root"
	// KindDefaults identifies a defaults definition document.
	KindDefaults DocumentKind = "defaults"
	// KindSteps identifies a steps definition document.
	KindSteps DocumentKind = "steps"
)

// Suite is the parsed tree anchored by the selected working directory.
type Suite struct {
	WorkDir string
	Root    *Directory
}

// Directory contains the definition and runtime state for one suite directory.
type Directory struct {
	Stage              int
	Path               string
	Parent             *Directory
	Children           []*Directory
	Files              []*File
	DefaultsFile       *File
	StepsFiles         []*File
	DefaultsDefinition *DefaultsDefinition
	StepsDefinitions   []*StepsDefinition
	ResolvedDefaults   Defaults
	ResolvedSteps      [][]Step
	RuntimeSteps       [][]Step
}

// File contains one definition file and its owning directory.
type File struct {
	Stage     int
	Path      string
	Kind      DocumentKind
	Bytes     []byte
	Directory *Directory
}

// BaseDefinition contains the fields shared by definition documents.
type BaseDefinition struct {
	App  string     `yaml:"app"`
	Kind string     `yaml:"kind"`
	Spec YAMLString `yaml:"spec"`
}

// DefaultsDefinition contains one decoded defaults document.
type DefaultsDefinition struct {
	App      string       `yaml:"app"`
	Kind     DocumentKind `yaml:"kind"`
	Metadata Metadata     `yaml:"metadata"`
	Spec     Defaults     `yaml:"spec"`
	File     *File        `yaml:"-"`
}

// StepsDefinition contains one decoded steps document.
type StepsDefinition struct {
	App      string       `yaml:"app"`
	Kind     DocumentKind `yaml:"kind"`
	Metadata Metadata     `yaml:"metadata"`
	Spec     struct {
		Steps []Step `yaml:"steps"`
	} `yaml:"spec"`
	File *File `yaml:"-"`
}

// Metadata describes a definition document.
type Metadata struct {
	Name   string   `yaml:"name"`
	Labels []string `yaml:"labels"`
}

// Defaults contains request values inherited by steps.
type Defaults struct {
	BaseURL  string            `yaml:"baseUrl"`
	BasePath string            `yaml:"basePath"`
	Headers  map[string]string `yaml:"headers"`
	Timeout  int               `yaml:"timeout"`
	Retries  int               `yaml:"retries"`
}

// Step contains one declarative or runtime request step.
type Step struct {
	Vars    map[string]YAMLString `yaml:"vars" json:"vars"`
	Request struct {
		Method   string            `yaml:"method" json:"method"`
		BaseURL  string            `yaml:"baseUrl" json:"baseUrl"`
		BasePath string            `yaml:"basePath" json:"basePath"`
		Path     string            `yaml:"path" json:"path"`
		Headers  map[string]string `yaml:"headers" json:"headers"`
		Timeout  int               `yaml:"timeout" json:"timeout"`
		Retries  int               `yaml:"retries" json:"retries"`
		Query    string            `yaml:"query" json:"query"`
		Body     YAMLString        `yaml:"body" json:"body"`
	} `yaml:"request" json:"request"`
	Response struct {
		Status   []int                 `yaml:"status" json:"status"`
		Body     string                `yaml:"body" json:"body"`
		Expected YAMLString            `yaml:"expected" json:"expected"`
		Types    map[string][]string   `yaml:"types" json:"types"`
		Capture  map[string]YAMLString `yaml:"capture" json:"capture"`
	} `yaml:"response" json:"response"`
	Debug      bool             `yaml:"debug" json:"debug"`
	Definition *StepsDefinition `yaml:"-" json:"-"`
	Index      int              `yaml:"-" json:"index"`
}

// DirectoryStage returns the stage of the step's source directory.
func (s *Step) DirectoryStage() int {
	return s.Definition.File.Directory.Stage
}

// DirectoryPath returns the path of the step's source directory.
func (s *Step) DirectoryPath() string {
	return s.Definition.File.Directory.Path
}

// FilePath returns the path of the step's source file.
func (s *Step) FilePath() string {
	return s.Definition.File.Path
}

// YAMLString stores a YAML value in its source string representation.
type YAMLString string

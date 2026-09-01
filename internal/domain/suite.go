package domain

// DocumentKind identifies the role of a definition document.
type DocumentKind string

// Supported definition document kinds.
const (
	KindRoot     DocumentKind = "root"
	KindDefaults DocumentKind = "defaults"
	KindSteps    DocumentKind = "steps"
)

// Suite is the parsed tree anchored by the selected working directory.
type Suite struct {
	WorkDir string
	Root    *Directory
}

// Directory is one directory in a suite's definition tree.
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

// File is a definition file and its owning directory.
type File struct {
	Stage     int
	Path      string
	Kind      DocumentKind
	Bytes     []byte
	Directory *Directory
}

// BaseDefinition contains the fields used to classify a definition document.
type BaseDefinition struct {
	App  string     `yaml:"app" json:"app"`
	Kind string     `yaml:"kind" json:"kind"`
	Spec YAMLString `yaml:"spec" json:"spec"`
}

// DefaultsDefinition contains request defaults decoded from a definition file.
type DefaultsDefinition struct {
	App      string       `yaml:"app" json:"app"`
	Kind     DocumentKind `yaml:"kind" json:"kind"`
	Metadata Metadata     `yaml:"metadata" json:"metadata"`
	Spec     Defaults     `yaml:"spec" json:"spec"`
	File     *File        `yaml:"-" json:"-"`
}

// StepsDefinition contains executable steps decoded from a definition file.
type StepsDefinition struct {
	App      string       `yaml:"app" json:"app"`
	Kind     DocumentKind `yaml:"kind" json:"kind"`
	Metadata Metadata     `yaml:"metadata" json:"metadata"`
	Spec     struct {
		Defaults Defaults `yaml:"defaults" json:"defaults"`
		Steps    []Step   `yaml:"steps" json:"steps"`
	} `yaml:"spec" json:"spec"`
	File *File `yaml:"-" json:"-"`
}

// Metadata describes a named definition and its labels.
type Metadata struct {
	Name   string   `yaml:"name" json:"name"`
	Labels []string `yaml:"labels" json:"labels"`
}

// Defaults contains request values inherited across definition scopes.
type Defaults struct {
	BaseURL        string            `yaml:"base_url" json:"base_url"`
	BasePath       string            `yaml:"base_path" json:"base_path"`
	Headers        map[string]string `yaml:"headers" json:"headers"`
	DisableCookies *bool             `yaml:"disable_cookies" json:"disable_cookies"`
	Timeout        int               `yaml:"timeout" json:"timeout"`
	Retries        int               `yaml:"retries" json:"retries"`
}

// Step contains the declarative and runtime state of one request step.
type Step struct {
	Index   int                   `yaml:"-" json:"index"`
	Vars    map[string]YAMLString `yaml:"vars" json:"vars"`
	Request struct {
		Path     string     `yaml:"path" json:"path"`
		Method   string     `yaml:"method" json:"method"`
		Query    string     `yaml:"query" json:"query"`
		Body     YAMLString `yaml:"body" json:"body"`
		Defaults Defaults   `yaml:"defaults" json:"defaults"`
	} `yaml:"request" json:"request"`
	Response struct {
		ExpectedStatus int                   `yaml:"expected_status" json:"expected_status"`
		ActualStatus   int                   `yaml:"actual_status" json:"actual_status"`
		ExpectedBody   YAMLString            `yaml:"expected_body" json:"expected_body"`
		ActualBody     YAMLString            `yaml:"actual_body" json:"actual_body"`
		ExpectedTypes  map[string][]string   `yaml:"expected_types" json:"expected_types"`
		Capture        map[string]YAMLString `yaml:"capture" json:"capture"`
	} `yaml:"response" json:"response"`
	Debug bool `yaml:"debug" json:"debug"`
	// RawCurl retains the complete, unredacted statement returned by
	// runner.CurlRaw for the latest runtime request. Reporter emits it
	// separately from the Step JSON.
	RawCurl    string           `yaml:"-" json:"-"`
	Definition *StepsDefinition `yaml:"-" json:"-"`
}

// DirectoryStage returns the stage of the step's owning directory.
func (s *Step) DirectoryStage() int {
	return s.Definition.File.Directory.Stage
}

// DirectoryPath returns the path of the step's owning directory.
func (s *Step) DirectoryPath() string {
	return s.Definition.File.Directory.Path
}

// FilePath returns the path of the step's source definition file.
func (s *Step) FilePath() string {
	return s.Definition.File.Path
}

// YAMLString preserves a scalar string when marshaling and unmarshaling YAML.
type YAMLString string

// MarshalYAML returns the underlying string for YAML encoding.
func (s YAMLString) MarshalYAML() (interface{}, error) {
	return string(s), nil
}

// UnmarshalYAML decodes a YAML scalar string without further interpretation.
func (s *YAMLString) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	*s = YAMLString(value)
	return nil
}

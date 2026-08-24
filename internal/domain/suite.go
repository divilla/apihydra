package domain

type DocumentKind string

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

type File struct {
	Stage     int
	Path      string
	Kind      DocumentKind
	Bytes     []byte
	Directory *Directory
}

type BaseDefinition struct {
	App  string     `yaml:"app"`
	Kind string     `yaml:"kind"`
	Spec YAMLString `yaml:"spec"`
}

type DefaultsDefinition struct {
	App      string       `yaml:"app"`
	Kind     DocumentKind `yaml:"kind"`
	Metadata Metadata     `yaml:"metadata"`
	Spec     Defaults     `yaml:"spec"`
	File     *File        `yaml:"-"`
}

type StepsDefinition struct {
	App      string       `yaml:"app"`
	Kind     DocumentKind `yaml:"kind"`
	Metadata Metadata     `yaml:"metadata"`
	Spec     struct {
		Steps []Step `yaml:"steps"`
	} `yaml:"spec"`
	File *File `yaml:"-"`
}

type Metadata struct {
	Name   string   `yaml:"name"`
	Labels []string `yaml:"labels"`
}

type Defaults struct {
	BaseURL  string            `yaml:"baseUrl"`
	BasePath string            `yaml:"basePath"`
	Headers  map[string]string `yaml:"headers"`
	Timeout  int               `yaml:"timeout"`
	Retries  int               `yaml:"retries"`
}

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
		ExpectedStatus int                   `yaml:"expected_status" json:"expected_status"`
		ActualStatus   int                   `yaml:"actual_status" json:"actual_status"`
		ExpectedBody   YAMLString            `yaml:"expected_body" json:"expected_body"`
		ActualBody     string                `yaml:"actual_body" json:"actual_body"`
		ExpectedTypes  map[string][]string   `yaml:"expected_types" json:"expected_types"`
		Capture        map[string]YAMLString `yaml:"capture" json:"capture"`
	} `yaml:"response" json:"response"`
	Debug      bool             `yaml:"debug" json:"debug"`
	Definition *StepsDefinition `yaml:"-" json:"-"`
	Index      int              `yaml:"-" json:"index"`
}

func (s *Step) DirectoryStage() int {
	return s.Definition.File.Directory.Stage
}

func (s *Step) DirectoryPath() string {
	return s.Definition.File.Directory.Path
}

func (s *Step) FilePath() string {
	return s.Definition.File.Path
}

type YAMLString string

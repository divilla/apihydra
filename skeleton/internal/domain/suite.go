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
	App  string     `yaml:"app" json:"app"`
	Kind string     `yaml:"kind" json:"kind"`
	Spec YAMLString `yaml:"spec" json:"spec"`
}

type DefaultsDefinition struct {
	App      string       `yaml:"app" json:"app"`
	Kind     DocumentKind `yaml:"kind" json:"kind"`
	Metadata Metadata     `yaml:"metadata" json:"metadata"`
	Spec     Defaults     `yaml:"spec" json:"spec"`
	File     *File        `yaml:"-" json:"-"`
}

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

type Metadata struct {
	Name   string   `yaml:"name" json:"name"`
	Labels []string `yaml:"labels" json:"labels"`
}

type Defaults struct {
	BaseURL  string            `yaml:"base_url" json:"base_url"`
	BasePath string            `yaml:"base_path" json:"base_path"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	Timeout  int               `yaml:"timeout" json:"timeout"`
	Retries  int               `yaml:"retries" json:"retries"`
}

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
	Debug      bool             `yaml:"debug" json:"debug"`
	Definition *StepsDefinition `yaml:"-" json:"-"`
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

func (s YAMLString) MarshalYAML() (interface{}, error) {
	return string(s), nil
}

func (s *YAMLString) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	*s = YAMLString(value)
	return nil
}

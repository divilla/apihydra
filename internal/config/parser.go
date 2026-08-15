package config

type (
	Wrapper struct {
		FilePath      string
		ParentConfig  *Config
		Config        Config
		RuntimeConfig Config
		RuntimeTasks  []Task
	}

	Config struct {
		App      string `yaml:"app"`
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name   string   `yaml:"name"`
			Labels []string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			BaseURL  string            `yaml:"baseUrl"`
			BasePath string            `yaml:"basePath"`
			Headers  map[string]string `yaml:"headers"`
			Tasks    []Task            `yaml:"tasks"`
		}
	}

	Task struct {
		Request struct {
			Vars     map[string]interface{} `yaml:"vars"`
			Method   string                 `yaml:"method"`
			BaseURL  string                 `yaml:"baseUrl"`
			BasePath string                 `yaml:"basePath"`
			Path     string                 `yaml:"path"`
			Headers  map[string]string      `yaml:"headers"`
			Query    string                 `yaml:"query"`
			Body     YAMLString             `yaml:"body"`
		} `yaml:"request"`
		Response struct {
			Vars     map[string]string `yaml:"setVars"`
			Types    map[string]string `yaml:"setVars"`
			Expected YAMLString        `yaml:"expected"`
		}
	}

	Parser struct {
		WorkDir string
		Files   []string
	}

	YAMLString string
)

func NewParser(workDir string, files []string) *Parser {
	return &Parser{
		WorkDir: workDir,
		Files:   files,
	}
}

func (p *Parser) Parse() {

}

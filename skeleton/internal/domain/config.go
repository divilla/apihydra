package domain

// Config carries parsed CLI values and the private per-run temporary
// directory created by cmd/apih. It is passed by value to runtime services.
type Config struct {
	Parallelism int
	Directory   string
	TempRunDir  string
}

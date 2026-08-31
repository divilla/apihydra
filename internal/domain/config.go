package domain

// Config carries parsed CLI values and the private per-run temporary
// directory created by cmd/cli. It is passed by value to runtime services.
type Config struct {
	Parallelism int
	Directory   string
	TempRunDir  string
}

package logging

// Factory menyediakan cara untuk membuat logger instances.
type Factory struct {
	defaultLevel Level
}

// NewFactory membuat Factory baru.
func NewFactory() *Factory {
	return &Factory{
		defaultLevel: LevelInfo,
	}
}

// SetDefaultLevel sets default log level untuk semua logger yang dibuat.
func (f *Factory) SetDefaultLevel(level Level) {
	f.defaultLevel = level
}

// Create membuat logger baru dengan prefix.
func (f *Factory) Create(prefix string) Logger {
	logger := NewStandardLogger(prefix)
	logger.SetLevel(f.defaultLevel)
	return logger
}

// Global factory instance
var globalFactory = NewFactory()

// GetLogger returns a logger dengan prefix yang diberikan.
func GetLogger(prefix string) Logger {
	return globalFactory.Create(prefix)
}

// SetGlobalLevel sets log level untuk semua logger yang akan dibuat.
func SetGlobalLevel(level Level) {
	globalFactory.SetDefaultLevel(level)
}

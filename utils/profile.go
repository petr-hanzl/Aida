package utils

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

func StartCPUProfile(cfg *Config) error {
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %s", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("could not start CPU profile: %s", err)
		}
	}
	return nil
}

func StopCPUProfile(cfg *Config) {
	if cfg.CPUProfile != "" {
		pprof.StopCPUProfile()
	}
}

func StartMemoryProfile(cfg *Config) error {
	// write memory profile if requested
	if cfg.MemoryProfile != "" {
		f, err := os.Create(cfg.MemoryProfile)
		if err != nil {
			return fmt.Errorf("could not create memory profile: %s", err)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			return fmt.Errorf("could not write memory profile: %s", err)
		}
	}
	return nil
}
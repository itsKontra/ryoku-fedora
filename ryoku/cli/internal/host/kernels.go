package host

import (
	"os"
	"path/filepath"
	"sort"
)

// InstalledKernels lists the kernels with a vmlinuz under modulesDir (a
// /usr/lib/modules), newest first in RPM's order.
func InstalledKernels(modulesDir string) []string {
	entries, _ := os.ReadDir(modulesDir)
	var kernels []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(modulesDir, entry.Name(), "vmlinuz")); err == nil {
			kernels = append(kernels, entry.Name())
		}
	}
	sort.Slice(kernels, func(i, j int) bool { return CompareEVR(kernels[i], kernels[j]) > 0 })
	return kernels
}

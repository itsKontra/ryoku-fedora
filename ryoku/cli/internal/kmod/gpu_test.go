package kmod

import (
	"testing"

	"ryoku-cli/internal/host"
)

func TestGPUReportsTheGenerationCheckAnswer(t *testing.T) {
	for _, tc := range []struct {
		exit int
		want GPU
	}{
		{0, GPUSupported},
		{3, GPUUnsupported},
		{4, GPUAbsent},
		{5, GPUUnknown},
		{127, GPUUnknown},
	} {
		runner := &fakeRunner{answer: func(host.Command) host.Result { return host.Result{Code: tc.exit} }}
		got := New(Config{Root: t.TempDir(), Runner: runner}).GPU()
		if got != tc.want {
			t.Errorf("exit %d: GPU() = %q, want %q", tc.exit, got, tc.want)
		}
		if len(runner.commands) != 1 || runner.commands[0] != "bash "+GPUCheckScript+" --query" {
			t.Errorf("exit %d: ran %q, want only the query", tc.exit, runner.commands)
		}
	}
}

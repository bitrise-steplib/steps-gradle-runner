package common

import (
	"sort"
	"strings"

	"github.com/bitrise-io/go-utils/v2/log"
)

// LogBenchmarkSummary emits a single consolidated log line summarizing the
// resolved benchmark phase for each active build tool. The previous per-tool
// "(i) Benchmark phase: …" + "Benchmark baseline mode: …" / warmup info lines
// were misleading on multi-tool activations (e.g. React Native, where one
// tool can be in baseline while the other is in warmup) — they made the
// build look like it was entirely in baseline even when the tool the build
// actually used was caching normally.
//
// The summary highlights which tools have the cache enabled and only uses
// the warning channel when *every* active tool is in baseline (= the build
// genuinely cannot benefit from the cache). Otherwise it logs at info level.
//
// Tools without a resolved phase (no benchmark file present, e.g. on
// non-Bitrise CI or when the BE returned an empty phase) are skipped. The
// function is a no-op when none of the requested tools have a phase.
func LogBenchmarkSummary(logger log.Logger, tools []string) {
	type entry struct {
		tool  string
		phase string
	}

	entries := make([]entry, 0, len(tools))
	anyCacheEnabled := false

	for _, tool := range tools {
		phase := ReadBenchmarkPhaseFile(tool, logger)
		if phase == "" {
			continue
		}

		entries = append(entries, entry{tool: tool, phase: phase})

		if phase != BenchmarkPhaseBaseline {
			anyCacheEnabled = true
		}
	}

	if len(entries) == 0 {
		return
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].tool < entries[j].tool })

	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, formatBenchmarkEntry(e.tool, e.phase))
	}

	summary := strings.Join(parts, ", ")

	if anyCacheEnabled {
		logger.Infof("(i) Benchmark phases: %s", summary)

		return
	}

	logger.Warnf("All active build tools in baseline benchmark mode (%s): cache disabled, analytics only", summary)
}

func formatBenchmarkEntry(tool, phase string) string {
	switch phase {
	case BenchmarkPhaseBaseline:
		return tool + "=baseline (cache disabled)"
	case BenchmarkPhaseWarmup:
		return tool + "=warmup (cache enabled, hit rate may not be ideal)"
	default:
		return tool + "=" + phase
	}
}

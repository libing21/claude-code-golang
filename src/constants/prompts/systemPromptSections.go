package prompts

import "sync"

// This is a minimal Go analog of TS systemPromptSections.ts.
// It exists mainly to preserve the "static vs dynamic section" mental model.

type systemPromptSection struct {
	name       string
	cacheBreak bool
	build      func() (string, error)
}

var (
	sectionCacheMu sync.Mutex
	sectionCache   = map[string]string{}
)

func cachedSection(name string, build func() (string, error)) systemPromptSection {
	return systemPromptSection{name: name, cacheBreak: false, build: build}
}

func uncachedSection(name string, build func() (string, error)) systemPromptSection {
	return systemPromptSection{name: name, cacheBreak: true, build: build}
}

func resolveSections(sections []systemPromptSection) ([]string, error) {
	out := make([]string, 0, len(sections))
	for _, s := range sections {
		if !s.cacheBreak {
			sectionCacheMu.Lock()
			if v, ok := sectionCache[s.name]; ok {
				sectionCacheMu.Unlock()
				if v != "" {
					out = append(out, v)
				}
				continue
			}
			sectionCacheMu.Unlock()
		}

		v, err := s.build()
		if err != nil {
			return nil, err
		}
		if !s.cacheBreak {
			sectionCacheMu.Lock()
			sectionCache[s.name] = v
			sectionCacheMu.Unlock()
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// Mirrors TS DANGEROUS_uncachedSystemPromptSection: always recompute and never cache.
// We don't store the "reason" string yet; the callsite should keep a comment.
func dangerousUncachedSection(name string, build func() (string, error)) systemPromptSection {
	return uncachedSection(name, build)
}

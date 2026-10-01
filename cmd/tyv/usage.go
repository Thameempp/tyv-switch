package main

import (
	"context"
	"sync"

	"github.com/Thameempp/tyv-switch/internal/launcher"
	"github.com/Thameempp/tyv-switch/internal/providers/antigravity"
	"github.com/Thameempp/tyv-switch/internal/selector"
)

// usageFetcher looks up quota for every profile in parallel and reports each
// result on updates. Start may be called again to refresh: it cancels any
// lookups still running from the previous round first.
type usageFetcher struct {
	l       *launcher.Launcher
	names   []string
	cache   *antigravity.Cache
	updates chan selector.Update

	agyPath string
	agyErr  error

	mu     sync.Mutex // guards cache and cancel
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newUsageFetcher(l *launcher.Launcher, names []string, cache *antigravity.Cache) *usageFetcher {
	f := &usageFetcher{
		l:       l,
		names:   names,
		cache:   cache,
		updates: make(chan selector.Update, len(names)),
	}
	f.agyPath, f.agyErr = l.FindAGY()
	return f
}

// Start begins a fresh round of lookups for all profiles.
func (f *usageFetcher) Start() {
	f.mu.Lock()
	if f.cancel != nil {
		f.cancel() // results of the previous round are no longer wanted
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.mu.Unlock()

	for i := range f.names {
		f.wg.Add(1)
		go func(i int) {
			defer f.wg.Done()
			f.fetchOne(ctx, i)
		}(i)
	}
}

// Stop cancels running lookups and waits for them to finish.
func (f *usageFetcher) Stop() {
	f.mu.Lock()
	if f.cancel != nil {
		f.cancel()
	}
	f.mu.Unlock()
	f.wg.Wait()
}

func (f *usageFetcher) fetchOne(ctx context.Context, i int) {
	name := f.names[i]
	u := selector.Update{Index: i, Gemini: selector.Unknown, Claude: selector.Unknown}
	set := func(x antigravity.Usage) {
		u.Gemini, u.Claude = x.Gemini, x.Claude
		u.GeminiReset, u.ClaudeReset = x.GeminiReset, x.ClaudeReset
	}

	if f.agyErr == nil {
		if env, err := f.l.ProfileEnv(name); err == nil {
			usage, err := antigravity.Fetch(ctx, f.agyPath, env)
			f.mu.Lock()
			if err == nil {
				set(usage)
				f.cache.Set(name, usage)
				f.cache.Save()
			} else if old, ok := f.cache.Entries[name]; ok {
				set(old.Usage) // lookup failed: show the last good reading
			}
			f.mu.Unlock()
		}
	}

	if ctx.Err() != nil {
		return // superseded by a newer round, or the picker has closed
	}
	select {
	case f.updates <- u:
	case <-ctx.Done():
	}
}

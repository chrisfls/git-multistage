package ms

import "sync"

// maxGitJobs bounds the Git processes that one command runs at the same time.
const maxGitJobs = 8

// parallel runs fns at the same time, at most maxGitJobs at once. It returns
// the error of the first fn, in argument order, that failed, as a sequence
// of the calls would.
func parallel(fns ...func() error) error {
	errs := make([]error, len(fns))
	sem := make(chan struct{}, maxGitJobs)
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

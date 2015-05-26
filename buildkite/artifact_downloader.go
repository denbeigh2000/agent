package buildkite

import (
	"github.com/buildkite/agent/buildkite/logger"
	"github.com/buildkite/agent/buildkite/pool"
	"os"
	"path/filepath"
)

type ArtifactDownloader struct {
	// The ID of the Build
	BuildID string

	// The query used to find the artifacts
	Query string

	// Which step should we look at for the jobs
	Step string

	// Where we'll be downloading artifacts to
	Destination string

	// The API used for communication
	API API
}

func (a *ArtifactDownloader) Download() error {
	// Turn the download destination into an absolute path and confirm it exists
	downloadDestination, _ := filepath.Abs(a.Destination)
	fileInfo, err := os.Stat(downloadDestination)
	if err != nil {
		logger.Fatal("Could not find information about destination: %s", downloadDestination)
	}
	if !fileInfo.IsDir() {
		logger.Fatal("%s is not a directory", downloadDestination)
	}

	// Find the artifacts that we want to download
	searcher := ArtifactSearcher{BuildID: a.BuildID, API: a.API}
	err = searcher.Search(a.Query, a.Step)
	if err != nil {
		return err
	}

	artifactCount := len(searcher.Artifacts)

	if artifactCount == 0 {
		logger.Info("No artifacts found for downloading")
	} else {
		logger.Info("Found %d artifacts. Starting to download to: %s", artifactCount, downloadDestination)

		p := pool.New(pool.MaxConcurrencyLimit)
		errors := []error{}

		for _, artifact := range searcher.Artifacts {
			// Create new instance of the artifact for the goroutine
			// See: http://golang.org/doc/effective_go.html#channels
			artifact := artifact

			p.Spawn(func() {
				err := Download{
					URL:         artifact.URL,
					Path:        artifact.Path,
					Destination: downloadDestination,
					Retries:     5,
				}.Start()

				// If the downloaded encountered an error, lock
				// the pool, collect it, then unlock the pool
				// again.
				if err != nil {
					logger.Error("Failed to download artifact: %s", err)

					p.Lock()
					errors = append(errors, err)
					p.Unlock()
				}
			})
		}

		p.Wait()

		if len(errors) > 0 {
			logger.Fatal("There were errors with downloading some of the artifacts")
		}
	}

	return nil
}
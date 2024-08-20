package plan

import (
	"context"

	log "github.com/sirupsen/logrus"
)

type BuildOptions struct {
	Yml                string
	Templater          string
	Tags               []string
	GraphWidth         int
	MatchAll           bool
	EnableDependencies bool
}

// Build plan with yml and tags/matchALL options.
func (p *Plan) Build(ctx context.Context, o BuildOptions) (err error) {
	p.templater = o.Templater

	// Create Body
	var body *planBody
	body, err = NewBody(ctx, o.Yml, false)
	if err != nil {
		return
	}
	p.body = body

	// Run hooks
	err = p.body.Lifecycle.RunPreBuild(ctx)
	if err != nil {
		return
	}

	defer func() {
		var lifecycleErr error
		for _, r := range p.body.Releases {
			lifecycle := r.Lifecycle()
			lifecycleErr = lifecycle.RunPostBuild(ctx)
			if lifecycleErr != nil {
				log.Errorf("got an error from postbuild hooks for release %s: %v", r.Name(), lifecycleErr)
				if err == nil {
					err = lifecycleErr
				}
				return
			}
		}
		lifecycleErr = p.body.Lifecycle.RunPostBuild(ctx)
		if lifecycleErr != nil {
			log.Errorf("got an error from postbuild hooks: %v", lifecycleErr)
			if err == nil {
				err = lifecycleErr
			}
			return
		}
	}()

	return p.build(ctx, o)
}

func (p *Plan) build(ctx context.Context, o BuildOptions) error {
	var err error

	log.Info("🔨 Building releases...")
	p.body.Releases, err = p.buildReleases(ctx, o)
	if err != nil {
		return err
	}

	log.Info("🔨 Building values...")
	err = p.buildValues(ctx)
	if err != nil {
		return err
	}

	log.Info("🔨 Building repositories...")
	p.body.Repositories, err = p.buildRepositories()
	if err != nil {
		return err
	}

	err = SyncRepositories(ctx, p.body.Repositories)
	if err != nil {
		return err
	}

	log.Info("🔨 Building registries...")
	p.body.Registries, err = p.buildRegistries()
	if err != nil {
		return err
	}

	err = p.syncRegistries(ctx)
	if err != nil {
		return err
	}

	log.Info("🔨 Building charts...")
	err = p.buildCharts()
	if err != nil {
		return err
	}

	// Validating plan after it was changed
	err = p.body.Validate()
	if err != nil {
		return err
	}

	log.Info("🔨 Building manifests...")
	err = p.buildManifest(ctx)
	if err != nil {
		return err
	}

	if o.GraphWidth != 1 {
		log.Info("🔨 Building graphs...")
		p.graphMD = buildGraphMD(p.body.Releases)
		log.Infof("show graph:\n%s", p.BuildGraphASCII(o.GraphWidth))
	}

	return nil
}

package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/raft-tech/konfirm/internal/utils"
	"github.com/raft-tech/konfirm/logging"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	helmcli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
)

type HelmClient interface {
	Install(ctx context.Context, config HelmReleaseParams) (*release.Release, error)
	Uninstall(ctx context.Context, release string) error
}

type HelmClientOptions struct {
	HelmDriver       string // Defaults to secrets if left empty
	ReleaseNamespace string
	RESTMapper       meta.RESTMapper
	RESTConfig       *rest.Config
	Settings         *helmcli.EnvSettings
	WorkDir          string
}

func NewHelmClient(opt HelmClientOptions) (HelmClient, error) {
	hc := &helmClient{
		config:    &helm.Configuration{},
		settings:  opt.Settings,
		namespace: opt.ReleaseNamespace,
		workDir:   opt.WorkDir,
	}
	if hc.settings == nil {
		hc.settings = helmcli.New()
	}
	if hc.workDir == "" {
		hc.workDir = os.TempDir()
	}
	if err := hc.config.Init(&restClientGetter{config: opt.RESTConfig, restMapper: opt.RESTMapper}, opt.ReleaseNamespace, opt.HelmDriver, hc.log); err != nil {
		return nil, err
	}
	return hc, nil
}

type HelmReleaseParams struct {
	ReleaseName string
	Values      map[string]any
	Chart       ChartRef
}

type ChartRef struct {
	URL      string
	Username string
	Password string
}

type helmClient struct {
	config    *helm.Configuration
	settings  *helmcli.EnvSettings
	logger    *logging.Logger
	namespace string
	workDir   string
}

func (h *helmClient) initLogger(ctx context.Context, kv ...any) {
	h.logger = logging.FromContextWithName(ctx, "helm", kv...).WithCallDepth(2)
}

func (h *helmClient) log(str string, v ...any) {
	if l := h.logger; l != nil && l.Enabled() {
		h.logger.Info(fmt.Sprintf(str, v...))
	}
}
func (h *helmClient) Install(ctx context.Context, params HelmReleaseParams) (*release.Release, error) {

	h.initLogger(ctx, "releaseNamespace", h.namespace, "release", params.ReleaseName)

	// Ensure an empty working dir is available
	h.logger.Debug("initializing release working dir")
	dir := path.Join(h.workDir, h.namespace+"-"+params.ReleaseName)
	if e := os.RemoveAll(dir); e != nil {
		return nil, utils.WrapError(e, "error cleaning release directory")
	}
	if e := os.Mkdir(dir, 0755); e != nil {
		return nil, utils.WrapError(e, "error creating release directory")
	}
	defer func() {
		if e := os.RemoveAll(dir); e != nil {
			h.logger.Error(e, "error removing working dir")
		}
	}()

	// Pull the chart
	h.logger.Debug("pulling chart")
	var cchart *chart.Chart
	if p, e := h.Pull(ctx, params.Chart, dir); e == nil {
		if cchart, e = loader.Load(p); e != nil {
			return nil, utils.WrapError(e, "error pulling chart")
		}
	} else {
		return nil, utils.WrapError(e, "error pulling chart")
	}

	// Ensure any existing releases are removed
	if err := h.Uninstall(ctx, params.ReleaseName); err != nil {
		return nil, utils.WrapError(err, "error uninstalling existing release")
	}

	// Install the release
	i := helm.NewInstall(h.config)
	i.ReleaseName = params.ReleaseName
	if params.ReleaseName == "" {
		i.GenerateName = true
	}
	i.Namespace = h.namespace
	i.Wait = true
	i.WaitForJobs = true
	i.Timeout = 300 * time.Second

	return i.RunWithContext(ctx, cchart, params.Values)
}

func (h *helmClient) Pull(ctx context.Context, ref ChartRef, dst string) (string, error) {

	dl := downloader.ChartDownloader{
		Out:     io.Discard,
		Verify:  downloader.VerifyNever,
		Getters: getter.All(h.settings),
		Options: []getter.Option{
			getter.WithBasicAuth(ref.Username, ref.Password),
		},
	}

	if out, _, err := dl.DownloadTo(ref.URL, "", dst); err == nil {
		return out, nil
	} else {
		return "", err
	}
}

func (h *helmClient) Uninstall(ctx context.Context, release string) error {
	h.initLogger(ctx, "releaseNamespace", h.namespace, "release", release)
	ui := helm.NewUninstall(h.config)
	ui.KeepHistory = false
	ui.Wait = true
	ui.Timeout = 300 * time.Second
	h.logger.Debug("uninstalling release if present")
	if res, err := ui.Run(release); err == nil {
		h.logger.Info("uninstalled release", "version", res.Release.Version)
	} else if errors.Is(err, driver.ErrReleaseNotFound) {
		h.logger.Info("release not found")
	} else {
		return utils.WrapError(err, "error uninstalling release")
	}
	return nil
}

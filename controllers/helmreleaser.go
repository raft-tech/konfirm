package controllers

import (
	"fmt"
	"github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/logging"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"os"
	"regexp"
	"strings"
)

func releaseName(stageName string, projectPath string) string {
	reg, err := regexp.Compile("[^a-zA-Z]+")
	if err != nil {
		panic(err)
	}
	splitPath := strings.Split(projectPath, "/")
	helmSafeChartName := reg.ReplaceAllString(splitPath[len(splitPath)-1], "")

	return fmt.Sprintf("%s-%s", stageName, helmSafeChartName)
}

func install(logger *logging.Logger, config v1alpha1.HelmHook) error {
	actionConfig, settings, err := initialize(logger, config.Namespace)
	if err != nil {
		return err
	}
	client := action.NewInstall(actionConfig)

	if config.Version != "" {
		client.Version = config.Version
	}
	chartPath, err := client.LocateChart(config.Path, settings)
	if err != nil {
		return err
	}

	myChart, err := loader.Load(chartPath)
	if err != nil {
		return err
	}
	client.ReleaseName = releaseName(config.Name, config.Path)
	client.Namespace = config.Namespace
	client.CreateNamespace = true
	if config.Values.Object == nil {
		logger.Info("Running helm Install with default values")
	} else {
		logger.Info("Running helm install with provided values")
	}
	_, err = client.Run(myChart, config.Values.Object)
	return err
}

func upgrade(logger *logging.Logger, config v1alpha1.HelmHook) error {
	actionConfig, settings, err := initialize(logger, config.Namespace)
	if err != nil {
		return err
	}
	client := action.NewUpgrade(actionConfig)

	if config.Version != "" {
		client.Version = config.Version
	}
	chartPath, err := client.LocateChart(config.Path, settings)
	if err != nil {
		return err
	}

	myChart, err := loader.Load(chartPath)
	if err != nil {
		return err
	}
	client.Namespace = config.Namespace
	release := releaseName(config.Name, config.Path)
	if config.Values.Object == nil {
		logger.Info("Running helm upgrade with default values")
	} else {
		logger.Info("Running helm upgrade with provided values")
	}
	_, err = client.Run(release, myChart, config.Values.Object)
	return err
}

func initialize(logger *logging.Logger, namespace string) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if namespace != "" {
		settings.SetNamespace(namespace)
	}
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), logger.Info); err != nil {
		return actionConfig, settings, err
	}
	return actionConfig, settings, nil
}

func HelmRelease(logger *logging.Logger, config v1alpha1.HelmHook) error {
	err := install(logger, config)
	if err != nil {
		logger.Info(fmt.Sprintf("Helm release %s already exsits. Performing Helm upgrade...", config.Name))
		err = upgrade(logger, config)
	}
	return err
}

func HelmDelete(logger *logging.Logger, config v1alpha1.HelmHook) error {
	actionConfig, _, err := initialize(logger, config.Namespace)
	if err != nil {
		return err
	}
	client := action.NewUninstall(actionConfig)
	release := releaseName(config.Name, config.Path)
	_, err = client.Run(release)
	return err
}

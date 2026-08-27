/*
Copyright 2026 Serge Logvinov.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package helm provides a wrapper around the Helm SDK for managing Helm releases in Kubernetes.
package helm

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
	helmv1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Client wraps the Helm SDK client.
type Client struct {
	actionClient *action.Configuration
}

// NewHelmClient creates a new HelmClient from a RESTClientGetter.
func NewHelmClient(kclient *k8s.Client, namespace string) (*Client, error) {
	origFlags := kclient.ToRawKubeConfigLoader()
	configFlags := &genericclioptions.ConfigFlags{
		KubeConfig: origFlags.KubeConfig,
		Namespace:  &namespace,
		Context:    origFlags.Context,
		// Global overrides that apply to every context.
		Impersonate:      origFlags.Impersonate,
		ImpersonateGroup: origFlags.ImpersonateGroup,
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(configFlags, namespace, os.Getenv("HELM_DRIVER")); err != nil {
		return nil, fmt.Errorf("initialize helm: %w", err)
	}

	return &Client{actionClient: actionConfig}, nil
}

// ListReleases lists Helm releases in a namespace.
func (c *Client) ListReleases(namespace, labelSelector, statusFilter string) ([]helmv1.Release, error) {
	listCmd := action.NewList(c.actionClient)
	listCmd.Selector = labelSelector
	listCmd.AllNamespaces = namespace == ""

	switch statusFilter {
	case "deployed":
		listCmd.Deployed = true
	case "failed":
		listCmd.Failed = true
	default:
		listCmd.All = true
	}
	listCmd.SetStateMask()

	res, err := listCmd.Run()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("no releases found: %w", err)
		}
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var releases []helmv1.Release
	for _, r := range res {
		rel, ok := r.(*helmv1.Release)
		if !ok {
			continue
		}

		releases = append(releases, *rel)
	}

	return releases, nil
}

// GetRelease gets a single Helm release by name.
func (c *Client) GetRelease(name, namespace string) (*helmv1.Release, error) {
	getCmd := action.NewGet(c.actionClient)

	release, err := getCmd.Run(name)
	if err != nil {
		return nil, fmt.Errorf("release '%s' not found in namespace '%s'", name, namespace)
	}

	// Cast to *helmv1.Release to access fields
	rel, ok := release.(*helmv1.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type")
	}

	return rel, nil
}

// GetReleaseResources gets a single Helm release by name and its resources.
func (c *Client) GetReleaseResources(rel *helmv1.Release) (ResourceList, error) {
	list := ResourceList{}

	resources, err := c.actionClient.KubeClient.Build(bytes.NewBufferString(rel.Manifest), false)
	if err != nil {
		return ResourceList{}, fmt.Errorf("failed to build resources for release: %w", err)
	}

	for _, k := range resources {
		selector, _, _ := getSelectorFromObject(k.Object) //nolint:errcheck
		list = append(list, Resource{
			Name:     fmt.Sprintf("%s/%s", k.Mapping.GroupVersionKind.Kind, k.Name),
			Selector: selector,
		})
	}

	return list, nil
}

// GetReleaseHistory gets the history of a Helm release.
func (c *Client) GetReleaseHistory(name, namespace string, maxRevisions int) ([]HistoryEntry, error) {
	historyCmd := action.NewHistory(c.actionClient)
	historyCmd.Max = maxRevisions

	revisions, err := historyCmd.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get history for release '%s' in namespace '%s': %w", name, namespace, err)
	}

	var history []HistoryEntry
	for _, r := range revisions {
		// Cast to *helmv1.Release to access fields
		rel, ok := r.(*helmv1.Release)
		if !ok {
			continue
		}
		history = append(history, HistoryEntry{
			Revision:     rel.Version,
			Updated:      rel.Info.LastDeployed.String(),
			Status:       rel.Info.Status.String(),
			ChartVersion: rel.Chart.Metadata.Version,
			AppVersion:   rel.Chart.Metadata.AppVersion,
			Description:  rel.Info.Description,
		})
	}

	return history, nil
}

// Rollback rolls back a Helm release to a specific revision.
func (c *Client) Rollback(name string, revision int, hooks bool) error {
	rollbackCmd := action.NewRollback(c.actionClient)
	rollbackCmd.Version = revision
	rollbackCmd.WaitStrategy = kube.HookOnlyStrategy
	rollbackCmd.DisableHooks = !hooks
	rollbackCmd.WaitForJobs = hooks

	return rollbackCmd.Run(name)
}

// UpdateStatus updates the status of a Helm release.
func (c *Client) UpdateStatus(rls release.Releaser) error {
	return c.actionClient.Releases.Update(rls)
}

func getSelectorFromObject(obj runtime.Object) (map[string]string, bool, error) {
	typed := obj.(*unstructured.Unstructured)
	kind := typed.Object["kind"]
	switch kind {
	case "ReplicaSet", "Deployment", "StatefulSet", "DaemonSet", "Job":
		return unstructured.NestedStringMap(typed.Object, "spec", "selector", "matchLabels")
	case "ReplicationController":
		return unstructured.NestedStringMap(typed.Object, "spec", "selector")
	default:
		return nil, false, nil
	}
}

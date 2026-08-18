// Package helm provides a wrapper around the Helm SDK for managing Helm releases in Kubernetes.
package helm

import (
	"fmt"
	"os"
	"strings"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"helm.sh/helm/v4/pkg/action"
	v1 "helm.sh/helm/v4/pkg/release/v1"
)

// Client wraps the Helm SDK client.
type Client struct {
	actionClient *action.Configuration
}

// NewHelmClient creates a new HelmClient from a RESTClientGetter.
func NewHelmClient(kclient *k8s.Client, namespace string) (*Client, error) {
	configFlags := kclient.ToRawKubeConfigLoader()

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(configFlags, namespace, os.Getenv("HELM_DRIVER")); err != nil {
		return nil, fmt.Errorf("initialize helm: %w", err)
	}

	return &Client{actionClient: actionConfig}, nil
}

// ListReleases lists Helm releases in a namespace.
func (c *Client) ListReleases(namespace, labelSelector, statusFilter string) ([]ReleaseSummary, error) {
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

	releases, err := listCmd.Run()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("no releases found: %w", err)
		}
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var summaries []ReleaseSummary
	for _, r := range releases {
		rel, ok := r.(*v1.Release)
		if !ok {
			continue
		}

		summaries = append(summaries, ReleaseSummary{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			Revision:     rel.Version,
			Updated:      rel.Info.LastDeployed.String(),
			Status:       rel.Info.Status.String(),
			ChartName:    rel.Chart.Metadata.Name,
			ChartVersion: rel.Chart.Metadata.Version,
			AppVersion:   rel.Chart.Metadata.AppVersion,
		})
	}

	return summaries, nil
}

// GetRelease gets a single Helm release by name.
func (c *Client) GetRelease(name, namespace string) (*ReleaseStatus, error) {
	getCmd := action.NewGet(c.actionClient)

	release, err := getCmd.Run(name)
	if err != nil {
		return nil, fmt.Errorf("release '%s' not found in namespace '%s'", name, namespace)
	}

	// Cast to *v1.Release to access fields
	rel, ok := release.(*v1.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type")
	}

	return &ReleaseStatus{
		Name:         rel.Name,
		Namespace:    rel.Namespace,
		Revision:     rel.Version,
		Status:       rel.Info.Status.String(),
		LastDeployed: rel.Info.LastDeployed.String(),
		Description:  rel.Info.Description,
	}, nil
}

// GetReleaseHistory gets the history of a Helm release (last 3 revisions).
func (c *Client) GetReleaseHistory(name, namespace string, maxRevisions int) ([]HistoryEntry, error) {
	historyCmd := action.NewHistory(c.actionClient)
	historyCmd.Max = maxRevisions

	revisions, err := historyCmd.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get history for release '%s' in namespace '%s': %w", name, namespace, err)
	}

	var history []HistoryEntry
	for _, r := range revisions {
		// Cast to *v1.Release to access fields
		rel, ok := r.(*v1.Release)
		if !ok {
			continue
		}
		history = append(history, HistoryEntry{
			Revision:    rel.Version,
			Updated:     rel.Info.LastDeployed.String(),
			Status:      rel.Info.Status.String(),
			Chart:       rel.Chart.Metadata.Name + "-" + rel.Chart.Metadata.Version,
			AppVersion:  rel.Chart.Metadata.AppVersion,
			Description: rel.Info.Description,
		})
	}

	return history, nil
}

// Rollback rolls back a Helm release to a specific revision.
func (c *Client) Rollback(name string, revision int) error {
	rollbackCmd := action.NewRollback(c.actionClient)
	rollbackCmd.Version = revision

	return rollbackCmd.Run(name)
}

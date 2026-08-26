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

package toolsfluxcd

import (
	"context"
	"fmt"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileRequestAnnotation is the annotation Flux controllers watch to
// trigger an immediate reconciliation, matching `flux reconcile`.
const reconcileRequestAnnotation = meta.ReconcileRequestAnnotation

// Client wraps a controller-runtime client bound to the Flux API types.
type Client struct {
	client client.Client
}

// NewFluxClient creates a Flux client from the resolved identity of the
// given k8s.Client, so authentication, impersonation, and per-cluster
// targeting stay consistent with every other tool.
func NewFluxClient(kclient *k8s.Client) (*Client, error) {
	scheme := runtime.NewScheme()
	if err := sourcev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register flux source scheme: %w", err)
	}
	if err := helmv2.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register flux helm scheme: %w", err)
	}
	if err := kustomizev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register flux kustomize scheme: %w", err)
	}

	restCfg := kclient.RESTConfig()
	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create flux client: %w", err)
	}

	return &Client{client: c}, nil
}

// ListGitRepositories lists GitRepository resources, optionally scoped to a
// namespace and filtered by label/field selectors.
func (c *Client) ListGitRepositories(ctx context.Context, namespace, labelSelector, fieldSelector string) ([]SourceSummary, error) {
	list := &sourcev1.GitRepositoryList{}
	opts, err := listOptions(namespace, labelSelector, fieldSelector)
	if err != nil {
		return nil, err
	}
	if err := c.client.List(ctx, list, opts...); err != nil {
		return nil, listError("GitRepository", err)
	}

	summaries := make([]SourceSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, gitRepositorySummary(&list.Items[i]))
	}
	return summaries, nil
}

// ListOCIRepositories lists OCIRepository resources, optionally scoped to a
// namespace and filtered by label/field selectors.
func (c *Client) ListOCIRepositories(ctx context.Context, namespace, labelSelector, fieldSelector string) ([]SourceSummary, error) {
	list := &sourcev1.OCIRepositoryList{}
	opts, err := listOptions(namespace, labelSelector, fieldSelector)
	if err != nil {
		return nil, err
	}
	if err := c.client.List(ctx, list, opts...); err != nil {
		return nil, listError("OCIRepository", err)
	}

	summaries := make([]SourceSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, ociRepositorySummary(&list.Items[i]))
	}
	return summaries, nil
}

// ListHelmReleases lists HelmRelease resources, optionally scoped to a
// namespace and filtered by label/field selectors.
func (c *Client) ListHelmReleases(ctx context.Context, namespace, labelSelector, fieldSelector string) ([]HelmReleaseSummary, error) {
	list := &helmv2.HelmReleaseList{}
	opts, err := listOptions(namespace, labelSelector, fieldSelector)
	if err != nil {
		return nil, err
	}
	if err := c.client.List(ctx, list, opts...); err != nil {
		return nil, listError("HelmRelease", err)
	}

	summaries := make([]HelmReleaseSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, helmReleaseSummary(&list.Items[i]))
	}
	return summaries, nil
}

// ListKustomizations lists Kustomization resources, optionally scoped to a
// namespace and filtered by label/field selectors.
func (c *Client) ListKustomizations(ctx context.Context, namespace, labelSelector, fieldSelector string) ([]KustomizationSummary, error) {
	list := &kustomizev1.KustomizationList{}
	opts, err := listOptions(namespace, labelSelector, fieldSelector)
	if err != nil {
		return nil, err
	}
	if err := c.client.List(ctx, list, opts...); err != nil {
		return nil, listError("Kustomization", err)
	}

	summaries := make([]KustomizationSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, kustomizationSummary(&list.Items[i]))
	}
	return summaries, nil
}

// DescribeGitRepository fetches a single GitRepository by name.
func (c *Client) DescribeGitRepository(ctx context.Context, name, namespace string) (*GitRepositoryDescribeResult, error) {
	obj := &sourcev1.GitRepository{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("GitRepository", name, namespace, err)
	}

	result := &GitRepositoryDescribeResult{
		SourceSummary:          gitRepositorySummary(obj),
		Labels:                 obj.Labels,
		Annotations:            obj.Annotations,
		Conditions:             conditionInfos(obj.Status.Conditions),
		Interval:               obj.Spec.Interval.Duration.String(),
		Ref:                    gitRef(obj.Spec.Reference),
		LastHandledReconcileAt: obj.Status.GetLastHandledReconcileRequest(),
	}
	if obj.Spec.Timeout != nil {
		result.Timeout = obj.Spec.Timeout.Duration.String()
	}
	if obj.Status.Artifact != nil {
		result.Artifact = obj.Status.Artifact.Path
	}

	return result, nil
}

// DescribeOCIRepository fetches a single OCIRepository by name.
func (c *Client) DescribeOCIRepository(ctx context.Context, name, namespace string) (*OCIRepositoryDescribeResult, error) {
	obj := &sourcev1.OCIRepository{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("OCIRepository", name, namespace, err)
	}

	result := &OCIRepositoryDescribeResult{
		SourceSummary:          ociRepositorySummary(obj),
		Labels:                 obj.Labels,
		Annotations:            obj.Annotations,
		Conditions:             conditionInfos(obj.Status.Conditions),
		Interval:               obj.Spec.Interval.Duration.String(),
		Provider:               obj.Spec.Provider,
		LastHandledReconcileAt: obj.Status.GetLastHandledReconcileRequest(),
	}
	if obj.Spec.Timeout != nil {
		result.Timeout = obj.Spec.Timeout.Duration.String()
	}
	if obj.Spec.Reference != nil {
		result.Digest = obj.Spec.Reference.Digest
	}
	if obj.Spec.LayerSelector != nil {
		result.LayerSelector = strings.TrimPrefix(obj.Spec.LayerSelector.MediaType+"/"+obj.Spec.LayerSelector.Operation, "/")
	}
	if obj.Status.Artifact != nil {
		result.Artifact = obj.Status.Artifact.Path
	}

	return result, nil
}

// DescribeHelmRelease fetches a single HelmRelease by name.
func (c *Client) DescribeHelmRelease(ctx context.Context, name, namespace string) (*HelmReleaseDescribeResult, error) {
	obj := &helmv2.HelmRelease{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("HelmRelease", name, namespace, err)
	}

	result := &HelmReleaseDescribeResult{
		HelmReleaseSummary:    helmReleaseSummary(obj),
		Labels:                obj.Labels,
		Annotations:           obj.Annotations,
		Conditions:            conditionInfos(obj.Status.Conditions),
		LastAttemptedRevision: obj.Status.LastAttemptedRevision,
		LastAttemptedValues:   obj.Status.LastAttemptedConfigDigest, //nolint:staticcheck // checksum field is deprecated, digest is the current equivalent
	}

	if obj.Spec.Chart != nil {
		result.Chart = obj.Spec.Chart.Spec.Chart
		if obj.Spec.Chart.Spec.Version != "" {
			result.Chart = fmt.Sprintf("%s:%s", result.Chart, obj.Spec.Chart.Spec.Version)
		}
		result.SourceRef = fmt.Sprintf("%s/%s", obj.Spec.Chart.Spec.SourceRef.Kind, obj.Spec.Chart.Spec.SourceRef.Name)
		if obj.Spec.Chart.Spec.SourceRef.Namespace != "" {
			result.SourceRef = fmt.Sprintf("%s/%s/%s", obj.Spec.Chart.Spec.SourceRef.Kind, obj.Spec.Chart.Spec.SourceRef.Namespace, obj.Spec.Chart.Spec.SourceRef.Name)
		}
	}
	if obj.Spec.ChartRef != nil {
		result.SourceRef = fmt.Sprintf("%s/%s", obj.Spec.ChartRef.Kind, obj.Spec.ChartRef.Name)
		if obj.Spec.ChartRef.Namespace != "" {
			result.SourceRef = fmt.Sprintf("%s/%s/%s", obj.Spec.ChartRef.Kind, obj.Spec.ChartRef.Namespace, obj.Spec.ChartRef.Name)
		}
	}
	if obj.Spec.Values != nil {
		result.Values = string(obj.Spec.Values.Raw)
	}

	return result, nil
}

// DescribeKustomization fetches a single Kustomization by name.
func (c *Client) DescribeKustomization(ctx context.Context, name, namespace string) (*KustomizationDescribeResult, error) {
	obj := &kustomizev1.Kustomization{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("Kustomization", name, namespace, err)
	}

	result := &KustomizationDescribeResult{
		KustomizationSummary:  kustomizationSummary(obj),
		Labels:                obj.Labels,
		Annotations:           obj.Annotations,
		Conditions:            conditionInfos(obj.Status.Conditions),
		Path:                  obj.Spec.Path,
		Prune:                 obj.Spec.Prune,
		LastAppliedRevision:   obj.Status.LastAppliedRevision,
		LastAttemptedRevision: obj.Status.LastAttemptedRevision,
	}
	if obj.Spec.SourceRef.Name != "" {
		result.SourceRef = fmt.Sprintf("%s/%s", obj.Spec.SourceRef.Kind, obj.Spec.SourceRef.Name)
		if obj.Spec.SourceRef.Namespace != "" {
			result.SourceRef = fmt.Sprintf("%s/%s/%s", obj.Spec.SourceRef.Kind, obj.Spec.SourceRef.Namespace, obj.Spec.SourceRef.Name)
		}
	}

	return result, nil
}

// Reconcile triggers an immediate reconciliation of the named resource by
// writing the reconcile.fluxcd.io/requestedAt annotation, exactly like
// `flux reconcile`. When withSource is set on a HelmRelease/Kustomization,
// the referenced source is reconciled too (best-effort).
func (c *Client) Reconcile(ctx context.Context, kind, name, namespace string, withSource bool) (*ReconcileResult, error) {
	switch strings.ToLower(kind) {
	case "gitrepository":
		return c.reconcileGitRepository(ctx, name, namespace)
	case "helmrelease":
		return c.reconcileHelmRelease(ctx, name, namespace, withSource)
	case "kustomization":
		return c.reconcileKustomization(ctx, name, namespace, withSource)
	default:
		return nil, fmt.Errorf("invalid parameter 'kind': must be one of gitrepository, helmrelease, kustomization")
	}
}

// Suspend suspends a HelmRelease or Kustomization by setting spec.suspend=true.
func (c *Client) Suspend(ctx context.Context, kind, name, namespace string) (*ReconciliationResult, error) {
	return c.setSuspended(ctx, kind, name, namespace, true)
}

// Resume resumes a HelmRelease or Kustomization by setting spec.suspend=false.
func (c *Client) Resume(ctx context.Context, kind, name, namespace string) (*ReconciliationResult, error) {
	return c.setSuspended(ctx, kind, name, namespace, false)
}

func (c *Client) reconcileGitRepository(ctx context.Context, name, namespace string) (*ReconcileResult, error) {
	obj := &sourcev1.GitRepository{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("GitRepository", name, namespace, err)
	}

	requestedAt := requestedAtTimestamp()
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[reconcileRequestAnnotation] = requestedAt

	if err := c.client.Update(ctx, obj); err != nil {
		return nil, fmt.Errorf("failed to reconcile GitRepository '%s' in namespace '%s': %w", name, namespace, err)
	}

	ready, message := readyCondition(obj.Status.Conditions)

	return &ReconcileResult{
		Kind:        "GitRepository",
		Namespace:   namespace,
		Name:        name,
		RequestedAt: requestedAt,
		Ready:       ready,
		Message:     message,
	}, nil
}

func (c *Client) reconcileHelmRelease(ctx context.Context, name, namespace string, withSource bool) (*ReconcileResult, error) {
	obj := &helmv2.HelmRelease{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("HelmRelease", name, namespace, err)
	}

	requestedAt := requestedAtTimestamp()
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[reconcileRequestAnnotation] = requestedAt

	if err := c.client.Update(ctx, obj); err != nil {
		return nil, fmt.Errorf("failed to reconcile HelmRelease '%s' in namespace '%s': %w", name, namespace, err)
	}

	ready, message := readyCondition(obj.Status.Conditions)
	result := &ReconcileResult{
		Kind:        "HelmRelease",
		Namespace:   namespace,
		Name:        name,
		RequestedAt: requestedAt,
		Ready:       ready,
		Message:     message,
	}

	if withSource {
		ref := sourceRef{}
		if obj.Spec.Chart != nil && obj.Spec.Chart.Spec.SourceRef.Name != "" {
			ref = sourceRef{
				Kind:      obj.Spec.Chart.Spec.SourceRef.Kind,
				Name:      obj.Spec.Chart.Spec.SourceRef.Name,
				Namespace: obj.Spec.Chart.Spec.SourceRef.Namespace,
			}
		} else if obj.Spec.ChartRef != nil && obj.Spec.ChartRef.Name != "" {
			ref = sourceRef{
				Kind:      obj.Spec.ChartRef.Kind,
				Name:      obj.Spec.ChartRef.Name,
				Namespace: obj.Spec.ChartRef.Namespace,
			}
		}
		if err := c.reconcileSourceRef(ctx, namespace, ref); err != nil {
			result.Message = strings.TrimSpace(fmt.Sprintf("%s; source reconcile failed: %v", result.Message, err))
		} else {
			result.SourceReconciled = true
		}
	}

	return result, nil
}

func (c *Client) reconcileKustomization(ctx context.Context, name, namespace string, withSource bool) (*ReconcileResult, error) {
	obj := &kustomizev1.Kustomization{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, getError("Kustomization", name, namespace, err)
	}

	requestedAt := requestedAtTimestamp()
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[reconcileRequestAnnotation] = requestedAt

	if err := c.client.Update(ctx, obj); err != nil {
		return nil, fmt.Errorf("failed to reconcile Kustomization '%s' in namespace '%s': %w", name, namespace, err)
	}

	ready, message := readyCondition(obj.Status.Conditions)
	result := &ReconcileResult{
		Kind:        "Kustomization",
		Namespace:   namespace,
		Name:        name,
		RequestedAt: requestedAt,
		Ready:       ready,
		Message:     message,
	}

	if withSource {
		ref := sourceRef{}
		if obj.Spec.SourceRef.Name != "" {
			ref = sourceRef{
				Kind:      obj.Spec.SourceRef.Kind,
				Name:      obj.Spec.SourceRef.Name,
				Namespace: obj.Spec.SourceRef.Namespace,
			}
		}
		if err := c.reconcileSourceRef(ctx, namespace, ref); err != nil {
			result.Message = strings.TrimSpace(fmt.Sprintf("%s; source reconcile failed: %v", result.Message, err))
		} else {
			result.SourceReconciled = true
		}
	}

	return result, nil
}

// sourceRef is the minimal source reference needed to reconcile a source.
type sourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

// reconcileSourceRef reconciles the GitRepository/OCIRepository referenced by
// a HelmRelease chart template/chartRef or a Kustomization sourceRef.
func (c *Client) reconcileSourceRef(ctx context.Context, defaultNamespace string, ref sourceRef) error {
	if ref.Name == "" {
		return fmt.Errorf("no source reference found")
	}

	kind, name := ref.Kind, ref.Name
	namespace := ref.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}

	requestedAt := requestedAtTimestamp()

	switch kind {
	case "GitRepository":
		obj := &sourcev1.GitRepository{}
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return fmt.Errorf("GitRepository '%s' in namespace '%s': %w", name, namespace, err)
		}
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		obj.Annotations[reconcileRequestAnnotation] = requestedAt
		return c.client.Update(ctx, obj)
	case "OCIRepository":
		obj := &sourcev1.OCIRepository{}
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return fmt.Errorf("OCIRepository '%s' in namespace '%s': %w", name, namespace, err)
		}
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		obj.Annotations[reconcileRequestAnnotation] = requestedAt
		return c.client.Update(ctx, obj)
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
}

func (c *Client) setSuspended(ctx context.Context, kind, name, namespace string, suspended bool) (*ReconciliationResult, error) {
	action := "resume"
	if suspended {
		action = "suspend"
	}

	switch strings.ToLower(kind) {
	case "helmrelease":
		obj := &helmv2.HelmRelease{}
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return nil, getError("HelmRelease", name, namespace, err)
		}
		obj.Spec.Suspend = suspended
		if err := c.client.Update(ctx, obj); err != nil {
			return nil, fmt.Errorf("failed to %s HelmRelease '%s' in namespace '%s': %w", action, name, namespace, err)
		}
	case "kustomization":
		obj := &kustomizev1.Kustomization{}
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return nil, getError("Kustomization", name, namespace, err)
		}
		obj.Spec.Suspend = suspended
		if err := c.client.Update(ctx, obj); err != nil {
			return nil, fmt.Errorf("failed to %s Kustomization '%s' in namespace '%s': %w", action, name, namespace, err)
		}
	default:
		return nil, fmt.Errorf("invalid parameter 'kind': must be one of helmrelease, kustomization")
	}

	return &ReconciliationResult{
		Kind:      kindTitle(kind),
		Namespace: namespace,
		Name:      name,
		Action:    action,
		Suspended: suspended,
	}, nil
}

func listOptions(namespace, labelSelector, fieldSelector string) ([]client.ListOption, error) {
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if labelSelector != "" {
		selector, err := labels.Parse(labelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector %q: %w", labelSelector, err)
		}
		opts = append(opts, client.MatchingLabelsSelector{Selector: selector})
	}
	if fieldSelector != "" {
		selector, err := fields.ParseSelector(fieldSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid field selector %q: %w", fieldSelector, err)
		}
		opts = append(opts, client.MatchingFieldsSelector{Selector: selector})
	}
	return opts, nil
}

func gitRepositorySummary(obj *sourcev1.GitRepository) SourceSummary {
	summary := SourceSummary{
		Namespace: obj.Namespace,
		Name:      obj.Name,
		URL:       obj.Spec.URL,
		Age:       formatFluxAge(obj.CreationTimestamp),
	}

	if obj.Status.Artifact != nil {
		summary.Revision = obj.Status.Artifact.Revision
		summary.LastAppliedAt = obj.Status.Artifact.LastUpdateTime.UTC().Format(time.RFC3339)
	}
	summary.Ready, summary.Message = readyCondition(obj.Status.Conditions)

	return summary
}

func ociRepositorySummary(obj *sourcev1.OCIRepository) SourceSummary {
	summary := SourceSummary{
		Namespace: obj.Namespace,
		Name:      obj.Name,
		URL:       obj.Spec.URL,
		Age:       formatFluxAge(obj.CreationTimestamp),
	}

	if obj.Status.Artifact != nil {
		summary.Revision = obj.Status.Artifact.Revision
		summary.LastAppliedAt = obj.Status.Artifact.LastUpdateTime.UTC().Format(time.RFC3339)
	}
	summary.Ready, summary.Message = readyCondition(obj.Status.Conditions)

	return summary
}

func helmReleaseSummary(obj *helmv2.HelmRelease) HelmReleaseSummary {
	summary := HelmReleaseSummary{
		Namespace: obj.Namespace,
		Name:      obj.Name,
		Revision:  obj.Status.LastAttemptedRevision,
		Age:       formatFluxAge(obj.CreationTimestamp),
	}

	if obj.Spec.Chart != nil && obj.Spec.Chart.Spec.Version != "" {
		summary.Version = obj.Spec.Chart.Spec.Version
	}
	summary.Ready, summary.Message = readyCondition(obj.Status.Conditions)

	return summary
}

func kustomizationSummary(obj *kustomizev1.Kustomization) KustomizationSummary {
	summary := KustomizationSummary{
		Namespace: obj.Namespace,
		Name:      obj.Name,
		Revision:  obj.Status.LastAppliedRevision,
		Age:       formatFluxAge(obj.CreationTimestamp),
	}

	summary.Ready, summary.Message = readyCondition(obj.Status.Conditions)

	return summary
}

// readyCondition extracts the Ready condition from a Flux status.
func readyCondition(conditions []metav1.Condition) (bool, string) {
	for i := range conditions {
		if conditions[i].Type == "Ready" {
			return conditions[i].Status == metav1.ConditionTrue, conditions[i].Message
		}
	}
	return false, ""
}

func conditionInfos(conditions []metav1.Condition) []ConditionInfo {
	if len(conditions) == 0 {
		return nil
	}

	infos := make([]ConditionInfo, 0, len(conditions))
	for i := range conditions {
		infos = append(infos, ConditionInfo{
			Type:    conditions[i].Type,
			Status:  string(conditions[i].Status),
			Reason:  conditions[i].Reason,
			Message: conditions[i].Message,
		})
	}
	return infos
}

// gitRef renders a GitRepositoryRef as a human-readable string.
func gitRef(ref *sourcev1.GitRepositoryRef) string {
	if ref == nil {
		return ""
	}

	switch {
	case ref.Commit != "":
		return fmt.Sprintf("commit/%s", ref.Commit)
	case ref.Name != "":
		return fmt.Sprintf("ref/%s", ref.Name)
	case ref.SemVer != "":
		return fmt.Sprintf("semver/%s", ref.SemVer)
	case ref.Tag != "":
		return fmt.Sprintf("tag/%s", ref.Tag)
	case ref.Branch != "":
		return fmt.Sprintf("branch/%s", ref.Branch)
	default:
		return ""
	}
}

func formatFluxAge(created metav1.Time) string {
	diff := time.Since(created.Time)

	switch {
	case diff < time.Minute:
		return "0s"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

// kindTitle returns the display form of a lower-cased kind string
// ("helmrelease" -> "HelmRelease", "kustomization" -> "Kustomization").
func kindTitle(kind string) string {
	switch strings.ToLower(kind) {
	case "helmrelease":
		return "HelmRelease"
	case "kustomization":
		return "Kustomization"
	default:
		return kind
	}
}

func listError(kind string, err error) error {
	if apierrors.IsNotFound(err) || isNoMatchError(err) {
		return fmt.Errorf("flux CRDs not installed in cluster: %w", err)
	}
	return fmt.Errorf("failed to list %s resources: %w", kind, err)
}

func getError(kind, name, namespace string, err error) error {
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("%s '%s' not found in namespace '%s'", kind, name, namespace)
	}
	if isNoMatchError(err) {
		return fmt.Errorf("flux CRDs not installed in cluster: %w", err)
	}
	return fmt.Errorf("failed to get %s '%s' in namespace '%s': %w", kind, name, namespace, err)
}

// isNoMatchError reports whether the error is a "no matches for kind" API
// error, which means the Flux CRDs are not served by the cluster.
func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no matches for kind") || strings.Contains(err.Error(), "the server could not find the requested resource")
}

package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Config holds Kubernetes client configuration.
type Config struct {
	Kubeconfig string
	Context    string
	Namespace  string
}

// Client is a Kubernetes clientset plus the resolved identity of the active
// context/cluster/namespace, so callers can report *what* they are talking to.
type Client struct {
	kubernetes.Interface

	// ContextName is the resolved active context (from --context or current-context).
	ContextName string
	// ClusterName is the cluster the active context points to.
	ClusterName string
	// Namespace is the effective namespace: --namespace > kubeconfig context namespace > "default".
	Namespace string
	// User is the resolved AuthInfo (user) of the active context, including impersonation.
	User UserInfo
}

// UserInfo describes the authenticated identity used by the active context.
type UserInfo struct {
	// Name is the kubeconfig user name the active context references.
	Name string
	// Username is the basic-auth username, if any.
	Username string
	// Impersonate is the user to impersonate (kubeconfig AuthInfo.Impersonate), if any.
	Impersonate string
	// ImpersonateGroups are the groups to impersonate, if any.
	ImpersonateGroups []string
	// HasToken indicates whether the context authenticates via a token.
	HasToken bool
}

// NewClient builds a typed Kubernetes client from the supplied config, resolving
// the active context, cluster, and namespace via k8s.io/client-go/tools/clientcmd.
func NewClient(cfg *Config) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = cfg.Kubeconfig

	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: cfg.Context,
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build REST config: %w", err)
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	resolved, err := resolveContext(loadingRules, cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		Interface:   clientSet,
		ContextName: resolved.context,
		ClusterName: resolved.cluster,
		Namespace:   resolved.namespace,
		User:        resolved.user,
	}, nil
}

type resolvedIdentity struct {
	context   string
	cluster   string
	namespace string
	user      UserInfo
}

// resolveContext loads the merged kubeconfig and extracts the active context's
// cluster and default namespace, honoring overrides from flags.
func resolveContext(loadingRules *clientcmd.ClientConfigLoadingRules, cfg *Config) (*resolvedIdentity, error) {
	raw, err := loadingRules.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	contextName := cfg.Context
	if contextName == "" {
		contextName = raw.CurrentContext
	}

	namespace := cfg.Namespace
	cluster := ""
	user := UserInfo{}

	if ctx, ok := raw.Contexts[contextName]; ok {
		cluster = ctx.Cluster
		if namespace == "" {
			namespace = ctx.Namespace
		}

		if auth, ok := raw.AuthInfos[ctx.AuthInfo]; ok {
			user = UserInfo{
				Name:              ctx.AuthInfo,
				Username:          auth.Username,
				Impersonate:       auth.Impersonate,
				ImpersonateGroups: auth.ImpersonateGroups,
				HasToken:          auth.Token != "" || auth.TokenFile != "",
			}
		}
	}

	if namespace == "" {
		namespace = "default"
	}

	return &resolvedIdentity{
		context:   contextName,
		cluster:   cluster,
		namespace: namespace,
		user:      user,
	}, nil
}

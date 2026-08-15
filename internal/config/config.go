package config

type Config struct {
	Kubeconfig       string
	Context          string
	Namespace        string
	Port             int
	AllowDestructive bool
	LogLevel         string
}

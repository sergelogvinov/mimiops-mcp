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

// Package config defines the configuration structure for the mimiops-mcp server.
package config

import "k8s.io/cli-runtime/pkg/genericclioptions"

// Config holds the configuration for the mimiops-mcp server.
type Config struct {
	ConfigFlags      *genericclioptions.ConfigFlags
	Port             int
	AllowDestructive bool
	LogLevel         string
	LogFormat        string
}

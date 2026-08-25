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

package tools

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// toServiceSummary converts a service to a ServiceSummary.
func toServiceSummary(svc *corev1.Service) ServiceSummary {
	ports := make([]PortInfo, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		port := p.Port
		if p.NodePort != 0 {
			port = p.NodePort
		}

		portInfo := PortInfo{
			Name: p.Name,
			Port: fmt.Sprintf("%d/%s", port, p.Protocol),
		}
		ports = append(ports, portInfo)
	}

	selector := formatMatchLabels(svc.Spec.Selector)

	var externalIP string
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			ingresses := make([]string, 0, len(svc.Status.LoadBalancer.Ingress))
			for _, ing := range svc.Status.LoadBalancer.Ingress {
				if ing.IP != "" {
					ingresses = append(ingresses, ing.IP)
				}
				if ing.Hostname != "" {
					ingresses = append(ingresses, ing.Hostname)
				}
			}
			externalIP = strings.Join(ingresses, ",")
		}
	}

	return ServiceSummary{
		Name:       svc.Name,
		Namespace:  svc.Namespace,
		Type:       string(svc.Spec.Type),
		ClusterIP:  svc.Spec.ClusterIP,
		ExternalIP: externalIP,
		Ports:      ports,
		Selector:   selector,
		Age:        formatAge(svc.CreationTimestamp),
	}
}

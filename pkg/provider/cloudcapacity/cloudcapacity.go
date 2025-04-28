/*
Copyright 2025 The Kubernetes Authors.

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

package cloudcapacity

// import (
// 	"context"
// 	"fmt"

// 	"github.com/bizflycloud/gobizfly"
// 	"github.com/go-logr/logr"
// 	corev1 "k8s.io/api/core/v1"
// 	"k8s.io/apimachinery/pkg/api/resource"
// )

// type Provider struct {
// 	log           logr.Logger
// 	bizflyClient  *gobizfly.Client
// 	region        string
// 	capacityZones map[string]NodeCapacity
// }

// type NodeCapacity struct {
// 	Name string
// 	// Capacity is the total amount of resources available on the node.
// 	Capacity corev1.ResourceList
// 	// Overhead is the amount of resource overhead expected to be used by Proxmox host.
// 	Overhead corev1.ResourceList
// 	// Allocatable is the total amount of resources available to the VMs.
// 	Allocatable corev1.ResourceList
// }

// func NewProvider(log logr.Logger, bizflyClient *gobizfly.Client, region string) *Provider {
// 	return &Provider{
// 		log:           log,
// 		bizflyClient:  bizflyClient,
// 		region:        region,
// 		capacityZones: make(map[string]NodeCapacity),
// 	}
// }

// func (p *Provider) Sync(ctx context.Context) error {
// 	// Get all flavors
// 	flavors, err := p.bizflyClient.Server.ListFlavors(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to list flavors: %w", err)
// 	}

// 	// Get all zones
// 	zones, err := p.bizflyClient.Server.ListZones(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to list zones: %w", err)
// 	}

// 	// Calculate capacity for each zone
// 	for _, zone := range zones {
// 		capacity := NodeCapacity{
// 			Name: zone.Name,
// 			Capacity: corev1.ResourceList{
// 				corev1.ResourceCPU:    resource.MustParse("0"),
// 				corev1.ResourceMemory: resource.MustParse("0"),
// 			},
// 			Allocatable: corev1.ResourceList{
// 				corev1.ResourceCPU:    resource.MustParse("0"),
// 				corev1.ResourceMemory: resource.MustParse("0"),
// 			},
// 		}

// 		// Add capacity from each flavor
// 		for _, flavor := range flavors {
// 			capacity.Capacity[corev1.ResourceCPU] = *resource.NewQuantity(int64(flavor.VCPUs), resource.DecimalSI)
// 			capacity.Capacity[corev1.ResourceMemory] = *resource.NewQuantity(int64(flavor.RAM)*1024*1024, resource.BinarySI)

// 			capacity.Allocatable[corev1.ResourceCPU] = *resource.NewQuantity(int64(flavor.VCPUs), resource.DecimalSI)
// 			capacity.Allocatable[corev1.ResourceMemory] = *resource.NewQuantity(int64(flavor.RAM)*1024*1024, resource.BinarySI)
// 		}

// 		p.capacityZones[zone.Name] = capacity
// 	}

// 	p.log.V(1).Info("Capacity of zones", "capacityZones", p.capacityZones)
// 	return nil
// }

// func (p *Provider) Zones() []string {
// 	zones := make([]string, 0, len(p.capacityZones))
// 	for zone := range p.capacityZones {
// 		zones = append(zones, zone)
// 	}
// 	return zones
// }

// func (p *Provider) Fit(zone string, req corev1.ResourceList) bool {
// 	capacity, ok := p.capacityZones[zone]
// 	if !ok {
// 		return false
// 	}

// 	return capacity.Allocatable.Cpu().Cmp(*req.Cpu()) >= 0 && capacity.Allocatable.Memory().Cmp(*req.Memory()) >= 0
// }

// func (p *Provider) GetAvailableZones(req corev1.ResourceList) []string {
// 	zones := []string{}
// 	for zone := range p.capacityZones {
// 		if p.Fit(zone, req) {
// 			zones = append(zones, zone)
// 		}
// 	}
// 	return zones
// }

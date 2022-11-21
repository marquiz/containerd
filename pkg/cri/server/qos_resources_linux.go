/*
   Copyright The containerd Authors.

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

package server

import (
	"fmt"

	"github.com/containerd/containerd/v2/oci"
	"github.com/containerd/containerd/v2/pkg/blockio"
	"github.com/containerd/containerd/v2/pkg/rdt"
	"github.com/containerd/log"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// HACK: dummyQoS resources
var dummyContainerQoSResourcesInfo []*runtime.QOSResourceInfo
var dummyContainerQoSResources map[string]map[string]struct{}

var dummyPodQoSResourcesInfo []*runtime.QOSResourceInfo
var dummyPodQoSResources map[string]map[string]struct{}

// generateContainerQoSResourceSpecOpts generates SpecOpts for QoS resources.
func (c *criService) generateContainerQoSResourceSpecOpts(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) ([]oci.SpecOpts, error) {
	specOpts := []oci.SpecOpts{}

	// Handle QoS resource assignments
	for _, r := range config.GetQOSResources() {
		name := r.GetName()
		class := r.GetClass()
		switch name {
		case runtime.QoSResourceRdt:
		case runtime.QoSResourceBlockio:
			// We handle RDT and blockio separately as we have pod and
			// container annotations as fallback interface
		default:
			cr, ok := dummyContainerQoSResources[name]
			if !ok {
				return nil, fmt.Errorf("unknown QoS resource type %q", name)
			}
			if _, ok := cr[class]; !ok {
				return nil, fmt.Errorf("unknown %s class %q", name, class)
			}
			log.L.Infof("setting dummy QoS resource %s=%s", name, class)
		}

		if class == "" {
			return nil, fmt.Errorf("empty class name not allowed for QoS resource type %q", name)
		}

		if class == "" {
			return nil, fmt.Errorf("empty class name not allowed for QoS resource type %q", name)
		}
	}

	// Handle RDT
	if cls, err := c.getContainerRdtClass(config, sandboxConfig); err != nil {
		if !rdt.IsEnabled() && c.config.ContainerdConfig.IgnoreRdtNotEnabledErrors {
			log.L.Debugf("continuing create container %s, ignoring rdt not enabled (%v)", containerName, err)
		} else {
			return nil, fmt.Errorf("failed to set RDT class: %w", err)
		}
	} else if cls != "" {
		specOpts = append(specOpts, oci.WithRdt(cls, "", ""))
	}

	// Handle Block IO
	if cls, err := c.getContainerBlockioClass(config, sandboxConfig); err != nil {
		if !blockio.IsEnabled() && c.config.ContainerdConfig.IgnoreBlockIONotEnabledErrors {
			log.L.Debugf("continuing create container %s, ignoring blockio not enabled (%v)", containerName, err)
		} else {
			return nil, fmt.Errorf("failed to set blockio class: %w", err)
		}
	} else if cls != "" {
		if linuxBlockIO, err := blockio.ClassNameToLinuxOCI(cls); err == nil {
			specOpts = append(specOpts, oci.WithBlockIO(linuxBlockIO))
		} else {
			return nil, err
		}
	}

	return specOpts, nil
}

// GetPodQoSResourcesInfo returns information about all pod-level QoS resources.
func GetPodQoSResourcesInfo() []*runtime.QOSResourceInfo {
	// NOTE: stub as currently no pod-level QoS resources are available
	info := []*runtime.QOSResourceInfo{}
	info = append(info, dummyPodQoSResourcesInfo...)
	return info
}

// GetContainerQoSResourcesInfo returns information about all container-level QoS resources.
func GetContainerQoSResourcesInfo() []*runtime.QOSResourceInfo {
	info := []*runtime.QOSResourceInfo{}

	// Handle RDT
	if classes := rdt.GetClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.QOSResourceInfo{
				Name:    runtime.QoSResourceRdt,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	// Handle blockio
	if classes := blockio.GetClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.QOSResourceInfo{
				Name:    runtime.QoSResourceBlockio,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	info = append(info, dummyContainerQoSResourcesInfo...)

	return info
}

func createClassInfos(names ...string) []*runtime.QOSResourceClassInfo {
	out := make([]*runtime.QOSResourceClassInfo, len(names))
	for i, name := range names {
		out[i] = &runtime.QOSResourceClassInfo{Name: name, Capacity: uint64(i)}
	}
	return out
}

func init() {
	// Initialize our dummy QoS resources hack
	dummuGen := func(in []*runtime.QOSResourceInfo) map[string]map[string]struct{} {
		out := make(map[string]map[string]struct{}, len(in))
		for _, info := range in {
			classes := make(map[string]struct{}, len(info.Classes))
			for _, c := range info.Classes {
				classes[c.Name] = struct{}{}
			}
			out[info.Name] = classes
		}
		return out
	}

	dummyPodQoSResourcesInfo = []*runtime.QOSResourceInfo{
		&runtime.QOSResourceInfo{
			Name:    "podres-1",
			Classes: createClassInfos("qos-a", "qos-b", "qos-c", "qos-d"),
		},
		&runtime.QOSResourceInfo{
			Name:    "podres-2",
			Classes: createClassInfos("cls-1", "cls-2", "cls-3", "cls-4", "cls-5"),
		},
	}

	dummyContainerQoSResourcesInfo = []*runtime.QOSResourceInfo{
		&runtime.QOSResourceInfo{
			Name:    "dummy-1",
			Classes: createClassInfos("class-a", "class-b", "class-c", "class-d"),
		},
		&runtime.QOSResourceInfo{
			Name:    "dummy-2",
			Classes: createClassInfos("platinum", "gold", "silver", "bronze"),
		},
	}

	dummyPodQoSResources = dummuGen(dummyPodQoSResourcesInfo)
	dummyContainerQoSResources = dummuGen(dummyContainerQoSResourcesInfo)
}

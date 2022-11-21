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

	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/services/tasks"
	"github.com/sirupsen/logrus"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// HACK: dummyQoS resources
var dummyContainerQoSResourcesInfo []*runtime.QoSResourceInfo
var dummyContainerQoSResources map[string]map[string]struct{}

var dummyPodQoSResourcesInfo []*runtime.QoSResourceInfo
var dummyPodQoSResources map[string]map[string]struct{}

// generateSandboxQoSResourceSpecOpts generates SpecOpts for QoS resources.
func (c *criService) generateSandboxQoSResourceSpecOpts(config *runtime.PodSandboxConfig) ([]oci.SpecOpts, error) {
	specOpts := []oci.SpecOpts{}

	for r, c := range config.GetQosResources().GetClasses() {
		switch r {
		default:
			cr, ok := dummyPodQoSResources[r]
			if !ok {
				return nil, fmt.Errorf("unknown pod-level QoS resource type %q", r)
			}
			if _, ok := cr[c]; !ok {
				return nil, fmt.Errorf("unknown %s class %q", r, c)
			}
			logrus.Infof("setting dummy QoS resource %s=%s", r, c)
		}

		if c == "" {
			return nil, fmt.Errorf("empty class name not allowed for QoS resource type %q", r)
		}
	}
	return specOpts, nil
}

// generateContainerQoSResourceSpecOpts generates SpecOpts for QoS resources.
func (c *criService) generateContainerQoSResourceSpecOpts(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) ([]oci.SpecOpts, error) {
	specOpts := []oci.SpecOpts{}

	// Handle QoS resource assignments
	for r, c := range config.GetQosResources().GetClasses() {
		switch r {
		case runtime.QoSResourceRdt:
		case runtime.QoSResourceBlockio:
			// We handle RDT and blockio separately as we have pod and
			// container annotations as fallback interface
		default:
			cr, ok := dummyContainerQoSResources[r]
			if !ok {
				return nil, fmt.Errorf("unknown QoS resource type %q", r)
			}
			if _, ok := cr[c]; !ok {
				return nil, fmt.Errorf("unknown %s class %q", r, c)
			}
			logrus.Infof("setting dummy QoS resource %s=%s", r, c)
		}

		if c == "" {
			return nil, fmt.Errorf("empty class name not allowed for QoS resource type %q", r)
		}
	}

	// Handle RDT
	if cls, err := c.getContainerRdtClass(config, sandboxConfig); err != nil {
		if !tasks.RdtEnabled() && c.config.ContainerdConfig.IgnoreRdtNotEnabledErrors {
			logrus.Debugf("continuing create container %s, ignoring rdt not enabled (%v)", containerName, err)
		} else {
			return nil, fmt.Errorf("failed to set RDT class: %w", err)
		}
	} else if cls != "" {
		specOpts = append(specOpts, oci.WithRdt(cls, "", ""))
	}

	// Handle Block IO
	if cls, err := c.getContainerBlockioClass(config, sandboxConfig); err != nil {
		if !tasks.BlockIOEnabled() && c.config.ContainerdConfig.IgnoreBlockIONotEnabledErrors {
			logrus.Debugf("continuing create container %s, ignoring blockio not enabled (%v)", containerName, err)
		} else {
			return nil, fmt.Errorf("failed to set blockio class: %w", err)
		}
	} else if cls != "" {
		if linuxBlockIO, err := blockIOToLinuxOci(cls); err == nil {
			specOpts = append(specOpts, oci.WithBlockIO(linuxBlockIO))
		} else {
			return nil, err
		}
	}

	return specOpts, nil
}

// GetPodQoSResourcesInfo returns information about all pod-level QoS resources.
func GetPodQoSResourcesInfo() []*runtime.QoSResourceInfo {
	// NOTE: stub as currently no pod-level QoS resources are available
	info := []*runtime.QoSResourceInfo{}
	info = append(info, dummyPodQoSResourcesInfo...)
	return info
}

// GetContainerQoSResourcesInfo returns information about all container-level QoS resources.
func GetContainerQoSResourcesInfo() []*runtime.QoSResourceInfo {
	info := []*runtime.QoSResourceInfo{}

	// Handle RDT
	if classes := tasks.GetRdtClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.QoSResourceInfo{
				Name:    runtime.QoSResourceRdt,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	// Handle blockio
	if classes := tasks.GetBlockioClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.QoSResourceInfo{
				Name:    runtime.QoSResourceBlockio,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	info = append(info, dummyContainerQoSResourcesInfo...)

	return info
}

func createClassInfos(names ...string) []*runtime.QoSResourceClassInfo {
	out := make([]*runtime.QoSResourceClassInfo, len(names))
	for i, name := range names {
		out[i] = &runtime.QoSResourceClassInfo{Name: name, Capacity: uint64(i)}
	}
	return out
}

func init() {
	// Initialize our dummy QoS resources hack
	dummuGen := func(in []*runtime.QoSResourceInfo) map[string]map[string]struct{} {
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

	dummyPodQoSResourcesInfo = []*runtime.QoSResourceInfo{
		&runtime.QoSResourceInfo{
			Name:    "podres-1",
			Classes: createClassInfos("qos-a", "qos-b", "qos-c", "qos-d"),
		},
		&runtime.QoSResourceInfo{
			Name:    "podres-2",
			Classes: createClassInfos("cls-1", "cls-2", "cls-3", "cls-4", "cls-5"),
		},
	}

	dummyContainerQoSResourcesInfo = []*runtime.QoSResourceInfo{
		&runtime.QoSResourceInfo{
			Name:    "dummy-1",
			Classes: createClassInfos("class-a", "class-b", "class-c", "class-d"),
		},
		&runtime.QoSResourceInfo{
			Name:    "dummy-2",
			Classes: createClassInfos("platinum", "gold", "silver", "bronze"),
		},
	}

	dummyPodQoSResources = dummuGen(dummyPodQoSResourcesInfo)
	dummyContainerQoSResources = dummuGen(dummyContainerQoSResourcesInfo)
}

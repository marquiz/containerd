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

// HACK: dummyclass resources
var dummyContainerClassResourcesInfo []*runtime.ClassResourceInfo
var dummyContainerClassResources map[string]map[string]struct{}

var dummyPodClassResourcesInfo []*runtime.ClassResourceInfo
var dummyPodClassResources map[string]map[string]struct{}

// generateSandboxClassResourceSpecOpts generates SpecOpts for class resources.
func (c *criService) generateSandboxClassResourceSpecOpts(config *runtime.PodSandboxConfig) ([]oci.SpecOpts, error) {
	specOpts := []oci.SpecOpts{}

	for r, c := range config.GetClassResources().GetClasses() {
		switch r {
		default:
			cr, ok := dummyPodClassResources[r]
			if !ok {
				return nil, fmt.Errorf("unknown pod-level class resource type %q", r)
			}
			if _, ok := cr[c]; !ok {
				return nil, fmt.Errorf("unknown %s class %q", r, c)
			}
			logrus.Infof("setting dummy class resource %s=%s", r, c)
		}

		if c == "" {
			return nil, fmt.Errorf("empty class name not allowed for class resource type %q", r)
		}
	}
	return specOpts, nil
}

// generateContainerClassResourceSpecOpts generates SpecOpts for class resources.
func (c *criService) generateContainerClassResourceSpecOpts(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) ([]oci.SpecOpts, error) {
	specOpts := []oci.SpecOpts{}

	// Handle class resource assignments
	for r, c := range config.GetClassResources().GetClasses() {
		switch r {
		case runtime.ClassResourceRdt:
		case runtime.ClassResourceBlockio:
			// We handle RDT and blockio separately as we have pod and
			// container annotations as fallback interface
		default:
			cr, ok := dummyContainerClassResources[r]
			if !ok {
				return nil, fmt.Errorf("unknown class resource type %q", r)
			}
			if _, ok := cr[c]; !ok {
				return nil, fmt.Errorf("unknown %s class %q", r, c)
			}
			logrus.Infof("setting dummy class resource %s=%s", r, c)
		}

		if c == "" {
			return nil, fmt.Errorf("empty class name not allowed for class resource type %q", r)
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

// GetPodClassResourcesInfo returns information about all pod-level class resources.
func GetPodClassResourcesInfo() []*runtime.ClassResourceInfo {
	// NOTE: stub as currently no pod-level class resources are available
	info := []*runtime.ClassResourceInfo{}
	info = append(info, dummyPodClassResourcesInfo...)
	return info
}

// GetContainerClassResourcesInfo returns information about all container-level class resources.
func GetContainerClassResourcesInfo() []*runtime.ClassResourceInfo {
	info := []*runtime.ClassResourceInfo{}

	// Handle RDT
	if classes := tasks.GetRdtClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.ClassResourceInfo{
				Name:    runtime.ClassResourceRdt,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	// Handle blockio
	if classes := tasks.GetBlockioClasses(); len(classes) > 0 {
		info = append(info,
			&runtime.ClassResourceInfo{
				Name:    runtime.ClassResourceBlockio,
				Mutable: false,
				Classes: createClassInfos(classes...),
			})
	}

	info = append(info, dummyContainerClassResourcesInfo...)

	return info
}

func createClassInfos(names ...string) []*runtime.ClassResourceClassInfo {
	out := make([]*runtime.ClassResourceClassInfo, len(names))
	for i, name := range names {
		out[i] = &runtime.ClassResourceClassInfo{Name: name, Capacity: uint64(i)}
	}
	return out
}

func init() {
	// Initialize our dummy class resources hack
	dummuGen := func(in []*runtime.ClassResourceInfo) map[string]map[string]struct{} {
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

	dummyPodClassResourcesInfo = []*runtime.ClassResourceInfo{
		&runtime.ClassResourceInfo{
			Name:    "podres-1",
			Classes: createClassInfos("qos-a", "qos-b", "qos-c", "qos-d"),
		},
		&runtime.ClassResourceInfo{
			Name:    "podres-2",
			Classes: createClassInfos("cls-1", "cls-2", "cls-3", "cls-4", "cls-5"),
		},
	}

	dummyContainerClassResourcesInfo = []*runtime.ClassResourceInfo{
		&runtime.ClassResourceInfo{
			Name:    "dummy-1",
			Classes: createClassInfos("class-a", "class-b", "class-c", "class-d"),
		},
		&runtime.ClassResourceInfo{
			Name:    "dummy-2",
			Classes: createClassInfos("platinum", "gold", "silver", "bronze"),
		},
	}

	dummyPodClassResources = dummuGen(dummyPodClassResourcesInfo)
	dummyContainerClassResources = dummuGen(dummyContainerClassResourcesInfo)
}

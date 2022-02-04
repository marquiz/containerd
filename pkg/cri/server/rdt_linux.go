//go:build !no_rdt

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

	"github.com/containerd/containerd/services/tasks"
	"github.com/intel/goresctrl/pkg/rdt"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// getContainerRdtClass gets the effective RDT class of a container.
func (c *criService) getContainerRdtClass(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) (string, error) {
	containerName := config.GetMetadata().GetName()

	// Get class from container config
	cls, ok := config.GetClassResources().GetClasses()[runtime.ClassResourceRdt]

	// Fallback: if RDT class is not specified in CRI class resources we check the pod annotations
	if !ok {
		var err error
		cls, err = rdt.ContainerClassFromAnnotations(containerName, config.Annotations, sandboxConfig.Annotations)
		if err != nil {
			return "", err
		}
	}

	if cls != "" {
		// Check that our RDT support status
		if !tasks.RdtEnabled() {
			return "", fmt.Errorf("RDT disabled, refusing to set RDT class of container %q to %q", containerName, cls)
		}
		// Check that the class exists
		if _, ok := rdt.GetClass(cls); !ok {
			return "", fmt.Errorf("invalid RDT class %q: not specified in configuration", cls)
		}
	}

	return cls, nil
}

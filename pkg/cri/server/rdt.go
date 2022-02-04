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

	"github.com/containerd/containerd/v2/pkg/rdt"
	"github.com/containerd/log"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// getContainerRdtClass gets the effective RDT class of a container.
func (c *criService) getContainerRdtClass(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) (cls string, err error) {
	containerName := config.GetMetadata().GetName()

	// Get class from container config
	var found bool
	for _, r := range config.GetQOSResources() {
		if r.GetName() == runtime.QoSResourceRdt {
			found = true
			cls = r.GetClass()
		}
	}
	log.L.Infof("RDT class %q (%v) from container config (%s)", cls, found, containerName)

	// Fallback: if RDT class is not specified in CRI QoS resources we check the pod annotations
	if !found {
		cls, err = rdt.ContainerClassFromAnnotations(containerName, config.Annotations, sandboxConfig.Annotations)
		if err != nil {
			return
		}
	}

	if cls != "" {
		// Check that our RDT support status
		if !rdt.IsEnabled() {
			err = fmt.Errorf("RDT disabled, refusing to set RDT class of container %q to %q", containerName, cls)
		} else if !rdt.ClassExists(cls) {
			err = fmt.Errorf("invalid RDT class %q: not specified in configuration", cls)
		}
		if err != nil {
			log.L.Infof("RDT class %q from annotations (%s)", cls, containerName)
		}
	}

	return
}

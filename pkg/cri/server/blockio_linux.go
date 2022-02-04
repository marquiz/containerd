//go:build linux

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

	"github.com/containerd/containerd/v2/pkg/blockio"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// getContainerBlockioClass gets the effective blockio class of a container.
func (c *criService) getContainerBlockioClass(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) (cls string, err error) {
	containerName := config.GetMetadata().GetName()

	// Get class from container config
	var found bool
	for _, r := range config.GetQOSResources() {
		if r.GetName() == runtime.QoSResourceBlockio {
			found = true
			cls = r.GetClass()
		}
	}

	// Blockio class is not specified in CRI QoS resources. Check annotations as a fallback.
	if !found {
		cls, err = blockio.ContainerClassFromAnnotations(containerName, config.Annotations, sandboxConfig.Annotations)
		if err != nil {
			return
		}
	}

	if cls != "" {
		if !blockio.IsEnabled() {
			err = fmt.Errorf("blockio disabled, refusing to set blockio class of container %q to %q", containerName, cls)
		} else if !blockio.ClassExists(cls) {
			err = fmt.Errorf("invalid blockio class %q: not specified in configuration", cls)
		}
	}

	return
}

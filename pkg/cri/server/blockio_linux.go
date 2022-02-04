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

	"github.com/containerd/containerd/services/tasks"
	"github.com/intel/goresctrl/pkg/blockio"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// getContainerBlockioClass gets the effective blockio class of a container.
func (c *criService) getContainerBlockioClass(config *runtime.ContainerConfig, sandboxConfig *runtime.PodSandboxConfig) (string, error) {
	containerName := config.GetMetadata().GetName()

	// Get class from container config
	cls, ok := config.GetQosResources().GetClasses()[runtime.QoSResourceBlockio]
	logrus.Infof("Block IO class %q (%v) from container config (%s)", cls, ok, containerName)

	// Blockio class is not specified in CRI QoS resources. Check annotations as a fallback.
	if !ok {
		var err error
		cls, err = blockio.ContainerClassFromAnnotations(containerName, config.Annotations, sandboxConfig.Annotations)
		if err != nil {
			return "", err
		}
		logrus.Infof("Block IO class %q from annotations (%s)", cls, containerName)
	}

	if cls != "" {
		if !tasks.BlockIOEnabled() {
			return "", fmt.Errorf("blockio disabled, refusing to set blockio class of container %q to %q", containerName, cls)
		}
		if !classExists(cls) {
			return "", fmt.Errorf("invalid blockio class %q: not specified in configuration", cls)
		}
	}

	return cls, nil
}

func classExists(cls string) bool {
	for _, c := range blockio.GetClasses() {
		if cls == c {
			return true
		}
	}
	return false
}

// blockIOToLinuxOci converts blockio class name into the LinuxBlockIO
// structure in the OCI runtime spec.
func blockIOToLinuxOci(className string) (*runtimespec.LinuxBlockIO, error) {
	return blockio.OciLinuxBlockIO(className)
}

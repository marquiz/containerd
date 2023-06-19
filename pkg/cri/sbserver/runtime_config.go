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

package sbserver

import (
	"context"
	"sort"

	"github.com/containerd/containerd/log"
	runcoptions "github.com/containerd/containerd/runtime/v2/runc/options"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RuntimeConfig returns configuration information of the runtime.
func (c *criService) RuntimeConfig(ctx context.Context, r *runtime.RuntimeConfigRequest) (*runtime.RuntimeConfigResponse, error) {
	resp := &runtime.RuntimeConfigResponse{
		Linux: &runtime.LinuxRuntimeConfiguration{
			CgroupDriver: c.getCgroupDriver(ctx),
		},
	}
	return resp, nil
}

func (c *criService) getCgroupDriver(ctx context.Context) runtime.CgroupDriver {
	// Go through the runtime handlers in a predictable order, starting from the
	// default handler, others sorted in alphabetical order
	handlerNames := make([]string, 0, len(c.config.ContainerdConfig.Runtimes))
	for n := range c.config.ContainerdConfig.Runtimes {
		handlerNames = append(handlerNames, n)
	}
	sort.Slice(handlerNames, func(i, j int) bool {
		if handlerNames[i] == c.config.ContainerdConfig.DefaultRuntimeName {
			return true
		}
		if handlerNames[j] == c.config.ContainerdConfig.DefaultRuntimeName {
			return false
		}
		return handlerNames[i] < handlerNames[j]
	})

	// Fall back to finding cgroup driver setting from other runtime handlers
	for _, handler := range handlerNames {
		opts, err := generateRuntimeOptions(c.config.ContainerdConfig.Runtimes[handler])
		if err != nil {
			log.G(ctx).Debugf("failed to parse runtime handler options for %q", handler)
			continue
		}
		if d, ok := getCgroupDriverFromRuntimeHandlerOpts(opts); ok {
			return d
		}
		log.G(ctx).Debugf("runtime handler %q does not provide cgroup driver information", handler)
	}

	// If no runtime handlers have a setting, return systemd as the default
	log.G(ctx).Debugf("no runtime handler provided cgroup driver information, returning systemd as a fallback")
	return runtime.CgroupDriver_SYSTEMD
}

func getCgroupDriverFromRuntimeHandlerOpts(opts interface{}) (runtime.CgroupDriver, bool) {
	switch v := opts.(type) {
	case *runcoptions.Options:
		systemdCgroup := v.SystemdCgroup
		if systemdCgroup {
			return runtime.CgroupDriver_SYSTEMD, true
		}
		return runtime.CgroupDriver_CGROUPFS, true
	}
	return runtime.CgroupDriver_SYSTEMD, false
}

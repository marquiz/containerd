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
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd/v2/oci"
	"github.com/containerd/containerd/v2/pkg/blockio"
	"github.com/containerd/containerd/v2/pkg/rdt"
	cni "github.com/containerd/go-cni"
	"github.com/containerd/log"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	// QoSResourceNet is the name of the CNI QoS resource
	QoSResourceNet = "net"
)

type CniQoSClass struct {
	// Capacity is the max number of simultaneous pods that can use this class
	Capacity     uint64
	Capabilities struct {
		BandWidth *cni.BandWidth
	}
}

var cniQoSResource map[string]CniQoSClass

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

func generateCniQoSResourceOpts(config *runtime.PodSandboxConfig) ([]cni.NamespaceOpts, error) {
	nsOpts := []cni.NamespaceOpts{}

	for _, r := range config.GetQOSResources() {
		if r.GetName() == QoSResourceNet {
			className := r.GetClass()
			class, ok := cniQoSResource[className]
			if !ok {
				return nil, fmt.Errorf("unknown %q class %q", QoSResourceNet, className)
			}
			caps := class.Capabilities
			if caps.BandWidth != nil {
				nsOpts = append(nsOpts, cni.WithCapabilityBandWidth(*caps.BandWidth))
			}
			break
		}
	}
	return nsOpts, nil
}

// GetPodQoSResourcesInfo returns information about all pod-level QoS resources.
func GetPodQoSResourcesInfo() []*runtime.QOSResourceInfo {
	info := []*runtime.QOSResourceInfo{}

	if len(cniQoSResource) > 0 {
		classes := make([]*runtime.QOSResourceClassInfo, 0, len(cniQoSResource))
		for n, c := range cniQoSResource {
			classes = append(classes, &runtime.QOSResourceClassInfo{Name: n, Capacity: c.Capacity})
		}

		info = append(info, &runtime.QOSResourceInfo{
			Name:    QoSResourceNet,
			Mutable: false,
			Classes: classes,
		})
	}

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

func updateCniQoSResources(netplugin cni.CNI) error {
	qos, err := getCniQoSResources(netplugin)
	if err != nil {
		return err
	}
	cniQoSResource = qos
	return nil
}

func getCniQoSResources(netplugin cni.CNI) (map[string]CniQoSClass, error) {
	if netplugin == nil {
		return nil, fmt.Errorf("BUG: unable to parse CNI QoS resources, nil plugin was given")
	}

	cniConfig := netplugin.GetConfig()
	if len(cniConfig.Networks) < 2 {
		return nil, fmt.Errorf("unable to parse CNI config for QoS resources: no networks configured")
	}
	rawConf := cniConfig.Networks[1].Config.Source

	/*if len(cniConfig.Networks[1].Config.Plugins) == 0 {
		return nil, fmt.Errorf("unable to parse CNI config for QoS resources: no plugin configuration found in network")
	}
	rawConf := cniConfig.Networks[1].Config.Plugins[0].Source*/

	tmp := struct {
		Name string                 `json:"name,omitempty"`
		Qos  map[string]CniQoSClass `json:"qos,omitempty"`
	}{}
	log.L.Infof("parsing CNI  QoS config: %s", rawConf)

	if err := json.Unmarshal([]byte(rawConf), &tmp); err != nil {
		log.L.Infof("failed to parse CNI config: %s", rawConf)
		return nil, fmt.Errorf("failed to parse CNI config for QoS resources: %w", err)
	}

	log.L.Infof("parsed CNI  QoS config: %s", tmp)

	return tmp.Qos, nil
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

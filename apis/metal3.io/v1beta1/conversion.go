/*


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

package v1beta1

// Hub marks these types as the conversion hub (storage version) for the
// metal3.io API group. All other spoke versions (e.g. v1alpha1) convert to
// and from these types.

func (*BareMetalHost) Hub()              {}
func (*BareMetalHostList) Hub()          {}
func (*BareMetalSwitch) Hub()            {}
func (*BareMetalSwitchList) Hub()        {}
func (*BMCEventSubscription) Hub()       {}
func (*BMCEventSubscriptionList) Hub()   {}
func (*DataImage) Hub()                  {}
func (*DataImageList) Hub()              {}
func (*FirmwareSchema) Hub()             {}
func (*FirmwareSchemaList) Hub()         {}
func (*HardwareData) Hub()               {}
func (*HardwareDataList) Hub()           {}
func (*HostClaim) Hub()                  {}
func (*HostClaimList) Hub()              {}
func (*HostDeployPolicy) Hub()           {}
func (*HostDeployPolicyList) Hub()       {}
func (*HostFirmwareComponents) Hub()     {}
func (*HostFirmwareComponentsList) Hub() {}
func (*HostFirmwareSettings) Hub()       {}
func (*HostFirmwareSettingsList) Hub()   {}
func (*HostNetworkAttachment) Hub()      {}
func (*HostNetworkAttachmentList) Hub()  {}
func (*HostUpdatePolicy) Hub()           {}
func (*HostUpdatePolicyList) Hub()       {}
func (*PreprovisioningImage) Hub()       {}
func (*PreprovisioningImageList) Hub()   {}

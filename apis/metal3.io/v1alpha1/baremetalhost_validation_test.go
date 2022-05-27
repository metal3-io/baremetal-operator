package v1alpha1

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func errorArrContains(out []error, want string) bool {
	if out == nil {
		return want == ""
	}
	if want == "" {
		return false
	}
	for _, err := range out {
		if err.Error() == want {
			return true
		}
	}
	return false
}

func TestValidateCreate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BareMetalHost",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	inom := metav1.ObjectMeta{
		Name:      "test~",
		Namespace: "test-namespace",
	}

	inom2 := metav1.ObjectMeta{
		Name:      "07564256-96ae-4315-ab03-8d34ece60fbb",
		Namespace: "test-namespace",
	}
	enable := true

	// for RAID validation test cases
	numberOfPhysicalDisks := 3

	tests := []struct {
		name      string
		newBMH    *BareMetalHost
		oldBMH    *BareMetalHost
		wantedErr string
	}{
		{
			name:      "valid",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name:      "invalidName",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: inom, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot contain characters other than [A-Za-z0-9._-]",
		},
		{
			name:      "invalidName2",
			newBMH:    &BareMetalHost{TypeMeta: tm, ObjectMeta: inom2, Spec: BareMetalHostSpec{}},
			oldBMH:    nil,
			wantedErr: "BareMetalHost resource name cannot be a UUID",
		},
		{
			name: "invalidRAID",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
						SoftwareRAIDVolumes: []SoftwareRAIDVolume{
							{
								SizeGibibytes: nil,
								Level:         "",
								PhysicalDisks: nil,
							},
						},
					}}},
			oldBMH:    nil,
			wantedErr: "hardwareRAIDVolumes and softwareRAIDVolumes can not be set at the same time",
		},
		{
<<<<<<< HEAD
			name: "supportBMCType",
=======
			name: "validDNSName",
>>>>>>> 260d5ce5... add more testcases, minor fixes
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
<<<<<<< HEAD
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
=======
					BMC: BMCDetails{
						Address: "ipmi://host-0.example.com.org"}}},
>>>>>>> 260d5ce5... add more testcases, minor fixes
			oldBMH:    nil,
			wantedErr: "",
		},
		{
<<<<<<< HEAD
			name: "unsupportBMCType",
=======
			name: "validDNSName2",
>>>>>>> 260d5ce5... add more testcases, minor fixes
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
<<<<<<< HEAD
						Address:         "test:127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "Unknown BMC type 'test' for address test:127.0.1.1",
		},
		{
			name: "RAIDWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
					},
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
=======
						Address: "ipmi://baremetalhost"}}},
>>>>>>> 260d5ce5... add more testcases, minor fixes
			oldBMH:    nil,
			wantedErr: "",
		},
		{
<<<<<<< HEAD
			name: "RAIDWithUnsupportBMC",
=======
			name: "invalidDNSName",
>>>>>>> 260d5ce5... add more testcases, minor fixes
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								NumberOfPhysicalDisks: nil,
							},
						},
					},
					BMC: BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver ipmi does not support configuring RAID",
		},
		{
			name: "FirmwareWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					Firmware: &FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BootMACAddress: "01:02:03:04:05:06",
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "FirmwareWithUnsupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					Firmware: &FirmwareConfig{
						VirtualizationEnabled: &enable,
					},
					BMC: BMCDetails{
						Address:         "ipmi://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "firmware settings for ipmi are not supported",
		},
		{
			name: "BootMACAddressRequiredWithoutBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt requires a BootMACAddress value",
		},
		{
			name: "BootMACAddressRequiredWithBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "BootMACAddressRequired",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "libvirt://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "BMC driver libvirt does not support secure boot",
		},
		{
			name: "InvalidBootMACAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "address 00:00:00:00:00: invalid MAC address",
		},
		{
			name: "UEFISecureBootWithSupportBMC",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "irmc://127.0.1.1",
						CredentialsName: "test1",
					},
					BootMACAddress: "00:00:00:00:00:00",
					BootMode:       UEFISecureBoot,
				}},
			oldBMH:    nil,
			wantedErr: "",
		},
		{
			name: "'physicalDisks' in HardwareRAID without 'controller'.",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "idrac://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes: nil,
								Level:         "",
								Name:          "",
								Rotational:    nil,
								PhysicalDisks: []string{"Disk-1", "Disk-2"},
							}, // end of RAID volume
						}, // end of RAID volume slice
					}, // end of RAID config
				}, // end of BMH spec
			},
			oldBMH:    nil,
			wantedErr: "'physicalDisks' specified without 'controller' in hardware RAID volume 0",
		},
		{
			name: "'numberOfPhysicalDisks' not same as length of 'physicalDisks'",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address:         "idrac://127.0.0.1",
						CredentialsName: "test1",
					},
					RAID: &RAIDConfig{
						HardwareRAIDVolumes: []HardwareRAIDVolume{
							{
								SizeGibibytes:         nil,
								Level:                 "",
								Name:                  "",
								Rotational:            nil,
								Controller:            "Controller-1",
								PhysicalDisks:         []string{"Disk-1", "Disk-2"},
								NumberOfPhysicalDisks: &numberOfPhysicalDisks, // defined as 3 above
							}, // end of RAID volume
						}, // end of RAID volume slice
					}, // end of RAID config
				}, // end of BMH spec
			},
			oldBMH:    nil,
			wantedErr: fmt.Sprintf("the 'numberOfPhysicalDisks'[%d] and number of 'physicalDisks'[2] is not same for volume 0", numberOfPhysicalDisks),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.newBMH.validateHost(); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateBareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BareMetalHost",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		newBMH    *BareMetalHost
		oldBMH    *BareMetalHost
		wantedErr string
	}{
		{
			name: "updateAddress",
			newBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address-changed"}}},
			oldBMH: &BareMetalHost{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "test-address"}}},
			wantedErr: "BMC address can not be changed once it is set",
		},
		{
			name: "updateBootMAC",
			newBMH: &BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{BootMACAddress: "test-mac-changed"}},
			oldBMH: &BareMetalHost{
				TypeMeta: tm, ObjectMeta: om, Spec: BareMetalHostSpec{BootMACAddress: "test-mac"}},
			wantedErr: "bootMACAddress can not be changed once it is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.newBMH.validateChanges(tt.oldBMH); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateBareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

<volume>
  <name>{{ .VolumeName }}.qcow2</name>
  <capacity unit="G">{{ .VolumeCapacityInGB }}</capacity>
  <target>
    <format type='qcow2'/>
  </target>
</volume>

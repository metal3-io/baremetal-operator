<volume>
  <name>{{ .Name }}.qcow2</name>
  <capacity unit="G">{{ .Size }}</capacity>
  <target>
    <format type='qcow2'/>
  </target>
</volume>

<network>
  <name>{{ .Name }}</name>
  <forward mode='nat'>
    <nat>
      <port start='1024' end='65535'/>
    </nat>
  </forward>
  <bridge name='{{ .Bridge }}'/>
  <ip family='{{ .IPFamily }}' address='{{ .Address }}' prefix='{{ .Netmask }}'/>
  <!-- disabling dns disables libvirt dnsmasq server, so it does not -->
  <!-- interfere with ironic dnsmasq server -->
  <dns enable='no' />
</network>

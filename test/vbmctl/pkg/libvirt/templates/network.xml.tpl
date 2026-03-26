<network>
  <name>{{ .Name }}</name>
  <forward mode='nat'>
    <nat>
      <port start='1024' end='65535'/>
    </nat>
  </forward>
  <bridge name='{{ .Bridge }}'/>
  <ip address='{{ .Address }}' netmask='{{ .Netmask}}'/>
</network>

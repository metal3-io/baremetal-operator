# Serial-console fixtures

These minimal standard Redfish shapes follow the DMTF schemas:

- `system.json`: ComputerSystem v1.26.0, `HostSerialConsole.IPMI`.
- `manager.json`: Manager v1.20.0, `SerialConsole`, `NetworkProtocol` and
   `Links.ManagerForServers`.
- `network-protocol.json`: ManagerNetworkProtocol, `IPMI.ProtocolEnabled` and
   `IPMI.Port`.

They contain no SushyTools OEM extensions. OEM compatibility is tested
separately so it is not mistaken for standard Manager discovery.

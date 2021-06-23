## Load Balancer Request Parameters
The Harvester cloud controller manager can configure the load balancer request parameters by the annotations of services.

### Load Balancer Type
The Harvester load balancer has two types.
- Internal: The load balancer will get an IPv4 address that can only be accessed inside Harvester.
- External: The load balancer will get an IPv4 address from the DHCP server. Whether the IPv4 address is in the LAN or WAN depends on the DHCP server configuration.

We can configure the type by the annotation key `loadbalancer.harvesterhci.io/type`. Its value can be `internal` and `external`.

### Health Check
Harvester cloud controller manager supports TCP health check. We explain the meaning of the related annotations below.<br>
- `loadbalancer.harvesterhci.io/healthCheckPort` specifies the port. The prober will access the address composed of the backend server IP and the port. This option is required.
- `loadbalancer.harvesterhci.io/healthCheckSuccessThreshold` specifies the health check success threshold. The default value is 1. If the number of times that the prober continuously successfully detects an address reaches the success threshold, the backend server can start to forward traffic.
- `loadbalancer.harvesterhci.io/healthCheckFailureThreshold` specify the success and failure threshold. The default value is 3. The backend server will stop to forward traffic if the number of health check failure reaches the failure threshold. 
- `loadbalancer.harvesterhci.io/healthCheckPeriodSeconds` specifies the health check period. The default value is 5 seconds.
- `loadbalancer.harvesterhci.io/healthCheckTimeoutSeconds` specifies the timeout of every health check. The default value is 3 seconds.

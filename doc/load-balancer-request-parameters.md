## Load Balancer Request Parameters
The Harvester cloud controller manager can configure the load balancer request parameters by the annotations of services.

### IPAM
We can configure the IPAM mode by the annotation key `cloudprovider.harvesterhci.io/ipam`. Its value can be `pool` and `dhcp`. Defaults to `pool`.
- pool: Users should configure an IP address pool in the Harvester. The Harvester LoadBalancer will allocate an address from the IP address poll for the load balancer.
- dhcp: It requires a DHCP server. The Harvester LoadBalancer will request an address for the service from the DHCP server.

### Health Check
Harvester cloud controller manager supports TCP health check. We explain the meaning of the related annotations below.<br>
- `cloudprovider.harvesterhci.io/healthcheck-port` specifies the port. The prober will access the address composed of the backend server IP and the port. This option is required.
- `cloudprovider.harvesterhci.io/healthcheck-success-threshold` specifies the health check success threshold. The default value is 1. If the number of times that the prober continuously successfully detects an address reaches the success threshold, the backend server can start to forward traffic.
- `cloudprovider.harvesterhci.io/healthcheck-failure-threshold` specify the success and failure threshold. The default value is 3. The backend server will stop to forward traffic if the number of health check failure reaches the failure threshold. 
- `cloudprovider.harvesterhci.io/healthcheck-periodseconds` specifies the health check period. The default value is 5 seconds.
- `cloudprovider.harvesterhci.io/healthcheck-timeoutseconds` specifies the timeout of every health check. The default value is 3 seconds.

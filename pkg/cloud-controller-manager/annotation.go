package ccm

const (
	// Annotation key
	prefix                      = "cloudprovider.harvesterhci.io/"
	loadBalancerDescription     = prefix + "description"
	loadBalancerIPAM            = prefix + "ipam"
	healthCheckPort             = prefix + "healthcheck-port"
	healthCheckSuccessThreshold = prefix + "healthcheck-success-threshold"
	healthCheckFailureThreshold = prefix + "healthcheck-failure-threshold"
	healthCheckPeriodSeconds    = prefix + "healthcheck-periodseconds"
	healthCheckTimeoutSeconds   = prefix + "healthcheck-timeoutseconds"

	// Default value
	defaultSuccessThreshold = 1
	defaultFailThreshold    = 3
	defaultPeriodSeconds    = 5
	defaultTimeoutSeconds   = 3
)
